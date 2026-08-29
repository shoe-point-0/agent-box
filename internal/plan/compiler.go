// Package plan compiles validated intent and observed facts into immutable Plans.
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

const MaxArtifactBytes int64 = 16 << 20

// RuntimeFacts records behavior-affecting runtime-host facts observed before
// planning. Driver is currently either lima-qemu or lima-vz.
type RuntimeFacts struct {
	Driver string
}

// Input combines validated user intent with read-only platform observations.
// Compile reads Plan input artifacts but does not create or mutate resources.
type Input struct {
	Workspace    *manifest.Workspace
	WorkspaceID  identity.WorkspaceID
	BoxID        identity.BoxID
	TaskKey      string
	ManifestPath string
	Repositories []repository.Facts
	Runtime      RuntimeFacts
	Tools        contract.ToolVersions
}

// Compile returns the versioned Plan and its compact canonical JSON encoding.
func Compile(input Input) (contract.Plan, []byte, error) {
	if err := validateInput(input); err != nil {
		return contract.Plan{}, nil, err
	}

	manifestBytes, err := readRegularFile(input.ManifestPath, "manifest")
	if err != nil {
		return contract.Plan{}, nil, err
	}
	facts := input.Repositories[0]
	include := input.Workspace.Spec.Compose.Includes[0]
	composeSource := filepath.Join(facts.SourceRoot, include.Path)
	if err := requireContainedPath(facts.SourceRoot, composeSource, "Compose include"); err != nil {
		return contract.Plan{}, nil, err
	}
	composeBytes, err := readRegularFile(composeSource, "Compose include")
	if err != nil {
		return contract.Plan{}, nil, err
	}

	composePath := filepath.Join(facts.WorktreePath, include.Path)
	mounts := compileMounts(input.Workspace, facts)
	profiles := canonicalSet(input.Workspace.Spec.Compose.Profiles)
	name := "abox-" + input.BoxID.String()
	plan := contract.Plan{
		SchemaVersion: contract.PlanSchemaVersion,
		WorkspaceID:   input.WorkspaceID.String(),
		BoxID:         input.BoxID.String(),
		TaskKey:       input.TaskKey,
		Manifest: contract.Artifact{
			Path: input.ManifestPath, Digest: contentDigest(manifestBytes),
		},
		Repositories: []contract.PlanRepository{{
			Name: facts.Name, SourcePath: facts.SourceRoot, BaseCommit: facts.BaseCommit,
			Branch: facts.Branch, WorktreePath: facts.WorktreePath, GitCommonDir: facts.GitStorePath,
		}},
		RuntimeHost: contract.RuntimeHostPlan{
			Driver: input.Runtime.Driver, InstanceName: name,
			CPUMillicores: input.Workspace.Spec.Host.CPUMillicores,
			MemoryBytes:   input.Workspace.Spec.Host.MemoryBytes,
			DiskBytes:     input.Workspace.Spec.Host.DiskBytes,
		},
		Compose: contract.ComposePlan{
			ProjectName: name,
			Includes:    []contract.Artifact{{Path: composePath, Digest: contentDigest(composeBytes)}},
			Profiles:    profiles,
		},
		WorkspaceService: contract.WorkspaceServicePlan{
			Name:             input.Workspace.Spec.WorkspaceService.Name,
			Repository:       input.Workspace.Spec.WorkspaceService.Repository,
			WorkingDirectory: input.Workspace.Spec.WorkspaceService.WorkingDirectory,
		},
		Mounts: mounts,
		Tools:  input.Tools,
	}
	plan.PlanDigest, err = digestPlan(plan)
	if err != nil {
		return contract.Plan{}, nil, fmt.Errorf("compile Plan digest: %w", err)
	}
	encoded, err := CanonicalJSON(plan)
	if err != nil {
		return contract.Plan{}, nil, err
	}
	return plan, encoded, nil
}

// CanonicalJSON serializes a Plan only when its digest still matches its
// content, detecting mutation after compilation.
func CanonicalJSON(plan contract.Plan) ([]byte, error) {
	digest, err := digestPlan(plan)
	if err != nil {
		return nil, fmt.Errorf("encode canonical Plan: %w", err)
	}
	if plan.PlanDigest != digest {
		return nil, errors.New("encode canonical Plan: plan digest does not match content")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode canonical Plan: %w", err)
	}
	return encoded, nil
}

func validateInput(input Input) error {
	if input.Workspace == nil {
		return errors.New("compile Plan: workspace is required")
	}
	if err := manifest.CheckMilestone1Executable(input.Workspace); err != nil {
		return fmt.Errorf("compile Plan: %w", err)
	}
	if len(input.Workspace.Spec.Repositories) != 1 {
		return unsupported("spec.repositories", "multiple repositories")
	}
	if len(input.Workspace.Spec.Compose.Includes) != 1 {
		return unsupported("spec.compose.includes", "multiple Compose includes")
	}
	if len(input.Workspace.Spec.Mounts) == 0 {
		return errors.New("compile Plan: spec.mounts must contain the repository mount")
	}
	if err := identity.ValidateWorkspaceID(input.WorkspaceID); err != nil {
		return fmt.Errorf("compile Plan: %w", err)
	}
	if err := identity.ValidateBoxID(input.BoxID); err != nil {
		return fmt.Errorf("compile Plan: %w", err)
	}
	if err := identity.ValidateTaskKey(input.TaskKey); err != nil {
		return fmt.Errorf("compile Plan: %w", err)
	}
	workspaceID, err := identity.Workspace(input.ManifestPath)
	if err != nil {
		return fmt.Errorf("compile Plan: manifest path: %w", err)
	}
	if workspaceID != input.WorkspaceID {
		return errors.New("compile Plan: workspace ID does not match manifest path")
	}
	boxID, err := identity.Box(input.WorkspaceID, input.TaskKey)
	if err != nil {
		return fmt.Errorf("compile Plan: derive box ID: %w", err)
	}
	if boxID != input.BoxID {
		return errors.New("compile Plan: box ID does not match workspace and task")
	}
	if len(input.Repositories) != 1 {
		return errors.New("compile Plan: exactly one repository fact is required")
	}
	if err := validateRepository(input.Workspace.Spec.Repositories[0], input.Repositories[0]); err != nil {
		return err
	}
	if input.Runtime.Driver != "lima-qemu" && input.Runtime.Driver != "lima-vz" {
		return errors.New("compile Plan: runtime driver must be lima-qemu or lima-vz")
	}
	if err := validateTools(input.Tools); err != nil {
		return err
	}
	return validateWorkspaceProjection(input.Workspace)
}

func validateRepository(spec manifest.Repository, facts repository.Facts) error {
	if facts.Name != spec.Name {
		return errors.New("compile Plan: repository fact name does not match manifest")
	}
	if !facts.Clean {
		return errors.New("compile Plan: repository fact is not clean")
	}
	paths := []struct {
		name string
		path string
	}{
		{name: "source root", path: facts.SourceRoot},
		{name: "worktree path", path: facts.WorktreePath},
		{name: "Git common directory", path: facts.GitStorePath},
	}
	for _, candidate := range paths {
		if candidate.path == "" || !filepath.IsAbs(candidate.path) || filepath.Clean(candidate.path) != candidate.path || strings.IndexByte(candidate.path, 0) >= 0 {
			return fmt.Errorf("compile Plan: repository %s is not a clean absolute path", candidate.name)
		}
	}
	if facts.SourceRoot != spec.CanonicalLocal {
		return errors.New("compile Plan: repository source fact does not match the canonical manifest source")
	}
	if len(facts.BaseCommit) != 40 || strings.Trim(facts.BaseCommit, "0123456789abcdef") != "" {
		return errors.New("compile Plan: repository base commit is not a full lowercase SHA-1 ID")
	}
	if facts.Branch == "" || strings.IndexByte(facts.Branch, 0) >= 0 {
		return errors.New("compile Plan: repository branch is invalid")
	}
	return nil
}

func validateTools(tools contract.ToolVersions) error {
	values := []string{tools.AgentBox, tools.Git, tools.Lima, tools.Docker, tools.Compose}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.IndexByte(value, 0) >= 0 {
			return errors.New("compile Plan: every tool version must be non-empty and NUL-free")
		}
	}
	return nil
}

func validateWorkspaceProjection(workspace *manifest.Workspace) error {
	repositorySpec := workspace.Spec.Repositories[0]
	service := workspace.Spec.WorkspaceService
	if service.Repository != repositorySpec.Name {
		return errors.New("compile Plan: workspace service must use the Milestone 1 repository")
	}
	if service.WorkingDirectory != repositorySpec.WorkspacePath && !strings.HasPrefix(service.WorkingDirectory, repositorySpec.WorkspacePath+"/") {
		return errors.New("compile Plan: workspace service working directory must be inside its repository path")
	}
	mounted := false
	for index, mount := range workspace.Spec.Mounts {
		if mount.Repository != repositorySpec.Name {
			return fmt.Errorf("compile Plan: spec.mounts[%d] does not use the Milestone 1 repository", index)
		}
		if slices.Contains(mount.Services, service.Name) {
			mounted = true
		}
	}
	if !mounted {
		return errors.New("compile Plan: workspace service must receive the repository mount")
	}
	return nil
}

func compileMounts(workspace *manifest.Workspace, facts repository.Facts) []contract.MountPlan {
	type key struct {
		path     string
		writable bool
	}
	services := make(map[key]map[string]struct{})
	for _, mount := range workspace.Spec.Mounts {
		worktreeKey := key{path: facts.WorktreePath, writable: mount.Mode == "read-write"}
		gitKey := key{path: facts.GitStorePath, writable: workspace.Spec.Policy.GitMetadata == "read-write"}
		for _, current := range []key{worktreeKey, gitKey} {
			if services[current] == nil {
				services[current] = make(map[string]struct{})
			}
			for _, service := range mount.Services {
				services[current][service] = struct{}{}
			}
		}
	}
	mounts := make([]contract.MountPlan, 0, len(services))
	for current, names := range services {
		mounts = append(mounts, contract.MountPlan{
			Path: current.path, Services: sortedKeys(names), Writable: current.writable,
		})
	}
	slices.SortFunc(mounts, func(left, right contract.MountPlan) int {
		if compared := strings.Compare(left.Path, right.Path); compared != 0 {
			return compared
		}
		if left.Writable != right.Writable {
			if !left.Writable {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.Join(left.Services, "\x00"), strings.Join(right.Services, "\x00"))
	})
	return mounts
}

func canonicalSet(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return sortedKeys(set)
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func requireContainedPath(root, path, name string) error {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("compile Plan: canonicalize %s: %w", name, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return fmt.Errorf("compile Plan: make %s absolute: %w", name, err)
	}
	canonical = filepath.Clean(canonical)
	if canonical != path {
		return fmt.Errorf("compile Plan: %s must not traverse symlinks", name)
	}
	relative, err := filepath.Rel(root, canonical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("compile Plan: %s escapes repository root", name)
	}
	return nil
}

func readRegularFile(path, name string) ([]byte, error) {
	if err := requireContainedPath(filepath.Dir(path), path, name); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("compile Plan: open %s: %w", name, err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("compile Plan: inspect %s: %w", name, statErr)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("compile Plan: %s is not a regular file", name)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, MaxArtifactBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("compile Plan: read %s: %w", name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("compile Plan: close %s: %w", name, closeErr)
	}
	if int64(len(data)) > MaxArtifactBytes {
		return nil, fmt.Errorf("compile Plan: %s exceeds %d bytes", name, MaxArtifactBytes)
	}
	return data, nil
}

func contentDigest(content []byte) contract.Digest {
	sum := sha256.Sum256(content)
	return contract.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

type digestDocument struct {
	SchemaVersion    string                        `json:"schemaVersion"`
	WorkspaceID      string                        `json:"workspaceId"`
	BoxID            string                        `json:"boxId"`
	TaskKey          string                        `json:"taskKey"`
	Manifest         contract.Artifact             `json:"manifest"`
	Repositories     []contract.PlanRepository     `json:"repositories"`
	RuntimeHost      contract.RuntimeHostPlan      `json:"runtimeHost"`
	Compose          contract.ComposePlan          `json:"compose"`
	WorkspaceService contract.WorkspaceServicePlan `json:"workspaceService"`
	Mounts           []contract.MountPlan          `json:"mounts"`
	Tools            contract.ToolVersions         `json:"tools"`
}

func digestPlan(plan contract.Plan) (contract.Digest, error) {
	document := digestDocument{
		SchemaVersion: plan.SchemaVersion, WorkspaceID: plan.WorkspaceID, BoxID: plan.BoxID,
		TaskKey: plan.TaskKey, Manifest: plan.Manifest, Repositories: plan.Repositories,
		RuntimeHost: plan.RuntimeHost, Compose: plan.Compose, WorkspaceService: plan.WorkspaceService,
		Mounts: plan.Mounts, Tools: plan.Tools,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return contentDigest(encoded), nil
}

func unsupported(path, feature string) error {
	return fmt.Errorf("compile Plan: %w", &manifest.FeatureNotSupportedError{Path: path, Feature: feature})
}
