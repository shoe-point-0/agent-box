package spike

import (
	"errors"
	"os"
	"testing"
)

func TestInterruptedRunCanBeInspectedAndCleaned(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), "interrupted-fixture")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	if _, err := workspace.WriteOwnedFile("created-before-exit.txt", []byte("evidence"), 0o600); err != nil {
		t.Fatalf("WriteOwnedFile() error = %v", err)
	}

	inspection, err := Inspect(workspace.Root())
	if err != nil {
		t.Fatalf("Inspect(interrupted root) error = %v", err)
	}
	if inspection.Result != nil {
		t.Fatalf("interrupted inspection result = %+v, want nil", inspection.Result)
	}
	if len(inspection.Inventory) != 2 {
		t.Fatalf("interrupted inventory length = %d, want planned and created entries", len(inspection.Inventory))
	}
	if err := CleanupWorkspace(workspace.Root()); err != nil {
		t.Fatalf("CleanupWorkspace(interrupted root) error = %v", err)
	}
	if _, err := os.Stat(workspace.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(cleaned interrupted root) error = %v, want not exist", err)
	}
}
