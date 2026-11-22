package compose

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
)

// GuestCommandInvoker transports a complete command to one runtime host.
type GuestCommandInvoker interface {
	InvokeCommand(context.Context, lima.Spec, lima.GuestCommand) (process.Result, error)
}

// GuestRunner binds normalization to one freshly reconciled runtime host.
type GuestRunner struct {
	invoker GuestCommandInvoker
	spec    lima.Spec
}

func NewGuestRunner(invoker GuestCommandInvoker, spec lima.Spec) (*GuestRunner, error) {
	if invoker == nil || spec.RuntimeHost.InstanceName == "" {
		return nil, errors.New("configure Compose guest runner: invoker and runtime host are required")
	}
	return &GuestRunner{invoker: invoker, spec: spec}, nil
}

func (r *GuestRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	secrets := make([]process.Secret, len(command.Secrets))
	for index, secret := range command.Secrets {
		secrets[index] = process.Secret{Value: secret.Value}
	}
	inputs, err := guestComposeInputs(command.Argv, command.Dir)
	if err != nil {
		return CommandResult{}, err
	}
	result, err := r.invoker.InvokeCommand(ctx, r.spec, lima.GuestCommand{
		Argv: append([]string(nil), command.Argv...), Workdir: command.Dir,
		Environment: cloneMap(command.Env), Secrets: secrets, InputFiles: inputs,
	})
	return CommandResult{
		Stdout: append([]byte(nil), result.Stdout...), Stderr: append([]byte(nil), result.Stderr...),
		StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated,
	}, err
}

// guestComposeInputs reads only the owner-only generated Compose files. Source
// includes stay at their invariant worktree paths; no additional mount is needed.
func guestComposeInputs(argv []string, directory string) ([]lima.GuestInputFile, error) {
	var inputs []lima.GuestInputFile
	for index := 0; index < len(argv); index++ {
		if argv[index] != "-f" {
			continue
		}
		index++
		if index == len(argv) {
			return nil, errors.New("generated Compose input is missing")
		}
		path := argv[index]
		parent := filepath.Dir(path)
		base := filepath.Base(path)
		staged := filepath.Dir(parent) == directory && strings.HasPrefix(filepath.Base(parent), ".abox-compose-compile-")
		if !cleanRuntimePath(path) || (parent != directory && !staged) || (base != RootFileName && base != OverrideFileName) {
			return nil, errors.New("generated Compose input is outside its owned directory")
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || canonical != path {
			return nil, errors.New("generated Compose input is not canonical")
		}
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 || before.Size() > lima.MaxGuestInputBytes {
			return nil, errors.New("generated Compose input has unsafe type, mode, or size")
		}
		data, err := readGuestInput(path, before)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, lima.GuestInputFile{ArgIndex: index, Data: data})
	}
	return inputs, nil
}

func readGuestInput(path string, before os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	current, err := file.Stat()
	if err != nil || !os.SameFile(before, current) {
		return nil, errors.New("generated Compose input changed before reading")
	}
	data, err := io.ReadAll(io.LimitReader(file, lima.MaxGuestInputBytes+1))
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || len(data) > lima.MaxGuestInputBytes {
		return nil, errors.New("generated Compose input changed or exceeded size limit")
	}
	return data, nil
}
