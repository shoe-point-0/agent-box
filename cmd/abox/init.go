package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

const starterManifest = `apiVersion: abox.dev/v1alpha1
kind: Workspace
metadata:
  name: workspace
spec:
  repositories:
    - name: repository
      source:
        local: ..
      checkout:
        ref: HEAD
        branch: "abox/{{ box.task }}/repository"
      workspacePath: workspace
  host:
    cpus: 2
    memory: 4GiB
    disk: 20GiB
  compose:
    includes:
      - repository: repository
        path: compose.yaml
    profiles:
      - development
  workspaceService:
    name: app
    repository: repository
    workingDirectory: workspace
  mounts:
    - repository: repository
      services:
        - app
      mode: read-write
  policy:
    gitMetadata: read-write
    dockerSocket: deny
    machineMounts: declared-only
    networkEgress: allow
`

type initOptions struct {
	force bool
	help  bool
}

func parseInit(args []string) (initOptions, *contract.Failure) {
	var options initOptions
	for _, argument := range args {
		switch argument {
		case "--force":
			if options.force {
				return options, failure(contract.CodeCLIUsage, "--force may be specified only once")
			}
			options.force = true
		case "--help", "-h":
			options.help = true
		default:
			return options, failure(contract.CodeCLIUsage, "unknown init option")
		}
	}
	if options.help && options.force {
		return options, failure(contract.CodeCLIUsage, "--help cannot be combined with init arguments")
	}
	return options, nil
}

func initializeWorkspace(cwd string, force bool) (string, *contract.Failure) {
	root, err := filepath.Abs(cwd)
	if err != nil {
		return "", failure(contract.CodePrerequisiteFailed, "cannot resolve current directory")
	}
	root = filepath.Clean(root)
	directory := filepath.Join(root, ".abox")
	createdDirectory, err := ensureManifestDirectory(directory)
	if err != nil {
		return "", failure(contract.CodePrerequisiteFailed, "cannot create manifest directory")
	}
	path := filepath.Join(directory, "box.yaml")
	if force {
		err = atomicfile.WriteFile(path, []byte(starterManifest), 0o600)
	} else {
		err = writeManifestExclusive(path, []byte(starterManifest))
	}
	if err == nil {
		return path, nil
	}
	if createdDirectory {
		_ = os.Remove(directory)
	}
	if errors.Is(err, os.ErrExist) {
		return "", failure(contract.CodeIdentityConflict, "manifest already exists; use --force to replace it")
	}
	return "", failure(contract.CodePrerequisiteFailed, "cannot write starter manifest")
}

func ensureManifestDirectory(path string) (bool, error) {
	if err := os.Mkdir(path, 0o700); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrExist) {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, errors.New("manifest directory is not a real directory")
	}
	return false, nil
}

func writeManifestExclusive(path string, data []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	for len(data) > 0 {
		written, writeErr := file.Write(data)
		if writeErr != nil {
			return writeErr
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func writeInitHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox init [--force]

Create .abox/box.yaml without contacting aboxd or provisioning a Box.

Options:
  --force  replace an existing regular owner-only manifest atomically
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
