// Package atomicfile provides crash-consistent replacement of generated files.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TemporaryPrefix identifies an incomplete Agent Box atomic write. A process
// interruption can leave one of these files in the destination directory.
const TemporaryPrefix = ".abox-atomic-"

// Stage identifies the operation that prevented an atomic write from becoming
// durably complete.
type Stage string

const (
	StageValidate      Stage = "validate"
	StageCreate        Stage = "create"
	StageChmod         Stage = "chmod"
	StageWrite         Stage = "write"
	StageFileSync      Stage = "file-sync"
	StageClose         Stage = "close"
	StageRename        Stage = "rename"
	StageDirectorySync Stage = "directory-sync"
)

// Error reports the failed stage and whether the destination was already
// replaced. When Committed is true, the new file is visible but its directory
// entry was not proven durable.
type Error struct {
	Stage     Stage
	Path      string
	Committed bool
	Err       error
}

func (e *Error) Error() string {
	return fmt.Sprintf("atomic write %s: %s: %v", e.Path, e.Stage, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// Operations is the complete injectable I/O boundary used by Writer after a
// temporary file has been created. All functions must be non-nil.
type Operations struct {
	Write         func(*os.File, []byte) (int, error)
	SyncFile      func(*os.File) error
	Rename        func(string, string) error
	SyncDirectory func(string) error
}

// SystemOperations returns the operating-system implementation. Callers may
// replace individual functions before constructing a Writer for fault tests.
func SystemOperations() Operations {
	return Operations{
		Write:         func(file *os.File, data []byte) (int, error) { return file.Write(data) },
		SyncFile:      func(file *os.File) error { return file.Sync() },
		Rename:        os.Rename,
		SyncDirectory: syncDirectory,
	}
}

// Writer atomically replaces files using an explicit I/O boundary.
type Writer struct {
	operations Operations
}

// NewWriter constructs a Writer and refuses incomplete operation sets.
func NewWriter(operations Operations) (*Writer, error) {
	switch {
	case operations.Write == nil:
		return nil, errors.New("create atomic writer: write operation is nil")
	case operations.SyncFile == nil:
		return nil, errors.New("create atomic writer: file sync operation is nil")
	case operations.Rename == nil:
		return nil, errors.New("create atomic writer: rename operation is nil")
	case operations.SyncDirectory == nil:
		return nil, errors.New("create atomic writer: directory sync operation is nil")
	default:
		return &Writer{operations: operations}, nil
	}
}

// WriteFile uses operating-system operations to atomically replace path.
func WriteFile(path string, data []byte, mode fs.FileMode) error {
	writer, err := NewWriter(SystemOperations())
	if err != nil {
		return err
	}
	return writer.WriteFile(path, data, mode)
}

// WriteFile writes data completely, syncs it, renames it over path, and syncs
// the parent directory. Only owner-readable generated files are accepted.
func (w *Writer) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if w == nil {
		return stageError(StageValidate, path, false, errors.New("writer is nil"))
	}
	if err := validateMode(mode); err != nil {
		return stageError(StageValidate, path, false, err)
	}
	if err := validatePath(path); err != nil {
		return stageError(StageValidate, path, false, err)
	}
	if err := inspectDestination(path); err != nil {
		return stageError(StageValidate, path, false, err)
	}

	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, TemporaryPrefix)
	if err != nil {
		return stageError(StageCreate, path, false, err)
	}
	temporary := file.Name()
	closed := false
	committed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !committed {
			_ = os.Remove(temporary)
		}
	}()

	if err := file.Chmod(mode); err != nil {
		return stageError(StageChmod, path, false, err)
	}
	if err := w.writeAll(file, data); err != nil {
		return stageError(StageWrite, path, false, err)
	}
	if err := w.operations.SyncFile(file); err != nil {
		return stageError(StageFileSync, path, false, err)
	}
	if err := file.Close(); err != nil {
		closed = true
		return stageError(StageClose, path, false, err)
	}
	closed = true

	if err := inspectDestination(path); err != nil {
		return stageError(StageValidate, path, false, err)
	}
	if err := w.operations.Rename(temporary, path); err != nil {
		return stageError(StageRename, path, false, err)
	}
	committed = true
	if err := w.operations.SyncDirectory(directory); err != nil {
		return stageError(StageDirectorySync, path, true, err)
	}
	return nil
}

func (w *Writer) writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := w.operations.Write(file, data)
		if written < 0 || written > len(data) {
			return fmt.Errorf("invalid write count %d for %d bytes", written, len(data))
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return errors.New("write made no progress")
		}
	}
	return nil
}

func validateMode(mode fs.FileMode) error {
	if mode != 0o400 && mode != 0o600 {
		return fmt.Errorf("unsafe file mode %04o: only 0400 and 0600 are allowed", mode)
	}
	return nil
}

func validatePath(path string) error {
	if path == "" {
		return errors.New("destination path is empty")
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return errors.New("destination must name a file")
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("inspect destination directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("destination parent is not a directory")
	}
	return nil
}

func inspectDestination(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect destination: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("destination is a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return errors.New("destination is not a regular file")
	}
	if err := validateMode(info.Mode().Perm()); err != nil {
		return fmt.Errorf("destination permissions: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf("sync directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close directory: %w", err)
	}
	return nil
}

func stageError(stage Stage, path string, committed bool, err error) error {
	return &Error{Stage: stage, Path: path, Committed: committed, Err: err}
}
