package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultCaptureLimit    = 1024 * 1024
	defaultTermGrace       = 100 * time.Millisecond
	defaultShutdownTimeout = 2 * time.Second
	outputReadSize         = 4096
)

type Stream string

const (
	Stdout Stream = "stdout"
	Stderr Stream = "stderr"
)

// Command is a complete subprocess request. Env is the entire child
// environment, not an overlay on the Agent Box daemon's environment.
type Command struct {
	Argv    []string
	Dir     string
	Env     map[string]string
	Secrets []Secret
	Stdin   []byte
	Stdout  io.Writer
	Stderr  io.Writer
}

// Runner owns process-group lifecycle and bounded output capture.
type Runner struct {
	CaptureLimit    int
	TermGrace       time.Duration
	ShutdownTimeout time.Duration
}

// Result contains redacted partial output even when Run returns an error.
type Result struct {
	StartedAt       time.Time
	Duration        time.Duration
	ExitCode        int
	Signal          string
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
}

type ExitError struct {
	Code   int
	Signal string
}

func (e *ExitError) Error() string {
	if e.Signal != "" {
		return "process exited from signal " + e.Signal
	}
	return fmt.Sprintf("process exited with code %d", e.Code)
}

type CancellationError struct {
	Cause error
}

func (e *CancellationError) Error() string { return "process canceled" }
func (e *CancellationError) Unwrap() error { return e.Cause }

type SinkError struct {
	Stream Stream
}

func (e *SinkError) Error() string { return string(e.Stream) + " sink failed" }

type OutputLimitError struct {
	Stream Stream
	Limit  int
}

func (e *OutputLimitError) Error() string {
	return fmt.Sprintf("%s exceeded %d-byte output limit", e.Stream, e.Limit)
}

type ShutdownError struct{}

func (*ShutdownError) Error() string { return "process group shutdown exceeded deadline" }

type StartError struct {
	message string
}

func (e *StartError) Error() string { return "start process: " + e.message }

type PipeError struct {
	Stream Stream
}

func (e *PipeError) Error() string { return string(e.Stream) + " pipe failed" }

func (r Runner) Run(ctx context.Context, command Command) (Result, error) {
	result := Result{ExitCode: -1}
	factory, err := compileSecrets(command.Secrets)
	if err != nil {
		return result, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, &CancellationError{Cause: ctxErr}
	}

	path, argv, env, err := prepareCommand(command)
	if err != nil {
		return result, &StartError{message: sanitize(factory, err.Error())}
	}
	limit, termGrace, shutdownTimeout := r.settings()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return result, &StartError{message: "create stdout pipe"}
	}
	defer func() { _ = stdoutReader.Close() }()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return result, &StartError{message: "create stderr pipe"}
	}
	defer func() { _ = stderrReader.Close() }()

	cmd := exec.Command(path, argv[1:]...)
	cmd.Args = append([]string(nil), argv...)
	cmd.Dir = command.Dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(command.Stdin)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	result.StartedAt = time.Now()
	if err := cmd.Start(); err != nil {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		return result, &StartError{message: sanitize(factory, err.Error())}
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	pgid := cmd.Process.Pid

	issues := make(chan error, 2)
	var sinkMu sync.Mutex
	stdoutCollector := newCollector(Stdout, stdoutReader, command.Stdout, factory.new(), limit, &sinkMu, issues)
	stderrCollector := newCollector(Stderr, stderrReader, command.Stderr, factory.new(), limit, &sinkMu, issues)
	collectorsDone := make(chan struct{}, 2)
	go stdoutCollector.run(collectorsDone)
	go stderrCollector.run(collectorsDone)

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var runErr error
	var waitErr error
	waitReceived := false
	select {
	case waitErr = <-waitDone:
		waitReceived = true
	case runErr = <-issues:
	case <-ctx.Done():
		runErr = &CancellationError{Cause: ctx.Err()}
	}

	if runErr != nil || processGroupAlive(pgid) {
		waitErr, waitReceived, err = stopProcessGroup(pgid, termGrace, shutdownTimeout, waitDone, waitErr, waitReceived)
		if err != nil {
			runErr = err
		}
	}
	if !waitReceived {
		select {
		case waitErr = <-waitDone:
		case <-time.After(shutdownTimeout):
			runErr = &ShutdownError{}
		}
	}

	if !waitForCollectors(collectorsDone, shutdownTimeout) {
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
		runErr = &ShutdownError{}
		_ = waitForCollectors(collectorsDone, shutdownTimeout)
	}
	if runErr == nil {
		select {
		case runErr = <-issues:
		default:
		}
	}

	result.Duration = time.Since(result.StartedAt)
	result.Stdout = stdoutCollector.bytes()
	result.Stderr = stderrCollector.bytes()
	result.StdoutTruncated = stdoutCollector.truncated
	result.StderrTruncated = stderrCollector.truncated
	result.ExitCode, result.Signal = exitStatus(waitErr)

	if runErr != nil {
		return result, runErr
	}
	if waitErr != nil {
		return result, &ExitError{Code: result.ExitCode, Signal: result.Signal}
	}
	return result, nil
}

func (r Runner) settings() (limit int, termGrace, shutdownTimeout time.Duration) {
	limit = r.CaptureLimit
	if limit <= 0 {
		limit = defaultCaptureLimit
	}
	termGrace = r.TermGrace
	if termGrace <= 0 {
		termGrace = defaultTermGrace
	}
	shutdownTimeout = r.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	if termGrace >= shutdownTimeout {
		termGrace = shutdownTimeout / 2
	}
	return limit, termGrace, shutdownTimeout
}

func prepareCommand(command Command) (string, []string, []string, error) {
	if len(command.Argv) == 0 || command.Argv[0] == "" {
		return "", nil, nil, errors.New("empty argv")
	}
	if command.Dir == "" || !filepath.IsAbs(command.Dir) {
		return "", nil, nil, errors.New("working directory must be absolute")
	}
	for _, argument := range command.Argv {
		if strings.IndexByte(argument, 0) >= 0 {
			return "", nil, nil, errors.New("argv contains NUL")
		}
	}

	env := make([]string, 0, len(command.Env))
	for name, value := range command.Env {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.IndexByte(value, 0) >= 0 {
			return "", nil, nil, errors.New("invalid environment")
		}
		env = append(env, name+"="+value)
	}
	sort.Strings(env)

	path, err := resolveExecutable(command.Argv[0], command.Dir, command.Env["PATH"])
	if err != nil {
		return "", nil, nil, err
	}
	return path, append([]string(nil), command.Argv...), env, nil
}

func resolveExecutable(name, dir, pathValue string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		candidate := name
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(dir, candidate)
		}
		if executable(candidate) {
			return candidate, nil
		}
		return "", errors.New("executable not found")
	}
	if pathValue == "" {
		return "", errors.New("PATH is required for bare executable names")
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if directory == "" {
			directory = dir
		} else if !filepath.IsAbs(directory) {
			directory = filepath.Join(dir, directory)
		}
		candidate := filepath.Join(directory, name)
		if executable(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("executable not found in supplied PATH")
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func sanitize(factory *redactorFactory, value string) string {
	redactor := factory.new()
	var output bytes.Buffer
	emit := func(data []byte) error {
		_, _ = output.Write(data)
		return nil
	}
	_ = redactor.Write([]byte(value), emit)
	_ = redactor.Flush(emit)
	return output.String()
}

func stopProcessGroup(
	pgid int,
	grace time.Duration,
	timeout time.Duration,
	waitDone <-chan error,
	waitErr error,
	waitReceived bool,
) (error, bool, error) {
	_ = signalProcessGroup(pgid, syscall.SIGTERM)
	started := time.Now()
	graceDeadline := started.Add(grace)
	hardDeadline := started.Add(timeout)
	killed := false
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !processGroupAlive(pgid) && waitReceived {
			return waitErr, true, nil
		}
		now := time.Now()
		if !killed && !now.Before(graceDeadline) {
			_ = signalProcessGroup(pgid, syscall.SIGKILL)
			killed = true
		}
		if !now.Before(hardDeadline) {
			_ = signalProcessGroup(pgid, syscall.SIGKILL)
			return waitErr, waitReceived, &ShutdownError{}
		}
		select {
		case waitErr = <-waitDone:
			waitReceived = true
		case <-ticker.C:
		}
	}
}

func signalProcessGroup(pgid int, signal syscall.Signal) error {
	err := syscall.Kill(-pgid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processGroupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func waitForCollectors(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for count := 0; count < 2; count++ {
		select {
		case <-done:
		case <-timer.C:
			return false
		}
	}
	return true
}

func exitStatus(waitErr error) (int, string) {
	if waitErr == nil {
		return 0, ""
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return -1, ""
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return exitErr.ExitCode(), ""
	}
	if status.Signaled() {
		return -1, status.Signal().String()
	}
	return status.ExitStatus(), ""
}

type collector struct {
	stream    Stream
	reader    io.Reader
	sink      io.Writer
	redactor  *streamRedactor
	limit     int
	sinkMu    *sync.Mutex
	issues    chan<- error
	capture   bytes.Buffer
	truncated bool
	failed    bool
	rawBytes  int
}

func newCollector(
	stream Stream,
	reader io.Reader,
	sink io.Writer,
	redactor *streamRedactor,
	limit int,
	sinkMu *sync.Mutex,
	issues chan<- error,
) *collector {
	return &collector{
		stream: stream, reader: reader, sink: sink, redactor: redactor,
		limit: limit, sinkMu: sinkMu, issues: issues,
	}
}

func (c *collector) run(done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	buffer := make([]byte, outputReadSize)
	for {
		count, err := c.reader.Read(buffer)
		if count > 0 {
			c.process(buffer[:count], false)
			c.rawBytes += count
			if c.rawBytes > c.limit {
				c.truncated = true
				c.report(&OutputLimitError{Stream: c.stream, Limit: c.limit})
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.report(&PipeError{Stream: c.stream})
			}
			c.process(nil, true)
			return
		}
	}
}

func (c *collector) process(data []byte, flush bool) {
	var output bytes.Buffer
	emit := func(chunk []byte) error {
		_, _ = output.Write(chunk)
		return nil
	}
	var err error
	if flush {
		err = c.redactor.Flush(emit)
	} else {
		err = c.redactor.Write(data, emit)
	}
	if err != nil || c.failed || output.Len() == 0 {
		return
	}
	c.write(output.Bytes())
}

func (c *collector) write(data []byte) {
	remaining := c.limit - c.capture.Len()
	if remaining < len(data) {
		c.truncated = true
		c.report(&OutputLimitError{Stream: c.stream, Limit: c.limit})
		if remaining > 0 {
			_, _ = c.capture.Write(data[:remaining])
		}
	} else {
		_, _ = c.capture.Write(data)
	}
	if c.sink == nil {
		return
	}
	c.sinkMu.Lock()
	written, err := c.sink.Write(data)
	c.sinkMu.Unlock()
	if err != nil || written != len(data) {
		c.report(&SinkError{Stream: c.stream})
	}
}

func (c *collector) report(err error) {
	if c.failed {
		return
	}
	c.failed = true
	select {
	case c.issues <- err:
	default:
	}
}

func (c *collector) bytes() []byte {
	return append([]byte(nil), c.capture.Bytes()...)
}
