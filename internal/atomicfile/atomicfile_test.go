package atomicfile_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
)

var errInjected = errors.New("injected failure")

func TestWriteFileReplacesExistingFile(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "plan.json")
	writeFixture(t, destination, []byte("complete-old-content\n"), 0o600)

	if err := atomicfile.WriteFile(destination, []byte("complete-new-content\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	assertContent(t, destination, []byte("complete-new-content\n"))
	assertNoTemporaryFiles(t, directory)
}

func TestWriterFailureLeavesCompleteVersion(t *testing.T) {
	oldContent := bytes.Repeat([]byte("old-content\n"), 32)
	newContent := bytes.Repeat([]byte("new-content\n"), 32)

	tests := []struct {
		name      string
		stage     atomicfile.Stage
		committed bool
		configure func(*atomicfile.Operations)
	}{
		{
			name:  "write",
			stage: atomicfile.StageWrite,
			configure: func(operations *atomicfile.Operations) {
				calls := 0
				operations.Write = func(file *os.File, data []byte) (int, error) {
					calls++
					if calls == 1 {
						return file.Write(data[:len(data)/2])
					}
					return 0, errInjected
				}
			},
		},
		{
			name:  "file sync",
			stage: atomicfile.StageFileSync,
			configure: func(operations *atomicfile.Operations) {
				operations.SyncFile = func(*os.File) error { return errInjected }
			},
		},
		{
			name:  "rename",
			stage: atomicfile.StageRename,
			configure: func(operations *atomicfile.Operations) {
				operations.Rename = func(string, string) error { return errInjected }
			},
		},
		{
			name:      "directory sync",
			stage:     atomicfile.StageDirectorySync,
			committed: true,
			configure: func(operations *atomicfile.Operations) {
				operations.SyncDirectory = func(string) error { return syscall.ENOTSUP }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			destination := filepath.Join(directory, "plan.json")
			writeFixture(t, destination, oldContent, 0o600)

			operations := atomicfile.SystemOperations()
			test.configure(&operations)
			writer, err := atomicfile.NewWriter(operations)
			if err != nil {
				t.Fatalf("NewWriter() error = %v", err)
			}
			err = writer.WriteFile(destination, newContent, 0o600)
			if err == nil {
				t.Fatal("WriteFile() error = nil, want injected failure")
			}

			var writeError *atomicfile.Error
			if !errors.As(err, &writeError) {
				t.Fatalf("WriteFile() error type = %T, want *atomicfile.Error", err)
			}
			if writeError.Stage != test.stage || writeError.Committed != test.committed {
				t.Fatalf("WriteFile() error = stage %q committed %t, want stage %q committed %t", writeError.Stage, writeError.Committed, test.stage, test.committed)
			}
			if test.stage == atomicfile.StageDirectorySync {
				if !errors.Is(err, syscall.ENOTSUP) {
					t.Fatalf("WriteFile() error = %v, want wrapped ENOTSUP", err)
				}
			} else if !errors.Is(err, errInjected) {
				t.Fatalf("WriteFile() error = %v, want wrapped injected error", err)
			}

			want := oldContent
			if test.committed {
				want = newContent
			}
			assertContent(t, destination, want)
			assertNoTemporaryFiles(t, directory)
		})
	}
}

func TestWriterCompletesShortWritesAndSyncsDirectory(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "context.json")
	content := bytes.Repeat([]byte("complete-content\n"), 16)
	operations := atomicfile.SystemOperations()
	systemWrite := operations.Write
	systemSyncDirectory := operations.SyncDirectory
	writeCalls := 0
	directorySyncCalls := 0
	operations.Write = func(file *os.File, data []byte) (int, error) {
		writeCalls++
		if len(data) > 7 {
			data = data[:7]
		}
		return systemWrite(file, data)
	}
	operations.SyncDirectory = func(path string) error {
		directorySyncCalls++
		return systemSyncDirectory(path)
	}

	writer, err := atomicfile.NewWriter(operations)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	if err := writer.WriteFile(destination, content, 0o400); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if writeCalls < 2 {
		t.Fatalf("write calls = %d, want multiple short writes", writeCalls)
	}
	if directorySyncCalls != 1 {
		t.Fatalf("directory sync calls = %d, want 1", directorySyncCalls)
	}
	assertContent(t, destination, content)
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o400 {
		t.Fatalf("destination mode = %04o, want 0400", got)
	}
	assertNoTemporaryFiles(t, directory)
}

func TestWriterRefusesUnsafeDestinationsAndModes(t *testing.T) {
	t.Run("symbolic link", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		destination := filepath.Join(directory, "context.json")
		writeFixture(t, target, []byte("target\n"), 0o600)
		if err := os.Symlink(target, destination); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		err := atomicfile.WriteFile(destination, []byte("replacement\n"), 0o600)
		assertStage(t, err, atomicfile.StageValidate)
		assertContent(t, target, []byte("target\n"))
		info, statErr := os.Lstat(destination)
		if statErr != nil {
			t.Fatalf("Lstat() error = %v", statErr)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("destination is no longer a symbolic link")
		}
		assertNoTemporaryFiles(t, directory)
	})

	t.Run("unsafe requested mode", func(t *testing.T) {
		directory := t.TempDir()
		destination := filepath.Join(directory, "plan.json")
		err := atomicfile.WriteFile(destination, []byte("new\n"), 0o644)
		assertStage(t, err, atomicfile.StageValidate)
		if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("Lstat() error = %v, want not exist", statErr)
		}
		assertNoTemporaryFiles(t, directory)
	})

	t.Run("unsafe existing mode", func(t *testing.T) {
		directory := t.TempDir()
		destination := filepath.Join(directory, "plan.json")
		writeFixture(t, destination, []byte("old\n"), 0o644)
		err := atomicfile.WriteFile(destination, []byte("new\n"), 0o600)
		assertStage(t, err, atomicfile.StageValidate)
		assertContent(t, destination, []byte("old\n"))
		assertNoTemporaryFiles(t, directory)
	})

	t.Run("non-regular destination", func(t *testing.T) {
		directory := t.TempDir()
		destination := filepath.Join(directory, "plan.json")
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		err := atomicfile.WriteFile(destination, []byte("new\n"), 0o600)
		assertStage(t, err, atomicfile.StageValidate)
		assertNoTemporaryFiles(t, directory)
	})
}

func TestWriterInterruptionLeavesIdentifiableTemporaryFile(t *testing.T) {
	if os.Getenv("ABOX_ATOMIC_HELPER") == "1" {
		operations := atomicfile.SystemOperations()
		operations.Rename = func(string, string) error {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			os.Exit(99)
			return nil
		}
		writer, err := atomicfile.NewWriter(operations)
		if err != nil {
			os.Exit(98)
		}
		if err := writer.WriteFile(os.Getenv("ABOX_ATOMIC_DESTINATION"), []byte("complete-new-content\n"), 0o600); err != nil {
			os.Exit(97)
		}
		os.Exit(96)
	}

	directory := t.TempDir()
	destination := filepath.Join(directory, "plan.json")
	writeFixture(t, destination, []byte("complete-old-content\n"), 0o600)
	command := exec.Command(os.Args[0], "-test.run=^TestWriterInterruptionLeavesIdentifiableTemporaryFile$")
	command.Env = append(os.Environ(), "ABOX_ATOMIC_HELPER=1", "ABOX_ATOMIC_DESTINATION="+destination)
	if err := command.Run(); err == nil {
		t.Fatal("helper succeeded, want process interruption")
	}

	assertContent(t, destination, []byte("complete-old-content\n"))
	temporary := temporaryFiles(t, directory)
	if len(temporary) != 1 {
		t.Fatalf("temporary files = %v, want exactly one", temporary)
	}
	assertContent(t, temporary[0], []byte("complete-new-content\n"))
	info, err := os.Stat(temporary[0])
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("temporary mode = %04o, want 0600", got)
	}
}

func TestNewWriterRefusesIncompleteOperations(t *testing.T) {
	operations := atomicfile.SystemOperations()
	operations.Rename = nil
	if _, err := atomicfile.NewWriter(operations); err == nil {
		t.Fatal("NewWriter() error = nil, want incomplete operations error")
	}
}

func assertStage(t *testing.T, err error, stage atomicfile.Stage) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want stage %q", stage)
	}
	var writeError *atomicfile.Error
	if !errors.As(err, &writeError) {
		t.Fatalf("error type = %T, want *atomicfile.Error", err)
	}
	if writeError.Stage != stage || writeError.Committed {
		t.Fatalf("error = stage %q committed %t, want stage %q committed false", writeError.Stage, writeError.Committed, stage)
	}
}

func writeFixture(t *testing.T, path string, data []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func assertContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, got, want)
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	if temporary := temporaryFiles(t, directory); len(temporary) != 0 {
		t.Fatalf("temporary files = %v, want none", temporary)
	}
}

func temporaryFiles(t *testing.T, directory string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, atomicfile.TemporaryPrefix+"*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	return matches
}

func ExampleError() {
	operations := atomicfile.SystemOperations()
	operations.SyncDirectory = func(string) error { return syscall.ENOTSUP }
	writer, _ := atomicfile.NewWriter(operations)
	directory, _ := os.MkdirTemp("", "atomicfile-example-")
	defer func() { _ = os.RemoveAll(directory) }()

	err := writer.WriteFile(filepath.Join(directory, "plan.json"), []byte("{}\n"), 0o600)
	var writeError *atomicfile.Error
	if errors.As(err, &writeError) {
		fmt.Printf("stage=%s committed=%t unsupported=%t\n", writeError.Stage, writeError.Committed, errors.Is(err, syscall.ENOTSUP))
	}
	// Output: stage=directory-sync committed=true unsupported=true
}
