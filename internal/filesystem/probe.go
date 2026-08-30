package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

const (
	probeDirectory  = ".abox-probe"
	probeNonceBytes = 16
)

// ProbeFile is the bounded workspace-service view of one probe file. Data must
// only contain caller-generated probe bytes; implementations must not return
// workload file contents through this boundary.
type ProbeFile struct {
	Exists  bool
	Regular bool
	Mode    fs.FileMode
	Data    []byte
}

// WorkspaceProbe performs fixed filesystem operations in one immutable
// workspace-container ID. Implementations must not address containers by name.
type WorkspaceProbe interface {
	Observe(context.Context, string, string) (ProbeFile, error)
	Write(context.Context, string, string, []byte, fs.FileMode) error
	Rename(context.Context, string, string, string) error
	Remove(context.Context, string, string) error
}

// ProbeVerifier checks bidirectional bytes, modes, atomic rename visibility,
// and deletion visibility under a nonce-scoped directory in every worktree.
// Every created nonce is removed on all return paths.
type ProbeVerifier struct {
	workspace WorkspaceProbe
	random    func([]byte) error
}

func NewProbeVerifier(workspace WorkspaceProbe) (*ProbeVerifier, error) {
	if workspace == nil {
		return nil, failure(InvalidObservation, "probe", "workspace boundary is nil")
	}
	return &ProbeVerifier{
		workspace: workspace,
		random: func(data []byte) error {
			_, err := rand.Read(data)
			return err
		},
	}, nil
}

// Verify runs the probe against the workspace container from a fresh Docker
// observation. It rejects incomplete Plan projections before creating files.
func (v *ProbeVerifier) Verify(ctx context.Context, plan contract.Plan, observed compose.RuntimeObservation) error {
	containerID, worktrees, err := probeInputs(plan, observed)
	if err != nil {
		return failure(InvalidObservation, "probe", err.Error())
	}
	for _, worktree := range worktrees {
		if err := ctx.Err(); err != nil {
			return failure(ProbeFailed, "probe", "filesystem probe was canceled")
		}
		if err := v.verifyWorktree(ctx, containerID, worktree); err != nil {
			return err
		}
	}
	return nil
}

func probeInputs(plan contract.Plan, observed compose.RuntimeObservation) (string, []string, error) {
	service := plan.WorkspaceService.Name
	if service == "" || len(plan.Repositories) == 0 {
		return "", nil, errors.New("workspace service or repository set is empty")
	}
	containerID := ""
	for _, container := range observed.Containers {
		if container.Service != service {
			continue
		}
		if containerID != "" || !safeContainerID(container.ID) {
			return "", nil, errors.New("workspace container identity is ambiguous or invalid")
		}
		containerID = container.ID
	}
	if containerID == "" {
		return "", nil, errors.New("workspace container is absent")
	}

	mounts := make(map[string]contract.MountPlan, len(plan.Mounts))
	for _, mount := range plan.Mounts {
		if _, duplicate := mounts[mount.Path]; duplicate {
			return "", nil, errors.New("mount set contains a duplicate path")
		}
		mounts[mount.Path] = mount
	}
	worktrees := make([]string, 0, len(plan.Repositories))
	seen := make(map[string]struct{}, len(plan.Repositories))
	for _, repository := range plan.Repositories {
		path := repository.WorktreePath
		if !cleanAbsolute(path) {
			return "", nil, errors.New("worktree path is invalid")
		}
		if _, duplicate := seen[path]; duplicate {
			return "", nil, errors.New("worktree set contains a duplicate path")
		}
		seen[path] = struct{}{}
		mount, exists := mounts[path]
		if !exists || !mount.Writable || !slices.Contains(mount.Services, service) {
			return "", nil, errors.New("worktree is not writable in the workspace service")
		}
		worktrees = append(worktrees, path)
	}
	slices.Sort(worktrees)
	return containerID, worktrees, nil
}

func (v *ProbeVerifier) verifyWorktree(ctx context.Context, containerID, worktree string) (result error) {
	worktreeInfo, err := os.Lstat(worktree)
	if err != nil || !worktreeInfo.IsDir() || worktreeInfo.Mode()&fs.ModeSymlink != 0 {
		return failure(ProbeFailed, "probe-create", "worktree cannot be probed safely")
	}
	nonce, err := v.nonce()
	if err != nil {
		return failure(ProbeFailed, "probe-create", "probe nonce cannot be generated")
	}
	parent := filepath.Join(worktree, probeDirectory)
	createdParent, err := ensureProbeParent(parent)
	if err != nil {
		return failure(ProbeFailed, "probe-create", "probe directory cannot be created safely")
	}
	root := filepath.Join(parent, nonce)
	if err := os.Mkdir(root, 0o700); err != nil {
		if createdParent {
			_ = os.Remove(parent)
		}
		return failure(ProbeFailed, "probe-create", "probe directory cannot be created safely")
	}
	defer func() {
		if err := cleanupProbe(root, parent, createdParent); err != nil {
			result = failure(ProbeFailed, "probe-cleanup", "probe cleanup failed")
		}
	}()

	machineBytes := []byte("abox-machine-probe-" + nonce)
	workspaceBytes := []byte("abox-workspace-probe-" + nonce)
	if err := v.machineToWorkspace(ctx, containerID, root, machineBytes); err != nil {
		return err
	}
	return v.workspaceToMachine(ctx, containerID, root, workspaceBytes)
}

func (v *ProbeVerifier) machineToWorkspace(ctx context.Context, containerID, root string, data []byte) error {
	source := filepath.Join(root, "machine-source")
	target := filepath.Join(root, "machine-target")
	if err := writeProbeFile(source, data, 0o600); err != nil {
		return probeFailure(ctx, "machine-write", err)
	}
	observed, err := v.workspace.Observe(ctx, containerID, source)
	if err != nil || !matchesProbe(observed, data, 0o600) {
		return probeFailure(ctx, "workspace-read", err)
	}
	if err := os.Rename(source, target); err != nil {
		return probeFailure(ctx, "machine-rename", err)
	}
	if err := v.requireWorkspaceRename(ctx, containerID, source, target, data, 0o600); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		return probeFailure(ctx, "machine-delete", err)
	}
	observed, err = v.workspace.Observe(ctx, containerID, target)
	if err != nil || observed.Exists {
		return probeFailure(ctx, "workspace-delete-visibility", err)
	}
	return nil
}

func (v *ProbeVerifier) workspaceToMachine(ctx context.Context, containerID, root string, data []byte) error {
	source := filepath.Join(root, "workspace-source")
	target := filepath.Join(root, "workspace-target")
	if err := v.workspace.Write(ctx, containerID, source, data, 0o640); err != nil {
		return probeFailure(ctx, "workspace-write", err)
	}
	observed, err := observeMachine(source)
	if err != nil || !matchesProbe(observed, data, 0o640) {
		return probeFailure(ctx, "machine-read", err)
	}
	if err := v.workspace.Rename(ctx, containerID, source, target); err != nil {
		return probeFailure(ctx, "workspace-rename", err)
	}
	sourceObserved, sourceErr := observeMachine(source)
	targetObserved, targetErr := observeMachine(target)
	if sourceErr != nil || targetErr != nil || sourceObserved.Exists || !matchesProbe(targetObserved, data, 0o640) {
		return probeFailure(ctx, "machine-rename-visibility", errors.Join(sourceErr, targetErr))
	}
	if err := v.workspace.Remove(ctx, containerID, target); err != nil {
		return probeFailure(ctx, "workspace-delete", err)
	}
	targetObserved, err = observeMachine(target)
	if err != nil || targetObserved.Exists {
		return probeFailure(ctx, "machine-delete-visibility", err)
	}
	return nil
}

func (v *ProbeVerifier) requireWorkspaceRename(ctx context.Context, containerID, source, target string, data []byte, mode fs.FileMode) error {
	sourceObserved, sourceErr := v.workspace.Observe(ctx, containerID, source)
	targetObserved, targetErr := v.workspace.Observe(ctx, containerID, target)
	if sourceErr != nil || targetErr != nil || sourceObserved.Exists || !matchesProbe(targetObserved, data, mode) {
		return probeFailure(ctx, "workspace-rename-visibility", errors.Join(sourceErr, targetErr))
	}
	return nil
}

func (v *ProbeVerifier) nonce() (string, error) {
	data := make([]byte, probeNonceBytes)
	if err := v.random(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func ensureProbeParent(path string) (bool, error) {
	err := os.Mkdir(path, 0o700)
	created := err == nil
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false, errors.New("probe parent is not a directory")
	}
	return created, nil
}

func cleanupProbe(root, parent string, createdParent bool) error {
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if createdParent {
		if err := os.Remove(parent); err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
			return err
		}
	}
	return nil
}

func writeProbeFile(path string, data []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || written != len(data) {
		return errors.Join(writeErr, closeErr, fmt.Errorf("short probe write"))
	}
	return os.Chmod(path, mode)
}

func observeMachine(path string) (ProbeFile, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ProbeFile{}, nil
	}
	if err != nil {
		return ProbeFile{}, err
	}
	observed := ProbeFile{Exists: true, Regular: info.Mode().IsRegular(), Mode: info.Mode()}
	if !observed.Regular {
		return observed, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ProbeFile{}, err
	}
	observed.Data = data
	return observed, nil
}

func matchesProbe(observed ProbeFile, data []byte, mode fs.FileMode) bool {
	return observed.Exists && observed.Regular && observed.Mode.Perm() == mode.Perm() && slices.Equal(observed.Data, data)
}

func probeFailure(ctx context.Context, layer string, _ error) error {
	if ctx.Err() != nil {
		return failure(ProbeFailed, layer, "filesystem probe was canceled")
	}
	return failure(ProbeFailed, layer, "filesystem probe failed")
}
