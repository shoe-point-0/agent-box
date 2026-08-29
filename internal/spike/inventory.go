package spike

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const inventorySchema = "abox.spike.inventory/v1"

var ErrInvalidInventory = errors.New("invalid spike inventory")

type Resource struct {
	Kind       string            `json:"kind"`
	ID         string            `json:"id"`
	OwnerID    string            `json:"ownerId"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type InventoryEntry struct {
	SchemaVersion string    `json:"schemaVersion"`
	Sequence      uint64    `json:"sequence"`
	RecordedAt    time.Time `json:"recordedAt"`
	Action        string    `json:"action"`
	Resource      Resource  `json:"resource"`
}

type Inventory struct {
	mu      sync.Mutex
	path    string
	ownerID string
	next    uint64
}

func createInventory(path, ownerID string) (*Inventory, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create inventory: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("sync inventory: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close inventory: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &Inventory{path: path, ownerID: ownerID, next: 1}, nil
}

func openInventory(path, ownerID string) (*Inventory, error) {
	entries, err := readInventory(path)
	if err != nil {
		return nil, err
	}
	next := uint64(1)
	if len(entries) != 0 {
		next = entries[len(entries)-1].Sequence + 1
	}
	return &Inventory{path: path, ownerID: ownerID, next: next}, nil
}

func (i *Inventory) Append(action string, resource Resource) (InventoryEntry, error) {
	if i == nil {
		return InventoryEntry{}, fmt.Errorf("%w: inventory is nil", ErrInvalidInventory)
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.path == "" || i.ownerID == "" {
		return InventoryEntry{}, fmt.Errorf("%w: inventory is not initialized", ErrInvalidInventory)
	}
	if action != "planned" && action != "created" && action != "removed" {
		return InventoryEntry{}, fmt.Errorf("%w: unsupported action %q", ErrInvalidInventory, action)
	}
	if resource.Kind == "" || resource.ID == "" {
		return InventoryEntry{}, fmt.Errorf("%w: action, kind, and id are required", ErrInvalidInventory)
	}
	if resource.OwnerID != i.ownerID {
		return InventoryEntry{}, fmt.Errorf("%w: resource owner %q does not match root %q", ErrInvalidInventory, resource.OwnerID, i.ownerID)
	}

	entry := InventoryEntry{
		SchemaVersion: inventorySchema,
		Sequence:      i.next,
		RecordedAt:    time.Now().UTC(),
		Action:        action,
		Resource:      resource,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return InventoryEntry{}, fmt.Errorf("encode inventory entry: %w", err)
	}
	line = append(line, '\n')

	file, err := os.OpenFile(i.path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return InventoryEntry{}, fmt.Errorf("open inventory: %w", err)
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return InventoryEntry{}, fmt.Errorf("append inventory: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return InventoryEntry{}, fmt.Errorf("sync inventory: %w", err)
	}
	if err := file.Close(); err != nil {
		return InventoryEntry{}, fmt.Errorf("close inventory: %w", err)
	}

	i.next++
	return entry, nil
}

func (i *Inventory) Entries() ([]InventoryEntry, error) {
	if i == nil {
		return nil, fmt.Errorf("%w: inventory is nil", ErrInvalidInventory)
	}
	return readInventory(i.path)
}

func readInventory(path string) ([]InventoryEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open inventory: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	entries := make([]InventoryEntry, 0)
	var previous uint64
	for scanner.Scan() {
		var entry InventoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("%w: decode entry %d: %v", ErrInvalidInventory, len(entries)+1, err)
		}
		if entry.SchemaVersion != inventorySchema || entry.Sequence != previous+1 {
			return nil, fmt.Errorf("%w: entry %d has schema %q and sequence %d", ErrInvalidInventory, len(entries)+1, entry.SchemaVersion, entry.Sequence)
		}
		previous = entry.Sequence
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	return entries, nil
}
