// Package testutil provides filesystem fixtures for native platform tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a private, canonical directory with a short name. Unlike
// testing.T.TempDir, its pathname does not grow with the test name, so Unix
// sockets fit on Darwin. Resolving the temporary root also avoids /var aliases
// being mistaken for deliberately symlinked application paths.
func TempDir(t testing.TB) string {
	t.Helper()
	root, err := os.MkdirTemp("", "abox-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
