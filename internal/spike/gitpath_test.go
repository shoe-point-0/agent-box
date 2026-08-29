package spike

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGitPathCleanupRequiresMatchingOwnershipMarker(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), GitPathProbe)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace}
	tree, err := createOwnedTree(environment, gitPathTreeName)
	if err != nil {
		t.Fatalf("createOwnedTree() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "fixture"), []byte("owned"), 0o600); err != nil {
		t.Fatalf("WriteFile(fixture) error = %v", err)
	}
	entries, err := workspace.Entries()
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	resource := entries[len(entries)-1].Resource
	if err := os.WriteFile(filepath.Join(tree, gitTreeMarker), []byte("wrong-owner\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}
	if err := cleanupGitPathProbe(context.Background(), workspace, []Resource{resource}); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("cleanupGitPathProbe(wrong marker) error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(tree); err != nil {
		t.Fatalf("tree removed after ownership refusal: %v", err)
	}
}

func TestGitPathCleanupMarksTreeRemovedBeforeGenericCleanup(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), GitPathProbe)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace}
	tree, err := createOwnedTree(environment, gitPathTreeName)
	if err != nil {
		t.Fatalf("createOwnedTree() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "fixture"), []byte("owned"), 0o600); err != nil {
		t.Fatalf("WriteFile(fixture) error = %v", err)
	}
	entries, err := workspace.Entries()
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	resource := entries[len(entries)-1].Resource
	if err := cleanupGitPathProbe(context.Background(), workspace, []Resource{resource}); err != nil {
		t.Fatalf("cleanupGitPathProbe() error = %v", err)
	}
	if _, err := os.Stat(tree); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(cleaned tree) error = %v, want not exist", err)
	}
	if err := CleanupWorkspace(workspace.Root()); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
}

func TestGitPathCleanupPreflightsPermissionsBeforeDeletingMarker(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}

	workspace, err := NewWorkspace(t.TempDir(), GitPathProbe)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace}
	tree, err := createOwnedTree(environment, gitPathTreeName)
	if err != nil {
		t.Fatalf("createOwnedTree() error = %v", err)
	}
	blocked := filepath.Join(tree, "blocked")
	if err := os.Mkdir(blocked, 0o500); err != nil {
		t.Fatalf("Mkdir(blocked) error = %v", err)
	}
	entries, err := workspace.Entries()
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	resource := entries[len(entries)-1].Resource
	if err := cleanupGitPathProbe(context.Background(), workspace, []Resource{resource}); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("cleanupGitPathProbe(blocked) error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(filepath.Join(tree, gitTreeMarker)); err != nil {
		t.Fatalf("ownership marker was removed before refusal: %v", err)
	}
	if err := os.Chmod(blocked, 0o700); err != nil {
		t.Fatalf("Chmod(blocked) error = %v", err)
	}
	if err := cleanupGitPathProbe(context.Background(), workspace, []Resource{resource}); err != nil {
		t.Fatalf("cleanupGitPathProbe(after chmod) error = %v", err)
	}
	if err := CleanupWorkspace(workspace.Root()); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
}

func TestNoSharedGitObjectsDetectsHardlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "source")
	box := filepath.Join(root, "box")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("Mkdir(source) error = %v", err)
	}
	if err := os.Mkdir(box, 0o700); err != nil {
		t.Fatalf("Mkdir(box) error = %v", err)
	}
	sourceObject := filepath.Join(source, "object")
	if err := os.WriteFile(sourceObject, []byte("object"), 0o600); err != nil {
		t.Fatalf("WriteFile(source object) error = %v", err)
	}
	if err := os.Link(sourceObject, filepath.Join(box, "different-name")); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	isolated, err := noSharedGitObjects(source, box)
	if err != nil {
		t.Fatalf("noSharedGitObjects() error = %v", err)
	}
	if isolated {
		t.Fatal("noSharedGitObjects() = true for a cross-directory hardlink")
	}
}

func TestSnapshotsDetectMetadataMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatalf("WriteFile(before) error = %v", err)
	}
	before, err := snapshotTree(root)
	if err != nil {
		t.Fatalf("snapshotTree(before) error = %v", err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatalf("WriteFile(after) error = %v", err)
	}
	after, err := snapshotTree(root)
	if err != nil {
		t.Fatalf("snapshotTree(after) error = %v", err)
	}
	if snapshotsEqual(before, after) {
		t.Fatal("snapshotsEqual() = true after content mutation")
	}
}
