package spike

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootCreatesRestrictedDirectoryAndCleansOnlyItself(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	sibling := filepath.Join(parent, "sibling")
	if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile(sibling) error = %v", err)
	}

	root, err := NewRoot(parent)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	path := root.Path()
	if got, err := filepath.Rel(parent, path); err != nil || got == "." || got == ".." {
		t.Fatalf("root path %q is not beneath parent %q", path, parent)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(root) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("root mode = %o, want 700", got)
	}

	if err := root.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(cleaned root) error = %v, want not exist", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "keep" {
		t.Fatalf("sibling after cleanup = %q, %v; want keep", got, err)
	}
	if err := root.Cleanup(); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}
}

func TestRootRejectsInvalidParents(t *testing.T) {
	t.Parallel()

	if _, err := NewRoot(""); err == nil {
		t.Fatal("NewRoot(empty) error = nil, want error")
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := NewRoot(file); err == nil {
		t.Fatal("NewRoot(file) error = nil, want error")
	}
}
