// Package boxplan compiles and compares Plans without materializing resources.
package boxplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/plandiff"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
)

const maxRetainedPlanBytes = 16 << 20

// State is the read-only state-store projection required by planning.
type State interface {
	CurrentPlan(context.Context, string) (state.CurrentPlanRecord, bool, error)
}

// Request identifies desired intent. The caller must derive all identity fields
// from the same canonical manifest path and task key.
type Request struct {
	WorkspaceID  identity.WorkspaceID
	BoxID        identity.BoxID
	TaskKey      string
	ManifestPath string
}

// Service contains observation and compilation capabilities only.
type Service struct {
	State   State
	Git     repository.Git
	Paths   *paths.Provider
	Runtime plan.RuntimeFacts
	Tools   contract.ToolVersions
}

// Compile resolves desired intent and compares it with retained and live state.
// It has no collaborator capable of creating a Box resource.
func (s Service) Compile(ctx context.Context, request Request) (contract.PlanOutput, error) {
	if err := s.validate(request); err != nil {
		return contract.PlanOutput{}, err
	}
	workspaceRoot := sourceRoot(request.ManifestPath)
	loader, err := manifest.NewLoader([]string{workspaceRoot}, manifest.DefaultLimits())
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: configure manifest loader: %w", err)
	}
	workspace, err := loader.Load(request.ManifestPath)
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: load manifest: %w", err)
	}
	facts, err := (repository.Resolver{Git: s.Git, Paths: s.Paths}).Resolve(ctx, workspace, request.BoxID, request.TaskKey)
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: %w", err)
	}
	desired, _, err := plan.Compile(plan.Input{
		Workspace: workspace, WorkspaceID: request.WorkspaceID, BoxID: request.BoxID,
		TaskKey: request.TaskKey, ManifestPath: request.ManifestPath,
		Repositories: facts, Runtime: s.Runtime, Tools: s.Tools,
	})
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: %w", err)
	}

	currentRecord, found, err := s.State.CurrentPlan(ctx, request.BoxID.String())
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: observe current Plan: %w", err)
	}
	if !found {
		return absentOutput(desired), nil
	}
	expectedPlanPath, err := s.Paths.Plan(request.BoxID)
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: construct current Plan path: %w", err)
	}
	if currentRecord.Path != expectedPlanPath {
		return contract.PlanOutput{}, errors.New("plan Box: retained Plan path does not match Box ownership")
	}
	current, err := readPlan(currentRecord.Path)
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: read current Plan: %w", err)
	}
	if current.WorkspaceID != desired.WorkspaceID || current.BoxID != desired.BoxID || current.TaskKey != desired.TaskKey || current.Manifest.Path != desired.Manifest.Path {
		return diffOutput(desired, plandiff.Compare(&current, &desired, plandiff.Observation{})), nil
	}
	boxRoot, err := s.Paths.BoxRoot(request.BoxID)
	if err != nil {
		return contract.PlanOutput{}, fmt.Errorf("plan Box: construct Box root: %w", err)
	}
	for _, item := range current.Repositories {
		if !within(boxRoot, item.WorktreePath) || !within(boxRoot, item.GitCommonDir) {
			return contract.PlanOutput{}, errors.New("plan Box: retained repository path escapes Box ownership")
		}
	}
	observed, err := s.observeRepositories(ctx, current)
	if err != nil {
		return contract.PlanOutput{}, err
	}
	return diffOutput(desired, plandiff.Compare(&current, &desired, observed)), nil
}

func (s Service) validate(request Request) error {
	if s.State == nil || s.Git == nil || s.Paths == nil {
		return errors.New("plan Box: state, Git observer, and path provider are required")
	}
	if err := identity.ValidateWorkspaceID(request.WorkspaceID); err != nil {
		return fmt.Errorf("plan Box: %w", err)
	}
	if err := identity.ValidateBoxID(request.BoxID); err != nil {
		return fmt.Errorf("plan Box: %w", err)
	}
	if err := identity.ValidateTaskKey(request.TaskKey); err != nil {
		return fmt.Errorf("plan Box: %w", err)
	}
	if request.ManifestPath == "" || !filepath.IsAbs(request.ManifestPath) || filepath.Clean(request.ManifestPath) != request.ManifestPath {
		return errors.New("plan Box: manifest path must be clean and absolute")
	}
	workspaceID, err := identity.Workspace(request.ManifestPath)
	if err != nil || workspaceID != request.WorkspaceID {
		return errors.New("plan Box: workspace identity does not match manifest path")
	}
	boxID, err := identity.Box(request.WorkspaceID, request.TaskKey)
	if err != nil || boxID != request.BoxID {
		return errors.New("plan Box: Box identity does not match workspace and task")
	}
	if runtime.GOOS == "darwin" && s.Runtime.Driver != "lima-vz" || runtime.GOOS != "darwin" && s.Runtime.Driver != "lima-qemu" {
		return errors.New("plan Box: runtime driver does not match the current platform")
	}
	return nil
}

func sourceRoot(manifestPath string) string {
	directory := filepath.Dir(manifestPath)
	if filepath.Base(directory) == ".abox" {
		return filepath.Dir(directory)
	}
	return directory
}

func (s Service) observeRepositories(ctx context.Context, current contract.Plan) (plandiff.Observation, error) {
	observed := plandiff.Observation{Repositories: make([]plandiff.RepositoryObservation, 0, len(current.Repositories))}
	for _, item := range current.Repositories {
		status, err := s.Git.Status(ctx, item.WorktreePath)
		if err != nil {
			return plandiff.Observation{}, fmt.Errorf("plan Box: observe repository %s worktree: %w", item.Name, err)
		}
		changed, err := repository.ChangedPaths(status.Stdout)
		if err != nil {
			return plandiff.Observation{}, fmt.Errorf("plan Box: parse repository %s worktree status: %w", item.Name, err)
		}
		observed.Repositories = append(observed.Repositories, plandiff.RepositoryObservation{Name: item.Name, Dirty: len(changed) != 0})
	}
	return observed, nil
}

func readPlan(path string) (contract.Plan, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return contract.Plan{}, errors.New("retained Plan path is not clean and absolute")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return contract.Plan{}, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return contract.Plan{}, errors.New("retained Plan is not a real regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return contract.Plan{}, err
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return contract.Plan{}, statErr
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return contract.Plan{}, errors.New("retained Plan is not a regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxRetainedPlanBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return contract.Plan{}, errors.Join(readErr, closeErr)
	}
	if len(data) > maxRetainedPlanBytes {
		return contract.Plan{}, errors.New("retained Plan exceeds size limit")
	}
	var current contract.Plan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&current); err != nil {
		return contract.Plan{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return contract.Plan{}, errors.New("retained Plan contains trailing data")
	}
	if _, err := plan.CanonicalJSON(current); err != nil {
		return contract.Plan{}, err
	}
	return current, nil
}

func absentOutput(desired contract.Plan) contract.PlanOutput {
	operations := make([]contract.PlanOperation, 0, len(desired.Repositories)+len(desired.TCPPorts)+3)
	operations = append(operations, contract.PlanOperation{
		Action: "create", Resource: "plan", Reason: "box-absent",
		Description: "retain the compiled immutable Plan during reconciliation",
	})
	for _, item := range desired.Repositories {
		operations = append(operations, contract.PlanOperation{
			Action: "create", Resource: "repository/" + item.Name, Reason: "repository-absent",
			Description: "create the private Git store and linked worktree during reconciliation",
		})
	}
	for _, item := range desired.TCPPorts {
		operations = append(operations, contract.PlanOperation{
			Action: "create", Resource: "tcp-port/" + item.Name, Reason: "tcp-port-absent",
			Description: "allocate and verify the stable Machine-loopback TCP endpoint during reconciliation",
		})
	}
	operations = append(operations,
		contract.PlanOperation{Action: "create", Resource: "runtime-host", Reason: "runtime-host-absent", Description: "create and start the planned runtime host during reconciliation"},
		contract.PlanOperation{Action: "create", Resource: "compose", Reason: "compose-project-absent", Description: "compile and start the planned Compose project during reconciliation"},
	)
	return contract.PlanOutput{
		SchemaVersion: contract.PlanOutputSchemaVersion, Plan: desired,
		Classification: "in-place", Reason: "box-absent", Operations: operations,
	}
}

func diffOutput(desired contract.Plan, result plandiff.Result) contract.PlanOutput {
	operations := make([]contract.PlanOperation, 0, len(result.Changes))
	for _, change := range result.Changes {
		operation := contract.PlanOperation{
			Action: actionFor(change.Class), Resource: change.Resource, Reason: string(change.Reason), Description: change.Description,
			Restart: change.Class == plandiff.ClassRestart, Conflict: change.Class == plandiff.ClassConflict,
			Destructive: change.Class == plandiff.ClassRemoval || destructiveConflict(change.Reason),
		}
		operations = append(operations, operation)
	}
	return contract.PlanOutput{
		SchemaVersion: contract.PlanOutputSchemaVersion, Plan: desired,
		Classification: string(result.Classification), Reason: string(result.Reason), Operations: operations,
	}
}

func actionFor(class plandiff.Class) string {
	switch class {
	case plandiff.ClassRestart:
		return "restart"
	case plandiff.ClassGrowth:
		return "grow"
	case plandiff.ClassRemoval:
		return "remove"
	case plandiff.ClassConflict:
		return "conflict"
	default:
		return "update"
	}
}

func destructiveConflict(reason plandiff.Reason) bool {
	return reason == plandiff.ReasonRuntimeDiskShrink ||
		reason == plandiff.ReasonDirtyCheckoutReplacement ||
		reason == plandiff.ReasonDirtyRepositoryRemoval
}

func within(root, candidate string) bool {
	if root == "" || candidate == "" || !filepath.IsAbs(root) || !filepath.IsAbs(candidate) {
		return false
	}
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
