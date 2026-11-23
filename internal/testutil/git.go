package testutil

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

// RequireGit returns the absolute path to Git for a real-repository fixture.
// A missing Git skips the fixture; an unsafe or unusable lookup fails it.
func RequireGit(t testing.TB) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if errors.Is(err, exec.ErrNotFound) {
		t.Skip("real Git is unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
