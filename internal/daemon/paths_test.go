//go:build linux

package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUserPathsUsesAbsoluteXDGRoots(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateRoot)
	t.Setenv("XDG_RUNTIME_DIR", runtimeRoot)
	paths, err := DefaultUserPaths()
	if err != nil {
		t.Fatal(err)
	}
	applicationRoot := filepath.Join(stateRoot, "agent-box")
	if paths.RuntimeDir != filepath.Join(runtimeRoot, "agent-box") ||
		paths.StateDatabase != filepath.Join(applicationRoot, "state.db") ||
		paths.LogFile != filepath.Join(applicationRoot, "aboxd.log") ||
		paths.Socket != filepath.Join(runtimeRoot, "agent-box", "aboxd.sock") {
		t.Fatalf("DefaultUserPaths() = %#v", paths)
	}
}

func TestDefaultUserPathsRejectsRelativeXDGRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "relative/state")
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := DefaultUserPaths(); err == nil {
		t.Fatal("DefaultUserPaths() accepted a relative XDG state root")
	}
}
