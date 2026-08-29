package spike

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryPersistsOrderedEntries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "inventory.ndjson")
	inventory, err := createInventory(path, "owner")
	if err != nil {
		t.Fatalf("createInventory() error = %v", err)
	}
	resource := Resource{Kind: "path", ID: "/tmp/owned", OwnerID: "owner"}
	if _, err := inventory.Append("planned", resource); err != nil {
		t.Fatalf("Append(planned) error = %v", err)
	}
	if _, err := inventory.Append("created", resource); err != nil {
		t.Fatalf("Append(created) error = %v", err)
	}

	reopened, err := openInventory(path, "owner")
	if err != nil {
		t.Fatalf("openInventory() error = %v", err)
	}
	entries, err := reopened.Entries()
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Sequence != 1 || entries[1].Sequence != 2 {
		t.Fatalf("entries = %+v, want ordered sequences 1 and 2", entries)
	}
	if _, err := reopened.Append("created", resource); err != nil {
		t.Fatalf("Append after reopen error = %v", err)
	}
	entries, err = readInventory(path)
	if err != nil {
		t.Fatalf("readInventory() error = %v", err)
	}
	if got := entries[len(entries)-1].Sequence; got != 3 {
		t.Fatalf("last sequence = %d, want 3", got)
	}
}

func TestInventoryRejectsWrongOwnerAndCorruption(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "inventory.ndjson")
	inventory, err := createInventory(path, "owner")
	if err != nil {
		t.Fatalf("createInventory() error = %v", err)
	}
	if _, err := inventory.Append("created", Resource{Kind: "path", ID: "/tmp/path", OwnerID: "other"}); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("Append(wrong owner) error = %v, want ErrInvalidInventory", err)
	}
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt inventory) error = %v", err)
	}
	if _, err := readInventory(path); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("readInventory(corrupt) error = %v, want ErrInvalidInventory", err)
	}
}
