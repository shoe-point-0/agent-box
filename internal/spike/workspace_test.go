package spike

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupRefusesUnknownPathBeforeMutation(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), "fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	owned, err := workspace.WriteOwnedFile("owned.txt", []byte("owned"), 0o600)
	if err != nil {
		t.Fatalf("WriteOwnedFile() error = %v", err)
	}
	unknown := filepath.Join(workspace.Root(), "unknown.txt")
	if err := os.WriteFile(unknown, []byte("unknown"), 0o600); err != nil {
		t.Fatalf("WriteFile(unknown) error = %v", err)
	}

	if err := CleanupWorkspace(workspace.Root()); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("CleanupWorkspace() error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(owned); err != nil {
		t.Fatalf("owned file was mutated before refusal: %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown file was mutated before refusal: %v", err)
	}

	if err := os.Remove(unknown); err != nil {
		t.Fatalf("Remove(unknown) error = %v", err)
	}
	if err := CleanupWorkspace(workspace.Root()); err != nil {
		t.Fatalf("CleanupWorkspace(after removing unknown) error = %v", err)
	}
	if _, err := os.Stat(workspace.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(cleaned root) error = %v, want not exist", err)
	}
}

func TestFailedOwnedWriteDoesNotClaimExistingFile(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), "fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	existing := filepath.Join(workspace.Root(), "existing.txt")
	if err := os.WriteFile(existing, []byte("unowned"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
	if _, err := workspace.WriteOwnedFile("existing.txt", []byte("replacement"), 0o600); err == nil {
		t.Fatal("WriteOwnedFile(existing) error = nil, want exclusive-create refusal")
	}
	if err := CleanupWorkspace(workspace.Root()); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("CleanupWorkspace() error = %v, want ErrCleanupRefused", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if string(got) != "unowned" {
		t.Fatalf("existing content = %q, want unowned", got)
	}
}

func TestCleanupRefusesRecordedPathOutsideRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	workspace, err := NewWorkspace(parent, "fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	owned, err := workspace.WriteOwnedFile("owned.txt", []byte("owned"), 0o600)
	if err != nil {
		t.Fatalf("WriteOwnedFile() error = %v", err)
	}
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	if _, err := workspace.inventory.Append("created", Resource{
		Kind:    "path",
		ID:      outside,
		OwnerID: workspace.metadata.ID,
	}); err != nil {
		t.Fatalf("Append(forged outside resource) error = %v", err)
	}

	if err := CleanupWorkspace(workspace.Root()); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("CleanupWorkspace() error = %v, want ErrCleanupRefused", err)
	}
	for _, path := range []string{owned, outside} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("path %s was mutated before refusal: %v", path, err)
		}
	}
}

func TestCleanupRefusesUnsupportedResourceKind(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), "fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	if _, err := workspace.inventory.Append("created", Resource{
		Kind:    "lima-instance",
		ID:      "fixture-vm",
		OwnerID: workspace.metadata.ID,
	}); err != nil {
		t.Fatalf("Append(unsupported resource) error = %v", err)
	}
	if err := CleanupWorkspace(workspace.Root()); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("CleanupWorkspace() error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(workspace.Root()); err != nil {
		t.Fatalf("workspace was removed despite refusal: %v", err)
	}
}

func TestOpenWorkspaceRejectsSymlinkRoot(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), "fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(workspace.Root(), link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := OpenWorkspace(link); err == nil {
		t.Fatal("OpenWorkspace(symlink) error = nil, want refusal")
	}
}
