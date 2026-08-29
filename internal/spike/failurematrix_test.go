package spike

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFailureMatrixOperationIDsAndResidue(t *testing.T) {
	t.Parallel()

	wantIDs := []string{
		"git.private-store.create",
		"git.worktree.create",
		"compose.project.apply",
		"routing.listener.bind",
		"routing.listener.close",
		"compose.project.remove",
		"git.worktree.remove",
		"git.private-store.remove",
	}
	if len(failureOperations) != len(wantIDs) {
		t.Fatalf("failure operation count = %d, want %d", len(failureOperations), len(wantIDs))
	}
	for index, want := range wantIDs {
		if got := failureOperations[index].ID; got != want {
			t.Fatalf("failure operation %d = %q, want %q", index, got, want)
		}
		before := expectedFailureResidue(index, failureBefore)
		after := expectedFailureResidue(index, failureAfter)
		if index == 0 && before != (FailureResidue{}) {
			t.Fatalf("first before residue = %+v, want empty", before)
		}
		if index == len(wantIDs)-1 && after != (FailureResidue{}) {
			t.Fatalf("last after residue = %+v, want empty", after)
		}
	}

	compose := FailureResidue{PrivateStore: true, Worktree: true, WorktreeRegistered: true, ComposeContainers: 1, ComposeNetworks: 1, ComposeVolumes: 1}
	if got := expectedFailureResidue(2, failureAfter); got != compose {
		t.Fatalf("compose after residue = %+v, want %+v", got, compose)
	}
	routing := compose
	routing.RouteReceipt = true
	if got := expectedFailureResidue(3, failureAfter); got != routing {
		t.Fatalf("routing after residue = %+v, want %+v", got, routing)
	}
}

func TestFailureMarkerRefusesMissingOrMismatchedProof(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "owner")
	if err := verifyFailureMarker(path, "owner-a", "fixture"); err == nil {
		t.Fatal("verifyFailureMarker(missing) error = nil, want conflict")
	}
	if err := os.WriteFile(path, []byte("owner-b\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := verifyFailureMarker(path, "owner-a", "fixture"); err == nil {
		t.Fatal("verifyFailureMarker(mismatch) error = nil, want conflict")
	}
	if err := verifyFailureMarker(path, "owner-b", "fixture"); err != nil {
		t.Fatalf("verifyFailureMarker(match) error = %v", err)
	}
}

func TestRemoveFailureMatrixTreeRefusesMismatchedMarker(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), FailureMatrix)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	env := &Environment{root: workspace.Root(), workspace: workspace}
	path, resource, err := createFailureMatrixTree(env)
	if err != nil {
		t.Fatalf("createFailureMatrixTree() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, failureMatrixMarker), []byte("foreign\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}
	if err := removeFailureMatrixTree(workspace, resource); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("removeFailureMatrixTree() error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failure matrix tree mutated after refusal: %v", err)
	}
}
