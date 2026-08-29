package spike

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Root is an owned temporary directory for one disposable probe run.
type Root struct {
	parent string
	path   string
}

func NewRoot(parent string) (*Root, error) {
	if strings.TrimSpace(parent) == "" {
		return nil, errors.New("create spike root: empty parent")
	}

	parent, err := filepath.Abs(parent)
	if err != nil {
		return nil, fmt.Errorf("create spike root: resolve parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		return nil, fmt.Errorf("create spike root: inspect parent: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("create spike root: parent is not a directory: %s", parent)
	}

	path, err := os.MkdirTemp(parent, "abox-spike-")
	if err != nil {
		return nil, fmt.Errorf("create spike root: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("create spike root: restrict permissions: %w", err)
	}

	return &Root{parent: parent, path: path}, nil
}

func (r *Root) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

func (r *Root) Cleanup() error {
	if r == nil || r.path == "" {
		return nil
	}

	rel, err := filepath.Rel(r.parent, r.path)
	if err != nil {
		return fmt.Errorf("clean spike root: resolve ownership: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("clean spike root: path escaped parent: %s", r.path)
	}

	if err := os.Remove(r.path); err != nil {
		return fmt.Errorf("clean spike root: %w", err)
	}
	r.path = ""
	return nil
}
