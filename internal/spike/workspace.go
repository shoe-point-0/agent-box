package spike

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	rootSchema     = "abox.spike.root/v1"
	resultFile     = "result.json"
	rootFile       = "root.json"
	inventoryFile  = "inventory.ndjson"
	diagnosticsDir = "diagnostics"
)

var ErrCleanupRefused = errors.New("spike cleanup refused")

type RootMetadata struct {
	SchemaVersion string    `json:"schemaVersion"`
	ID            string    `json:"id"`
	Scenario      string    `json:"scenario"`
	Path          string    `json:"path"`
	Parent        string    `json:"parent"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Workspace struct {
	metadata  RootMetadata
	inventory *Inventory
}

func NewWorkspace(parent, scenario string) (*Workspace, error) {
	root, err := NewRoot(parent)
	if err != nil {
		return nil, err
	}
	path := root.Path()
	cleanup := func() {
		_ = os.Remove(filepath.Join(path, inventoryFile))
		_ = os.Remove(filepath.Join(path, rootFile))
		_ = os.Remove(filepath.Join(path, diagnosticsDir))
		_ = root.Cleanup()
	}

	id, err := randomID()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create workspace identity: %w", err)
	}
	metadata := RootMetadata{
		SchemaVersion: rootSchema,
		ID:            id,
		Scenario:      scenario,
		Path:          path,
		Parent:        filepath.Dir(path),
		CreatedAt:     time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("encode workspace identity: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := writeSyncedFile(filepath.Join(path, rootFile), encoded, 0o600); err != nil {
		cleanup()
		return nil, fmt.Errorf("persist workspace identity: %w", err)
	}
	inventory, err := createInventory(filepath.Join(path, inventoryFile), id)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(path, diagnosticsDir), 0o700); err != nil {
		cleanup()
		return nil, fmt.Errorf("create diagnostics directory: %w", err)
	}
	if err := syncDirectory(path); err != nil {
		cleanup()
		return nil, err
	}
	return &Workspace{metadata: metadata, inventory: inventory}, nil
}

func OpenWorkspace(path string) (*Workspace, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open workspace: resolve path: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("open workspace: inspect root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("open workspace: root is not a real directory")
	}
	markerPath := filepath.Join(path, rootFile)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil {
		return nil, fmt.Errorf("open workspace: inspect identity: %w", err)
	}
	if !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("open workspace: identity is not a regular file")
	}
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return nil, fmt.Errorf("open workspace: read identity: %w", err)
	}
	var metadata RootMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("open workspace: decode identity: %w", err)
	}
	if metadata.SchemaVersion != rootSchema || metadata.ID == "" || metadata.Path != path || metadata.Parent != filepath.Dir(path) {
		return nil, errors.New("open workspace: identity does not match root")
	}
	inventory, err := openInventory(filepath.Join(path, inventoryFile), metadata.ID)
	if err != nil {
		return nil, err
	}
	return &Workspace{metadata: metadata, inventory: inventory}, nil
}

func (w *Workspace) Metadata() RootMetadata { return w.metadata }
func (w *Workspace) Root() string           { return w.metadata.Path }
func (w *Workspace) InventoryPath() string  { return filepath.Join(w.Root(), inventoryFile) }
func (w *Workspace) DiagnosticsPath() string {
	return filepath.Join(w.Root(), diagnosticsDir)
}

func (w *Workspace) Entries() ([]InventoryEntry, error) {
	return w.inventory.Entries()
}

func (w *Workspace) MarkRemoved(resource Resource) error {
	if resource.OwnerID != w.metadata.ID {
		return fmt.Errorf("mark resource removed: owner %q does not match root", resource.OwnerID)
	}
	_, err := w.inventory.Append("removed", resource)
	return err
}

func (w *Workspace) WriteOwnedFile(relative string, data []byte, mode os.FileMode) (string, error) {
	path, err := w.resolveRelative(relative)
	if err != nil {
		return "", err
	}
	if filepath.Dir(path) != w.Root() && filepath.Dir(path) != w.DiagnosticsPath() {
		return "", fmt.Errorf("write owned file: parent is not an owned core directory: %s", filepath.Dir(path))
	}
	resource := Resource{Kind: "path", ID: path, OwnerID: w.metadata.ID, Attributes: map[string]string{"type": "file"}}
	if _, err := w.inventory.Append("planned", resource); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return "", fmt.Errorf("write owned file: create: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write owned file: write: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write owned file: sync: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("write owned file: close: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	if _, err := w.inventory.Append("created", resource); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Workspace) WriteDiagnostic(name string, data []byte) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) {
		return "", errors.New("write diagnostic: invalid name")
	}
	return w.WriteOwnedFile(filepath.Join(diagnosticsDir, name), data, 0o600)
}

func (w *Workspace) WriteResult(result RunResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run result: %w", err)
	}
	data = append(data, '\n')
	if err := writeSyncedFile(filepath.Join(w.Root(), resultFile), data, 0o600); err != nil {
		return fmt.Errorf("persist run result: %w", err)
	}
	return nil
}

func (w *Workspace) resolveRelative(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("resolve owned path: relative path required")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("resolve owned path: path escapes root")
	}
	path := filepath.Join(w.Root(), clean)
	if !isWithin(w.Root(), path) {
		return "", errors.New("resolve owned path: path escapes root")
	}
	return path, nil
}

func CleanupWorkspace(path string) error {
	workspace, err := OpenWorkspace(path)
	if err != nil {
		return err
	}
	entries, err := workspace.Entries()
	if err != nil {
		return err
	}

	active := make(map[string]InventoryEntry)
	for _, entry := range entries {
		resource := entry.Resource
		if resource.OwnerID != workspace.metadata.ID {
			return fmt.Errorf("%w: resource %q has owner %q", ErrCleanupRefused, resource.ID, resource.OwnerID)
		}
		active[resource.Kind+"\x00"+resource.ID] = entry
	}

	owned := make(map[string]Resource)
	for _, entry := range active {
		if entry.Action != "created" {
			continue
		}
		resource := entry.Resource
		if resource.Kind != "path" {
			return fmt.Errorf("%w: unsupported resource kind %q requires scenario cleanup", ErrCleanupRefused, resource.Kind)
		}
		if !isWithin(workspace.Root(), resource.ID) || resource.ID == workspace.Root() {
			return fmt.Errorf("%w: resource path escapes root: %s", ErrCleanupRefused, resource.ID)
		}
		owned[filepath.Clean(resource.ID)] = resource
	}

	if err := preflightWorkspace(workspace.Root(), owned); err != nil {
		return err
	}
	paths := make([]string, 0, len(owned))
	for path := range owned {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(left, right int) bool {
		return strings.Count(paths[left], string(filepath.Separator)) > strings.Count(paths[right], string(filepath.Separator))
	})
	for _, ownedPath := range paths {
		if err := os.Remove(ownedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clean owned path %s: %w", ownedPath, err)
		}
	}
	for _, core := range []string{resultFile, inventoryFile, rootFile, diagnosticsDir} {
		if err := os.Remove(filepath.Join(workspace.Root(), core)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clean workspace control path %s: %w", core, err)
		}
	}
	root := &Root{parent: workspace.metadata.Parent, path: workspace.Root()}
	return root.Cleanup()
}

func preflightWorkspace(root string, owned map[string]Resource) error {
	core := map[string]bool{
		filepath.Join(root, rootFile):       true,
		filepath.Join(root, inventoryFile):  true,
		filepath.Join(root, resultFile):     true,
		filepath.Join(root, diagnosticsDir): true,
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%w: inspect %s: %v", ErrCleanupRefused, path, walkErr)
		}
		if path == root || owned[path].ID != "" || core[path] {
			return nil
		}
		return fmt.Errorf("%w: unowned path present: %s", ErrCleanupRefused, path)
	})
}

func isWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func randomID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
