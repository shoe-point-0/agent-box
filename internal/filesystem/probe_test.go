package filesystem_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
)

func TestProbeVerifierChecksBothDirectionsAndCleansEveryWorktree(t *testing.T) {
	root := t.TempDir()
	worktrees := []string{filepath.Join(root, "zeta"), filepath.Join(root, "alpha")}
	for _, worktree := range worktrees {
		if err := os.Mkdir(worktree, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := &sharedWorkspace{}
	verifier, err := filesystem.NewProbeVerifier(workspace)
	if err != nil {
		t.Fatal(err)
	}

	if err := verifier.Verify(t.Context(), probePlan(worktrees, true), workspaceObservation("immutable-container-id")); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(workspace.calls) != 16 {
		t.Fatalf("workspace calls = %d, want 16", len(workspace.calls))
	}
	for _, call := range workspace.calls {
		if call.containerID != "immutable-container-id" {
			t.Fatalf("workspace call used container ID %q", call.containerID)
		}
		if !strings.Contains(call.path, string(filepath.Separator)+".abox-probe"+string(filepath.Separator)) {
			t.Fatalf("workspace call escaped probe directory: %#v", call)
		}
	}
	if !strings.HasPrefix(workspace.calls[0].path, worktrees[1]+string(filepath.Separator)) {
		t.Fatalf("first call path = %q, want sorted alpha worktree first", workspace.calls[0].path)
	}
	for _, worktree := range worktrees {
		if _, err := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("probe directory remains in %q: %v", worktree, err)
		}
	}
}

func TestProbeVerifierPreservesPreexistingProbeParent(t *testing.T) {
	worktree := t.TempDir()
	parent := filepath.Join(worktree, ".abox-probe")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(parent, "owned-by-concurrent-probe")
	if err := os.WriteFile(sentinel, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	verifier, err := filesystem.NewProbeVerifier(&sharedWorkspace{})
	if err != nil {
		t.Fatal(err)
	}

	if err := verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation("container-1")); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(sentinel) {
		t.Fatalf("preexisting probe parent changed: %v", entries)
	}
}

func TestProbeVerifierCleansNonceAndRedactsEveryWorkspaceFailure(t *testing.T) {
	tests := []struct {
		operation string
		layer     string
	}{
		{operation: "observe", layer: "workspace-read"},
		{operation: "write", layer: "workspace-write"},
		{operation: "rename", layer: "workspace-rename"},
		{operation: "remove", layer: "workspace-delete"},
		{operation: "watch", layer: "workspace-watch-setup"},
	}
	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			worktree := t.TempDir()
			secret := "secret-workload-error"
			workspace := &sharedWorkspace{failOperation: test.operation, err: errors.New(secret)}
			verifier, err := filesystem.NewProbeVerifier(workspace)
			if err != nil {
				t.Fatal(err)
			}

			err = verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation("container-1"))
			assertError(t, err, filesystem.ProbeFailed, test.layer)
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), worktree) {
				t.Fatalf("probe error leaked runtime data: %v", err)
			}
			if _, statErr := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("probe residue remains after failure: %v", statErr)
			}
		})
	}
}

func TestProbeVerifierRejectsVisibilityMismatchAndCleansNonce(t *testing.T) {
	worktree := t.TempDir()
	workspace := &sharedWorkspace{corruptObservations: true}
	verifier, err := filesystem.NewProbeVerifier(workspace)
	if err != nil {
		t.Fatal(err)
	}

	err = verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation("container-1"))
	assertError(t, err, filesystem.ProbeFailed, "workspace-read")
	if _, statErr := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("probe residue remains after mismatch: %v", statErr)
	}
}

func TestProbeVerifierClassifiesCancellationAndCleansNonce(t *testing.T) {
	worktree := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	workspace := &sharedWorkspace{cancel: cancel, cancelOperation: "write"}
	verifier, err := filesystem.NewProbeVerifier(workspace)
	if err != nil {
		t.Fatal(err)
	}

	err = verifier.Verify(ctx, probePlan([]string{worktree}, true), workspaceObservation("container-1"))
	assertError(t, err, filesystem.ProbeFailed, "workspace-write")
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("probe residue remains after cancellation: %v", statErr)
	}
}

func TestProbeVerifierRejectsInvalidProjectionBeforeMutation(t *testing.T) {
	tests := []struct {
		name     string
		plan     func(string) contract.Plan
		observed compose.RuntimeObservation
	}{
		{
			name: "missing workspace container",
			plan: func(path string) contract.Plan { return probePlan([]string{path}, true) },
		},
		{
			name: "duplicate workspace container",
			plan: func(path string) contract.Plan { return probePlan([]string{path}, true) },
			observed: compose.RuntimeObservation{Containers: []compose.ContainerObservation{
				{ID: "one", Service: "workspace"}, {ID: "two", Service: "workspace"},
			}},
		},
		{
			name:     "read-only worktree",
			plan:     func(path string) contract.Plan { return probePlan([]string{path}, false) },
			observed: workspaceObservation("container-1"),
		},
		{
			name:     "duplicate worktree",
			plan:     func(path string) contract.Plan { return probePlan([]string{path, path}, true) },
			observed: workspaceObservation("container-1"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worktree := t.TempDir()
			workspace := &sharedWorkspace{}
			verifier, err := filesystem.NewProbeVerifier(workspace)
			if err != nil {
				t.Fatal(err)
			}
			err = verifier.Verify(t.Context(), test.plan(worktree), test.observed)
			assertError(t, err, filesystem.InvalidObservation, "probe")
			if len(workspace.calls) != 0 {
				t.Fatalf("workspace calls = %#v, want none", workspace.calls)
			}
			if _, statErr := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("invalid projection created probe state: %v", statErr)
			}
		})
	}
}

func TestProbeVerifierRefusesSymlinkWorktreeWithoutTouchingTarget(t *testing.T) {
	target := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")
	if err := os.Symlink(target, worktree); err != nil {
		t.Fatal(err)
	}
	workspace := &sharedWorkspace{}
	verifier, err := filesystem.NewProbeVerifier(workspace)
	if err != nil {
		t.Fatal(err)
	}

	err = verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation("container-1"))
	assertError(t, err, filesystem.ProbeFailed, "probe-create")
	if len(workspace.calls) != 0 {
		t.Fatalf("workspace calls = %#v, want none", workspace.calls)
	}
	if _, statErr := os.Lstat(filepath.Join(target, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("symlink target was changed: %v", statErr)
	}
}

func TestNewProbeVerifierRejectsNilBoundary(t *testing.T) {
	if _, err := filesystem.NewProbeVerifier(nil); err == nil {
		t.Fatal("NewProbeVerifier(nil) succeeded")
	}
}

func probePlan(worktrees []string, writable bool) contract.Plan {
	plan := contract.Plan{WorkspaceService: contract.WorkspaceServicePlan{Name: "workspace"}}
	for index, worktree := range worktrees {
		plan.Repositories = append(plan.Repositories, contract.PlanRepository{
			Name: "repository-" + string(rune('a'+index)), WorktreePath: worktree,
		})
		plan.Mounts = append(plan.Mounts, contract.MountPlan{
			Path: worktree, Services: []string{"workspace"}, Writable: writable,
		})
	}
	return plan
}

func workspaceObservation(id string) compose.RuntimeObservation {
	return compose.RuntimeObservation{Containers: []compose.ContainerObservation{{ID: id, Service: "workspace"}}}
}

type workspaceCall struct {
	operation   string
	containerID string
	path        string
}

type sharedWorkspace struct {
	calls               []workspaceCall
	failOperation       string
	err                 error
	corruptObservations bool
	cancel              context.CancelFunc
	cancelOperation     string
}

func (w *sharedWorkspace) Observe(_ context.Context, containerID, path string) (filesystem.ProbeFile, error) {
	w.calls = append(w.calls, workspaceCall{operation: "observe", containerID: containerID, path: path})
	if w.failOperation == "observe" {
		return filesystem.ProbeFile{}, w.err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return filesystem.ProbeFile{}, nil
	}
	if err != nil {
		return filesystem.ProbeFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return filesystem.ProbeFile{}, err
	}
	if w.corruptObservations {
		data = []byte("corrupt")
	}
	return filesystem.ProbeFile{Exists: true, Regular: info.Mode().IsRegular(), Mode: info.Mode(), Data: data}, nil
}

func (w *sharedWorkspace) Write(_ context.Context, containerID, path string, data []byte, mode fs.FileMode) error {
	w.calls = append(w.calls, workspaceCall{operation: "write", containerID: containerID, path: path})
	if w.cancelOperation == "write" {
		w.cancel()
		return context.Canceled
	}
	if w.failOperation == "write" {
		return w.err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func (w *sharedWorkspace) Rename(_ context.Context, containerID, oldPath, newPath string) error {
	w.calls = append(w.calls, workspaceCall{operation: "rename", containerID: containerID, path: oldPath})
	if w.failOperation == "rename" {
		return w.err
	}
	return os.Rename(oldPath, newPath)
}

func (w *sharedWorkspace) Remove(_ context.Context, containerID, path string) error {
	w.calls = append(w.calls, workspaceCall{operation: "remove", containerID: containerID, path: path})
	if w.failOperation == "remove" {
		return w.err
	}
	return os.Remove(path)
}

func (w *sharedWorkspace) Watch(ctx context.Context, containerID, target, ready string) error {
	w.calls = append(w.calls, workspaceCall{operation: "watch", containerID: containerID, path: target})
	if w.failOperation == "watch" {
		return w.err
	}
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := os.Lstat(target); err == nil {
				return nil
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
}

// Model name and attribute caches independently of mutations. The first two
// observations of each state are stale; subsequent observations are exact.
type cachedWorkspace struct {
	*sharedWorkspace
	observations map[string]int
	readRace     bool
}

func (w *cachedWorkspace) Observe(ctx context.Context, id, path string) (filesystem.ProbeFile, error) {
	actual, err := w.sharedWorkspace.Observe(ctx, id, path)
	if err != nil {
		return actual, err
	}
	key := path + strconv.FormatBool(actual.Exists)
	w.observations[key]++
	if w.observations[key] <= 2 {
		if actual.Exists {
			return filesystem.ProbeFile{}, nil
		}
		if w.readRace {
			return filesystem.ProbeFile{Exists: true}, nil
		}
		return filesystem.ProbeFile{Exists: true, Regular: true, Mode: 0o600, Data: []byte("stale")}, nil
	}
	return actual, nil
}
func TestProbeVerifierWaitsForVisibilityWithoutRepeatingMutations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readRace bool
	}{
		{name: "stale regular cache", readRace: false},
		{name: "stale stat read race", readRace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worktree := t.TempDir()
			workspace := &cachedWorkspace{sharedWorkspace: &sharedWorkspace{}, observations: map[string]int{}, readRace: tc.readRace}
			verifier, err := filesystem.NewProbeVerifier(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation("container-1")); err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, call := range workspace.calls {
				counts[call.operation]++
			}
			for _, operation := range []string{"write", "rename", "remove", "watch"} {
				if counts[operation] != 1 {
					t.Fatalf("%s repeated: %d", operation, counts[operation])
				}
			}
			if _, err := os.Stat(filepath.Join(worktree, ".abox-probe")); !os.IsNotExist(err) {
				t.Fatalf("probe residue: %v", err)
			}
		})
	}
}

type unreadableWorkspace struct {
	*sharedWorkspace
	blockDelete bool
}

func (w *unreadableWorkspace) Observe(ctx context.Context, id, path string) (filesystem.ProbeFile, error) {
	actual, err := w.sharedWorkspace.Observe(ctx, id, path)
	if err != nil {
		return actual, err
	}
	if w.blockDelete && !actual.Exists && strings.HasSuffix(path, "machine-target") {
		return filesystem.ProbeFile{Exists: true}, nil
	}
	if !w.blockDelete && actual.Exists {
		return filesystem.ProbeFile{Exists: true}, nil
	}
	return actual, nil
}

func TestProbeVerifierRefusesPermanentUnreadableObservation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		blockDelete bool
		wantLayer   string
	}{
		{name: "unreadable read", blockDelete: false, wantLayer: "workspace-read"},
		{name: "unreadable delete", blockDelete: true, wantLayer: "workspace-delete-visibility"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worktree := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			defer cancel()
			verifier, err := filesystem.NewProbeVerifier(&unreadableWorkspace{sharedWorkspace: &sharedWorkspace{}, blockDelete: tc.blockDelete})
			if err != nil {
				t.Fatal(err)
			}
			err = verifier.Verify(ctx, probePlan([]string{worktree}, true), workspaceObservation("container-1"))
			assertError(t, err, filesystem.ProbeFailed, tc.wantLayer)
			if _, statErr := os.Lstat(filepath.Join(worktree, ".abox-probe")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("probe residue remains: %v", statErr)
			}
		})
	}
}
func TestProbeVisibilityWaitHonorsCancellation(t *testing.T) {
	worktree := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	verifier, err := filesystem.NewProbeVerifier(&sharedWorkspace{corruptObservations: true})
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.Verify(ctx, probePlan([]string{worktree}, true), workspaceObservation("container-1"))
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("visibility wait did not honor cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".abox-probe")); !os.IsNotExist(err) {
		t.Fatalf("probe residue: %v", err)
	}
}
