package spike

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const defaultOutputLimit = 1024 * 1024

var ErrCommandFailed = errors.New("spike command failed")

type Command struct {
	Argv []string
	Dir  string
	Env  []string
}

type CommandResult struct {
	Argv            []string      `json:"argv"`
	ExitCode        int           `json:"exitCode"`
	Duration        time.Duration `json:"duration"`
	Stdout          []byte        `json:"-"`
	Stderr          []byte        `json:"-"`
	StdoutTruncated bool          `json:"stdoutTruncated"`
	StderrTruncated bool          `json:"stderrTruncated"`
}

type Runner struct {
	MaxOutput int
}

func (r Runner) Run(ctx context.Context, command Command) (CommandResult, error) {
	if len(command.Argv) == 0 || command.Argv[0] == "" {
		return CommandResult{ExitCode: -1}, errors.New("run spike command: empty argv")
	}
	limit := r.MaxOutput
	if limit <= 0 {
		limit = defaultOutputLimit
	}

	stdout := newLimitBuffer(limit)
	stderr := newLimitBuffer(limit)
	cmd := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err := cmd.Run()
	result := CommandResult{
		Argv:            append([]string(nil), command.Argv...),
		ExitCode:        0,
		Duration:        time.Since(started),
		Stdout:          stdout.Bytes(),
		Stderr:          stderr.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
	}
	if err == nil {
		return result, nil
	}
	result.ExitCode = -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("%w: %v", ErrCommandFailed, ctxErr)
	}
	return result, fmt.Errorf("%w: %v", ErrCommandFailed, err)
}

type limitBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newLimitBuffer(limit int) *limitBuffer {
	return &limitBuffer{limit: limit}
}

func (b *limitBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || original != 0
		return original, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(data)
	return original, nil
}

func (b *limitBuffer) Bytes() []byte {
	return append([]byte(nil), b.buffer.Bytes()...)
}

func (b *limitBuffer) Truncated() bool {
	return b.truncated
}
