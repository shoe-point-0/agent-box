package daemon

import (
	"path/filepath"
	"testing"
)

func TestDefaultUserPathsUsesDarwinUserLibrary(t *testing.T) {
	home := t.TempDir()
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// XDG settings must not override the native Darwin layout.
	t.Setenv("XDG_STATE_HOME", "relative/ignored-state")
	t.Setenv("XDG_RUNTIME_DIR", "relative/ignored-runtime")
	paths, err := DefaultUserPaths()
	if err != nil {
		t.Fatal(err)
	}
	applicationRoot := filepath.Join(canonicalHome, "Library", "Application Support", "Agent Box")
	want := UserPaths{
		RuntimeDir:    filepath.Join(applicationRoot, "runtime"),
		StateDatabase: filepath.Join(applicationRoot, "state.db"),
		LogFile:       filepath.Join(canonicalHome, "Library", "Logs", "Agent Box", "aboxd.log"),
		Socket:        filepath.Join(applicationRoot, "runtime", "aboxd.sock"),
	}
	if paths != want {
		t.Fatalf("DefaultUserPaths() = %#v, want %#v", paths, want)
	}
}

func TestDefaultUserPathsRejectsMissingDarwinHome(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := DefaultUserPaths(); err == nil {
		t.Fatal("DefaultUserPaths accepted missing HOME")
	}
}
