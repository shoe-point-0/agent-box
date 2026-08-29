package spike

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	FailureMatrix           = "failure-matrix"
	failureMatrixTreeName   = "failure-matrix-cases"
	failureMatrixMarker     = ".abox-spike-failure-owner"
	failureMatrixOwnerLabel = "dev.abox.spike.owner"
	failureMatrixSchema     = "abox.spike.failure-matrix/v1"
)

type failurePhase string

const (
	failureBefore failurePhase = "before"
	failureAfter  failurePhase = "after"
)

type failureOperation struct {
	ID      string
	Cleanup bool
}

var failureOperations = []failureOperation{
	{ID: "git.private-store.create"},
	{ID: "git.worktree.create"},
	{ID: "compose.project.apply"},
	{ID: "routing.listener.bind"},
	{ID: "routing.listener.close", Cleanup: true},
	{ID: "compose.project.remove", Cleanup: true},
	{ID: "git.worktree.remove", Cleanup: true},
	{ID: "git.private-store.remove", Cleanup: true},
}

type FailureMatrixReport struct {
	SchemaVersion string                    `json:"schemaVersion"`
	OS            string                    `json:"os"`
	Architecture  string                    `json:"architecture"`
	Operations    []string                  `json:"operations"`
	Cases         []FailureMatrixCaseResult `json:"cases"`
	Refusals      []FailureRefusalResult    `json:"refusals"`
}

type FailureMatrixCaseResult struct {
	OperationID string         `json:"operationId"`
	Phase       string         `json:"phase"`
	Expected    FailureResidue `json:"expectedResidue"`
	Observed    FailureResidue `json:"observedResidue"`
	Recovery    string         `json:"recovery"`
	Final       FailureResidue `json:"finalResidue"`
}

type FailureRefusalResult struct {
	Class              string `json:"class"`
	ActionableConflict bool   `json:"actionableConflict"`
	ResourcePreserved  bool   `json:"resourcePreserved"`
	CleanupConverged   bool   `json:"cleanupConverged"`
}

type FailureResidue struct {
	PrivateStore       bool `json:"privateStore"`
	Worktree           bool `json:"worktree"`
	WorktreeRegistered bool `json:"worktreeRegistered"`
	ComposeContainers  int  `json:"composeContainers"`
	ComposeNetworks    int  `json:"composeNetworks"`
	ComposeVolumes     int  `json:"composeVolumes"`
	RouteReceipt       bool `json:"routeReceipt"`
	ListenerReachable  bool `json:"listenerReachable"`
}

type failureWorkerConfig struct {
	CaseDir   string       `json:"caseDir"`
	OwnerID   string       `json:"ownerId"`
	Project   string       `json:"project"`
	Image     string       `json:"image"`
	Operation string       `json:"operation,omitempty"`
	Phase     failurePhase `json:"phase,omitempty"`
	Kill      bool         `json:"kill,omitempty"`
}

type failureCase struct {
	config       failureWorkerConfig
	configPath   string
	source       string
	store        string
	worktree     string
	composeFile  string
	routeReceipt string
}

func RegisterFailureMatrix(registry *Registry) error {
	return registry.RegisterScenario(FailureMatrix, Scenario{Run: runFailureMatrix, Cleanup: cleanupFailureMatrix})
}

func runFailureMatrix(ctx context.Context, env *Environment) error {
	if runtime.GOOS != "linux" {
		return errors.New("failure matrix is a Linux implementation probe")
	}
	tree, resource, err := createFailureMatrixTree(env)
	if err != nil {
		return err
	}
	image, err := runText(ctx, env, "failure-matrix-image", []string{"docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", composeBaseImage})
	if err != nil {
		return fmt.Errorf("inspect locally cached failure-matrix image: %w", err)
	}
	image = strings.TrimSpace(image)
	if !strings.Contains(image, "@sha256:") {
		return fmt.Errorf("failure-matrix image is not content-addressed: %q", image)
	}

	report := FailureMatrixReport{SchemaVersion: failureMatrixSchema, OS: runtime.GOOS, Architecture: runtime.GOARCH}
	for _, operation := range failureOperations {
		report.Operations = append(report.Operations, operation.ID)
	}
	caseIndex := 0
	for operationIndex, operation := range failureOperations {
		for _, phase := range []failurePhase{failureBefore, failureAfter} {
			fixture, err := prepareFailureCase(ctx, tree, env.workspace.metadata.ID, image, caseIndex)
			if err != nil {
				return err
			}
			caseIndex++
			fixture.config.Operation = operation.ID
			fixture.config.Phase = phase
			fixture.config.Kill = true
			if err := writeFailureConfig(fixture); err != nil {
				return err
			}
			if err := runKilledFailureWorker(ctx, fixture.configPath); err != nil {
				return fmt.Errorf("kill point %s/%s: %w", operation.ID, phase, err)
			}
			observed, err := observeFailureCase(ctx, fixture)
			if err != nil {
				return fmt.Errorf("observe %s/%s: %w", operation.ID, phase, err)
			}
			expected := expectedFailureResidue(operationIndex, phase)
			if observed != expected {
				return fmt.Errorf("kill point %s/%s residue = %+v, want %+v", operation.ID, phase, observed, expected)
			}
			fixture.config.Kill = false
			fixture.config.Operation = ""
			fixture.config.Phase = ""
			if err := writeFailureConfig(fixture); err != nil {
				return err
			}
			if output, err := runFailureWorkerProcess(ctx, fixture.configPath); err != nil {
				return fmt.Errorf("recover %s/%s: %w: %s", operation.ID, phase, err, output)
			}
			final, err := observeFailureCase(ctx, fixture)
			if err != nil {
				return err
			}
			if final != (FailureResidue{}) {
				return fmt.Errorf("recover %s/%s left residue: %+v", operation.ID, phase, final)
			}
			report.Cases = append(report.Cases, FailureMatrixCaseResult{OperationID: operation.ID, Phase: string(phase), Expected: expected, Observed: observed, Recovery: "converged", Final: final})
		}
	}

	dirty, err := exerciseDirtyWorktreeRefusal(ctx, tree, env.workspace.metadata.ID, image, caseIndex)
	if err != nil {
		return err
	}
	report.Refusals = append(report.Refusals, dirty)
	caseIndex++
	foreign, err := exerciseUnprovedVolumeRefusal(ctx, tree, env.workspace.metadata.ID, image, caseIndex)
	if err != nil {
		return err
	}
	report.Refusals = append(report.Refusals, foreign)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode failure matrix report: %w", err)
	}
	if _, err := env.workspace.WriteDiagnostic("failure-matrix-report.json", append(data, '\n')); err != nil {
		return err
	}
	if err := removeFailureMatrixTree(env.workspace, resource); err != nil {
		return err
	}
	return nil
}

func createFailureMatrixTree(env *Environment) (string, Resource, error) {
	path, err := env.workspace.resolveRelative(failureMatrixTreeName)
	if err != nil {
		return "", Resource{}, err
	}
	resource := Resource{Kind: "failure-matrix-tree", ID: path, OwnerID: env.workspace.metadata.ID, Attributes: map[string]string{"ownerMarker": failureMatrixMarker}}
	if _, err := env.workspace.inventory.Append("planned", resource); err != nil {
		return "", Resource{}, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return "", Resource{}, fmt.Errorf("create failure matrix tree: %w", err)
	}
	if err := writeSyncedFile(filepath.Join(path, failureMatrixMarker), []byte(env.workspace.metadata.ID+"\n"), 0o600); err != nil {
		return "", Resource{}, err
	}
	if _, err := env.workspace.inventory.Append("created", resource); err != nil {
		return "", Resource{}, err
	}
	return path, resource, nil
}

func prepareFailureCase(ctx context.Context, tree, ownerID, image string, index int) (failureCase, error) {
	caseDir := filepath.Join(tree, fmt.Sprintf("case-%02d", index))
	if err := os.Mkdir(caseDir, 0o700); err != nil {
		return failureCase{}, fmt.Errorf("create failure case: %w", err)
	}
	project := fmt.Sprintf("aboxfail%s%02d", ownerID[:10], index)
	fixture := failureCase{
		config:       failureWorkerConfig{CaseDir: caseDir, OwnerID: ownerID, Project: project, Image: image},
		configPath:   filepath.Join(caseDir, "worker.json"),
		source:       filepath.Join(caseDir, "source"),
		store:        filepath.Join(caseDir, "box.git"),
		worktree:     filepath.Join(caseDir, "worktree"),
		composeFile:  filepath.Join(caseDir, "compose.yaml"),
		routeReceipt: filepath.Join(caseDir, "route.addr"),
	}
	if err := os.Mkdir(fixture.source, 0o700); err != nil {
		return failureCase{}, err
	}
	commands := [][]string{
		{"git", "init", "--initial-branch=main", fixture.source},
		{"git", "-C", fixture.source, "config", "user.name", "Agent Box Failure Probe"},
		{"git", "-C", fixture.source, "config", "user.email", "probe@abox.invalid"},
	}
	for _, argv := range commands {
		if output, err := execFailureCommand(ctx, argv...); err != nil {
			return failureCase{}, fmt.Errorf("prepare source: %w: %s", err, output)
		}
	}
	if err := writeSyncedFile(filepath.Join(fixture.source, "README.md"), []byte("failure matrix\n"), 0o600); err != nil {
		return failureCase{}, err
	}
	for _, argv := range [][]string{{"git", "-C", fixture.source, "add", "README.md"}, {"git", "-C", fixture.source, "commit", "-m", "seed"}} {
		if output, err := execFailureCommand(ctx, argv...); err != nil {
			return failureCase{}, fmt.Errorf("seed source: %w: %s", err, output)
		}
	}
	compose := fmt.Sprintf("services:\n  probe:\n    image: %s\n    command: [\"sleep\", \"3600\"]\n    labels:\n      %s: %q\n    volumes:\n      - data:/data\nnetworks:\n  default:\n    labels:\n      %s: %q\nvolumes:\n  data:\n    name: %s-data\n    labels:\n      %s: %q\n", image, failureMatrixOwnerLabel, ownerID, failureMatrixOwnerLabel, ownerID, project, failureMatrixOwnerLabel, ownerID)
	if err := writeSyncedFile(fixture.composeFile, []byte(compose), 0o600); err != nil {
		return failureCase{}, err
	}
	if err := writeFailureConfig(fixture); err != nil {
		return failureCase{}, err
	}
	return fixture, nil
}

func writeFailureConfig(fixture failureCase) error {
	data, err := json.Marshal(fixture.config)
	if err != nil {
		return err
	}
	return writeSyncedFile(fixture.configPath, append(data, '\n'), 0o600)
}

// RunFailureWorker is the private subprocess boundary used to prove real process death.
func RunFailureWorker(ctx context.Context, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read failure worker config: %w", err)
	}
	var config failureWorkerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode failure worker config: %w", err)
	}
	fixture, err := failureCaseFromConfig(config, configPath)
	if err != nil {
		return err
	}
	for _, operation := range failureOperations {
		if config.Kill && config.Operation == operation.ID && config.Phase == failureBefore {
			killFailureWorker(operation.ID, config.Phase)
		}
		if err := applyFailureOperation(ctx, fixture, operation); err != nil {
			return fmt.Errorf("%s: %w", operation.ID, err)
		}
		if config.Kill && config.Operation == operation.ID && config.Phase == failureAfter {
			killFailureWorker(operation.ID, config.Phase)
		}
	}
	return nil
}

func failureCaseFromConfig(config failureWorkerConfig, configPath string) (failureCase, error) {
	caseDir, err := filepath.Abs(config.CaseDir)
	configPath, pathErr := filepath.Abs(configPath)
	if err != nil || pathErr != nil || len(config.OwnerID) < 10 || config.Project == "" || !strings.Contains(config.Image, "@sha256:") || configPath != filepath.Join(caseDir, "worker.json") {
		return failureCase{}, errors.New("invalid failure worker config")
	}
	tree := filepath.Dir(caseDir)
	root := filepath.Dir(tree)
	workspace, err := OpenWorkspace(root)
	if err != nil || workspace.metadata.Scenario != FailureMatrix || workspace.metadata.ID != config.OwnerID || tree != filepath.Join(root, failureMatrixTreeName) || !strings.HasPrefix(filepath.Base(caseDir), "case-") {
		return failureCase{}, errors.New("invalid failure worker ownership boundary")
	}
	if err := verifyFailureMarker(filepath.Join(tree, failureMatrixMarker), config.OwnerID, "failure matrix tree"); err != nil {
		return failureCase{}, err
	}
	if !strings.HasPrefix(config.Project, "aboxfail"+config.OwnerID[:10]) {
		return failureCase{}, errors.New("invalid failure worker project identity")
	}
	if config.Kill && (failureOperationIndex(config.Operation) < 0 || (config.Phase != failureBefore && config.Phase != failureAfter)) {
		return failureCase{}, errors.New("invalid failure worker kill point")
	}
	return failureCase{config: config, configPath: configPath, source: filepath.Join(caseDir, "source"), store: filepath.Join(caseDir, "box.git"), worktree: filepath.Join(caseDir, "worktree"), composeFile: filepath.Join(caseDir, "compose.yaml"), routeReceipt: filepath.Join(caseDir, "route.addr")}, nil
}

func killFailureWorker(operation string, phase failurePhase) {
	receipt := []byte(operation + "\n" + string(phase) + "\n")
	_ = writeSyncedFile(filepath.Join(filepath.Dir(os.Args[len(os.Args)-1]), "kill.receipt"), receipt, 0o600)
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	os.Exit(137)
}

func applyFailureOperation(ctx context.Context, fixture failureCase, operation failureOperation) error {
	switch operation.ID {
	case "git.private-store.create":
		return ensureFailureStore(ctx, fixture)
	case "git.worktree.create":
		return ensureFailureWorktree(ctx, fixture)
	case "compose.project.apply":
		return ensureFailureCompose(ctx, fixture)
	case "routing.listener.bind":
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return err
		}
		failureListeners[fixture.config.Project] = listener
		return writeSyncedFile(fixture.routeReceipt, []byte(listener.Addr().String()+"\n"), 0o600)
	case "routing.listener.close":
		if listener := failureListeners[fixture.config.Project]; listener != nil {
			if err := listener.Close(); err != nil {
				return err
			}
			delete(failureListeners, fixture.config.Project)
		}
		if err := os.Remove(fixture.routeReceipt); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	case "compose.project.remove":
		return removeFailureCompose(ctx, fixture)
	case "git.worktree.remove":
		return removeFailureWorktree(ctx, fixture)
	case "git.private-store.remove":
		return removeFailureStore(fixture)
	default:
		return fmt.Errorf("unknown failure operation %q", operation.ID)
	}
}

var failureListeners = map[string]net.Listener{}

func ensureFailureStore(ctx context.Context, fixture failureCase) error {
	if info, err := os.Stat(fixture.store); err == nil && info.IsDir() {
		return verifyFailureMarker(filepath.Join(fixture.store, failureMatrixMarker), fixture.config.OwnerID, "Git private store")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if output, err := execFailureCommand(ctx, "git", "clone", "--bare", "--no-hardlinks", fixture.source, fixture.store); err != nil {
		return fmt.Errorf("clone private store: %w: %s", err, output)
	}
	return writeSyncedFile(filepath.Join(fixture.store, failureMatrixMarker), []byte(fixture.config.OwnerID+"\n"), 0o600)
}

func ensureFailureWorktree(ctx context.Context, fixture failureCase) error {
	if _, err := os.Stat(fixture.worktree); err == nil {
		registered, err := failureWorktreeRegistered(ctx, fixture)
		if err != nil {
			return err
		}
		if !registered {
			return errors.New("ownership conflict: worktree path exists but is not registered by the private store")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	branchExists, err := failureBranchExists(ctx, fixture)
	if err != nil {
		return err
	}
	worktreeArgs := []string{"git", "-C", fixture.store, "worktree", "add"}
	if branchExists {
		worktreeArgs = append(worktreeArgs, fixture.worktree, "abox/failure")
	} else {
		worktreeArgs = append(worktreeArgs, "-b", "abox/failure", fixture.worktree, "HEAD")
	}
	if output, err := execFailureCommand(ctx, worktreeArgs...); err != nil {
		return fmt.Errorf("create worktree: %w: %s", err, output)
	}
	return nil
}

func failureBranchExists(ctx context.Context, fixture failureCase) (bool, error) {
	_, err := execFailureCommand(ctx, "git", "-C", fixture.store, "show-ref", "--verify", "--quiet", "refs/heads/abox/failure")
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("inspect failure branch: %w", err)
}

func ensureFailureCompose(ctx context.Context, fixture failureCase) error {
	for _, target := range failureComposeTargets(fixture) {
		if exists, owner, err := inspectDockerTarget(ctx, target); err != nil {
			return err
		} else if exists && owner != fixture.config.OwnerID {
			return fmt.Errorf("ownership conflict: %s %s owner %q does not match %q", target.kind, target.id, owner, fixture.config.OwnerID)
		}
	}
	argv := []string{"docker", "compose", "--project-name", fixture.config.Project, "-f", fixture.composeFile, "up", "--detach", "--pull", "never"}
	if output, err := execFailureCommand(ctx, argv...); err != nil {
		return fmt.Errorf("apply Compose project: %w: %s", err, output)
	}
	return nil
}

func removeFailureCompose(ctx context.Context, fixture failureCase) error {
	for _, target := range failureComposeTargets(fixture) {
		if exists, owner, err := inspectDockerTarget(ctx, target); err != nil {
			return err
		} else if exists && owner != fixture.config.OwnerID {
			return fmt.Errorf("ownership conflict: refuse removal of %s %s with owner %q", target.kind, target.id, owner)
		}
	}
	argv := []string{"docker", "compose", "--project-name", fixture.config.Project, "-f", fixture.composeFile, "down", "--volumes", "--remove-orphans"}
	if output, err := execFailureCommand(ctx, argv...); err != nil {
		return fmt.Errorf("remove Compose project: %w: %s", err, output)
	}
	return nil
}

func removeFailureWorktree(ctx context.Context, fixture failureCase) error {
	if _, err := os.Stat(fixture.worktree); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	registered, err := failureWorktreeRegistered(ctx, fixture)
	if err != nil {
		return err
	}
	if !registered {
		return errors.New("ownership conflict: refuse unregistered worktree removal")
	}
	status, err := execFailureCommand(ctx, "git", "-C", fixture.worktree, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("dirty worktree conflict: commit, publish, or remove changes before cleanup")
	}
	if output, err := execFailureCommand(ctx, "git", "-C", fixture.store, "worktree", "remove", fixture.worktree); err != nil {
		return fmt.Errorf("remove worktree: %w: %s", err, output)
	}
	return nil
}

func removeFailureStore(fixture failureCase) error {
	if _, err := os.Stat(fixture.store); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := verifyFailureMarker(filepath.Join(fixture.store, failureMatrixMarker), fixture.config.OwnerID, "Git private store"); err != nil {
		return err
	}
	if err := preflightRemovableTree(fixture.store); err != nil {
		return err
	}
	return os.RemoveAll(fixture.store)
}

type dockerTarget struct{ kind, id string }

func failureComposeTargets(fixture failureCase) []dockerTarget {
	return []dockerTarget{{kind: "container", id: fixture.config.Project + "-probe-1"}, {kind: "network", id: fixture.config.Project + "_default"}, {kind: "volume", id: fixture.config.Project + "-data"}}
}

func inspectDockerTarget(ctx context.Context, target dockerTarget) (bool, string, error) {
	var argv []string
	switch target.kind {
	case "container":
		argv = []string{"docker", "container", "inspect", "--format", "{{index .Config.Labels \"" + failureMatrixOwnerLabel + "\"}}", target.id}
	case "network":
		argv = []string{"docker", "network", "inspect", "--format", "{{index .Labels \"" + failureMatrixOwnerLabel + "\"}}", target.id}
	case "volume":
		argv = []string{"docker", "volume", "inspect", "--format", "{{index .Labels \"" + failureMatrixOwnerLabel + "\"}}", target.id}
	default:
		return false, "", errors.New("unsupported Docker target")
	}
	output, err := execFailureCommand(ctx, argv...)
	if err != nil {
		lowerOutput := strings.ToLower(output)
		if strings.Contains(lowerOutput, "no such") || strings.Contains(lowerOutput, "not found") {
			return false, "", nil
		}
		return false, "", fmt.Errorf("inspect %s %s: %w: %s", target.kind, target.id, err, output)
	}
	return true, strings.TrimSpace(output), nil
}

func failureWorktreeRegistered(ctx context.Context, fixture failureCase) (bool, error) {
	if _, err := os.Stat(fixture.store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	output, err := execFailureCommand(ctx, "git", "-C", fixture.store, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "worktree "+fixture.worktree+"\x00"), nil
}

func observeFailureCase(ctx context.Context, fixture failureCase) (FailureResidue, error) {
	residue := FailureResidue{}
	if info, err := os.Stat(fixture.store); err == nil {
		residue.PrivateStore = info.IsDir()
	} else if !errors.Is(err, os.ErrNotExist) {
		return residue, err
	}
	if info, err := os.Stat(fixture.worktree); err == nil {
		residue.Worktree = info.IsDir()
	} else if !errors.Is(err, os.ErrNotExist) {
		return residue, err
	}
	registered, err := failureWorktreeRegistered(ctx, fixture)
	if err != nil {
		return residue, err
	}
	residue.WorktreeRegistered = registered
	for _, target := range failureComposeTargets(fixture) {
		exists, owner, err := inspectDockerTarget(ctx, target)
		if err != nil {
			return residue, err
		}
		if exists && owner != fixture.config.OwnerID {
			return residue, fmt.Errorf("observed %s %s without ownership proof", target.kind, target.id)
		}
		if exists {
			switch target.kind {
			case "container":
				residue.ComposeContainers++
			case "network":
				residue.ComposeNetworks++
			case "volume":
				residue.ComposeVolumes++
			}
		}
	}
	address, err := os.ReadFile(fixture.routeReceipt)
	if err == nil {
		residue.RouteReceipt = true
		connection, dialErr := net.DialTimeout("tcp4", strings.TrimSpace(string(address)), 100*time.Millisecond)
		if dialErr == nil {
			residue.ListenerReachable = true
			_ = connection.Close()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return residue, err
	}
	return residue, nil
}

func expectedFailureResidue(operationIndex int, phase failurePhase) FailureResidue {
	applied := operationIndex
	if phase == failureAfter {
		applied++
	}
	residue := FailureResidue{}
	if applied >= 1 && applied < 8 {
		residue.PrivateStore = true
	}
	if applied >= 2 && applied < 7 {
		residue.Worktree = true
		residue.WorktreeRegistered = true
	}
	if applied >= 3 && applied < 6 {
		residue.ComposeContainers = 1
		residue.ComposeNetworks = 1
		residue.ComposeVolumes = 1
	}
	if applied >= 4 && applied < 5 {
		residue.RouteReceipt = true
	}
	return residue
}

func runKilledFailureWorker(ctx context.Context, configPath string) error {
	output, err := runFailureWorkerProcess(ctx, configPath)
	if err == nil {
		return errors.New("worker exited successfully instead of being killed")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return fmt.Errorf("worker did not reach SIGKILL: %w: %s", err, output)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		return fmt.Errorf("worker exit was %v, want SIGKILL: %s", exitErr, output)
	}
	configData, readErr := os.ReadFile(configPath)
	if readErr != nil {
		return fmt.Errorf("read killed worker config: %w", readErr)
	}
	var config failureWorkerConfig
	if decodeErr := json.Unmarshal(configData, &config); decodeErr != nil {
		return fmt.Errorf("decode killed worker config: %w", decodeErr)
	}
	receipt, readErr := os.ReadFile(filepath.Join(filepath.Dir(configPath), "kill.receipt"))
	if readErr != nil {
		return fmt.Errorf("read kill-point receipt: %w", readErr)
	}
	wantReceipt := config.Operation + "\n" + string(config.Phase) + "\n"
	if string(receipt) != wantReceipt {
		return fmt.Errorf("kill-point receipt = %q, want %q", receipt, wantReceipt)
	}
	return nil
}

func runFailureWorkerProcess(ctx context.Context, configPath string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, executable, "__failure-worker", configPath)
	output, err := command.CombinedOutput()
	return string(output), err
}

func execFailureCommand(ctx context.Context, argv ...string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("empty command")
	}
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	return string(output), err
}

func failureOperationIndex(id string) int {
	for index, operation := range failureOperations {
		if operation.ID == id {
			return index
		}
	}
	return -1
}

func verifyFailureMarker(path, ownerID, kind string) error {
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != ownerID {
		return fmt.Errorf("ownership conflict: %s marker does not prove owner %q", kind, ownerID)
	}
	return nil
}

func exerciseDirtyWorktreeRefusal(ctx context.Context, tree, ownerID, image string, index int) (FailureRefusalResult, error) {
	fixture, err := prepareFailureCase(ctx, tree, ownerID, image, index)
	if err != nil {
		return FailureRefusalResult{}, err
	}
	fixture.config.Kill = true
	fixture.config.Operation = "compose.project.apply"
	fixture.config.Phase = failureAfter
	if err := writeFailureConfig(fixture); err != nil {
		return FailureRefusalResult{}, err
	}
	if err := runKilledFailureWorker(ctx, fixture.configPath); err != nil {
		return FailureRefusalResult{}, err
	}
	dirtyPath := filepath.Join(fixture.worktree, "dirty-user-work.txt")
	const dirtyContent = "must survive refusal\n"
	if err := writeSyncedFile(dirtyPath, []byte(dirtyContent), 0o600); err != nil {
		return FailureRefusalResult{}, err
	}
	fixture.config.Kill = false
	fixture.config.Operation = ""
	fixture.config.Phase = ""
	if err := writeFailureConfig(fixture); err != nil {
		return FailureRefusalResult{}, err
	}
	output, runErr := runFailureWorkerProcess(ctx, fixture.configPath)
	conflict := runErr != nil && strings.Contains(output, "dirty worktree conflict")
	data, readErr := os.ReadFile(dirtyPath)
	preserved := readErr == nil && string(data) == dirtyContent
	if !conflict || !preserved {
		return FailureRefusalResult{}, fmt.Errorf("dirty refusal conflict=%t preserved=%t output=%q", conflict, preserved, output)
	}
	if err := os.Remove(dirtyPath); err != nil {
		return FailureRefusalResult{}, err
	}
	if output, err := runFailureWorkerProcess(ctx, fixture.configPath); err != nil {
		return FailureRefusalResult{}, fmt.Errorf("dirty refusal cleanup: %w: %s", err, output)
	}
	final, err := observeFailureCase(ctx, fixture)
	if err != nil {
		return FailureRefusalResult{}, err
	}
	return FailureRefusalResult{Class: "dirty-worktree", ActionableConflict: conflict, ResourcePreserved: preserved, CleanupConverged: final == (FailureResidue{})}, nil
}

func exerciseUnprovedVolumeRefusal(ctx context.Context, tree, ownerID, image string, index int) (FailureRefusalResult, error) {
	fixture, err := prepareFailureCase(ctx, tree, ownerID, image, index)
	if err != nil {
		return FailureRefusalResult{}, err
	}
	volume := fixture.config.Project + "-data"
	if output, err := execFailureCommand(ctx, "docker", "volume", "create", "--label", failureMatrixOwnerLabel+"=foreign-owner", volume); err != nil {
		return FailureRefusalResult{}, fmt.Errorf("create foreign volume fixture: %w: %s", err, output)
	}
	output, runErr := runFailureWorkerProcess(ctx, fixture.configPath)
	conflict := runErr != nil && strings.Contains(output, "ownership conflict") && strings.Contains(output, volume)
	exists, owner, inspectErr := inspectDockerTarget(ctx, dockerTarget{kind: "volume", id: volume})
	preserved := inspectErr == nil && exists && owner == "foreign-owner"
	if !conflict || !preserved {
		return FailureRefusalResult{}, fmt.Errorf("unproved-volume refusal conflict=%t preserved=%t output=%q inspect=%v", conflict, preserved, output, inspectErr)
	}
	if output, err := execFailureCommand(ctx, "docker", "volume", "rm", volume); err != nil {
		return FailureRefusalResult{}, fmt.Errorf("remove parent-owned foreign-volume fixture: %w: %s", err, output)
	}
	if output, err := runFailureWorkerProcess(ctx, fixture.configPath); err != nil {
		return FailureRefusalResult{}, fmt.Errorf("unproved volume cleanup: %w: %s", err, output)
	}
	final, err := observeFailureCase(ctx, fixture)
	if err != nil {
		return FailureRefusalResult{}, err
	}
	return FailureRefusalResult{Class: "resource-without-ownership-proof", ActionableConflict: conflict, ResourcePreserved: preserved, CleanupConverged: final == (FailureResidue{})}, nil
}

func cleanupFailureMatrix(ctx context.Context, workspace *Workspace, resources []Resource) error {
	for _, resource := range resources {
		if resource.Kind == "failure-matrix-tree" {
			if err := cleanupFailureMatrixCases(ctx, workspace, resource); err != nil {
				return err
			}
			return removeFailureMatrixTree(workspace, resource)
		}
	}
	return nil
}

func cleanupFailureMatrixCases(ctx context.Context, workspace *Workspace, resource Resource) error {
	if _, err := os.Stat(resource.ID); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := verifyFailureMarker(filepath.Join(resource.ID, failureMatrixMarker), workspace.metadata.ID, "failure matrix tree"); err != nil {
		return fmt.Errorf("%w: %v", ErrCleanupRefused, err)
	}
	entries, err := os.ReadDir(resource.ID)
	if err != nil {
		return err
	}
	fixtures := make([]failureCase, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == failureMatrixMarker {
			continue
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "case-") {
			return fmt.Errorf("%w: unknown failure matrix entry %s", ErrCleanupRefused, entry.Name())
		}
		configPath := filepath.Join(resource.ID, entry.Name(), "worker.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("%w: read failure case config: %v", ErrCleanupRefused, err)
		}
		var config failureWorkerConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("%w: decode failure case config: %v", ErrCleanupRefused, err)
		}
		fixture, err := failureCaseFromConfig(config, configPath)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCleanupRefused, err)
		}
		if err := preflightFailureCaseCleanup(ctx, fixture); err != nil {
			return err
		}
		fixtures = append(fixtures, fixture)
	}
	for _, fixture := range fixtures {
		if err := os.Remove(fixture.routeReceipt); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := removeFailureCompose(ctx, fixture); err != nil {
			return err
		}
		if err := removeFailureWorktree(ctx, fixture); err != nil {
			return err
		}
		if err := removeFailureStore(fixture); err != nil {
			return err
		}
	}
	return nil
}

func preflightFailureCaseCleanup(ctx context.Context, fixture failureCase) error {
	for _, target := range failureComposeTargets(fixture) {
		if exists, owner, err := inspectDockerTarget(ctx, target); err != nil {
			return err
		} else if exists && owner != fixture.config.OwnerID {
			return fmt.Errorf("%w: %s %s lacks ownership proof", ErrCleanupRefused, target.kind, target.id)
		}
	}
	if _, err := os.Stat(fixture.worktree); err == nil {
		registered, err := failureWorktreeRegistered(ctx, fixture)
		if err != nil || !registered {
			return fmt.Errorf("%w: worktree registration mismatch", ErrCleanupRefused)
		}
		status, err := execFailureCommand(ctx, "git", "-C", fixture.worktree, "status", "--porcelain=v2", "-z", "--untracked-files=all")
		if err != nil {
			return err
		}
		if status != "" {
			return fmt.Errorf("%w: dirty worktree conflict", ErrCleanupRefused)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(fixture.store); err == nil {
		if err := verifyFailureMarker(filepath.Join(fixture.store, failureMatrixMarker), fixture.config.OwnerID, "Git private store"); err != nil {
			return fmt.Errorf("%w: %v", ErrCleanupRefused, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeFailureMatrixTree(workspace *Workspace, resource Resource) error {
	if resource.OwnerID != workspace.metadata.ID || filepath.Base(resource.ID) != failureMatrixTreeName || !isWithin(workspace.Root(), resource.ID) {
		return fmt.Errorf("%w: failure matrix tree identity mismatch", ErrCleanupRefused)
	}
	if _, err := os.Stat(resource.ID); errors.Is(err, os.ErrNotExist) {
		return workspace.MarkRemoved(resource)
	} else if err != nil {
		return err
	}
	if err := verifyFailureMarker(filepath.Join(resource.ID, failureMatrixMarker), workspace.metadata.ID, "failure matrix tree"); err != nil {
		return fmt.Errorf("%w: %v", ErrCleanupRefused, err)
	}
	if err := preflightRemovableTree(resource.ID); err != nil {
		return err
	}
	if err := os.RemoveAll(resource.ID); err != nil {
		return err
	}
	return workspace.MarkRemoved(resource)
}
