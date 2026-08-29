package process

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunnerUsesLiteralArgumentsExplicitEnvironmentAndDirectory(t *testing.T) {
	dir := t.TempDir()
	ambientName := "ABOX_PROCESS_AMBIENT_CANARY"
	t.Setenv(ambientName, "must-not-be-inherited")
	literal := "$(touch owned); * ; & |"

	command := helperCommand(t, dir, "inspect", literal, ambientName)
	command.Env["ALLOWED_VALUE"] = "visible"
	result, err := (Runner{}).Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run(inspect) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")
	if len(lines) != 4 || lines[0] != literal || lines[1] != "visible" || lines[2] != "" || lines[3] != dir {
		t.Fatalf("Run(inspect) lines = %#v, want literal, allowlisted value, empty ambient value, and %q", lines, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "owned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("shell metacharacters were interpreted")
	}
}

func TestRunnerResolvesBareExecutableOnlyFromSuppliedPath(t *testing.T) {
	dir := t.TempDir()
	command := helperCommand(t, dir, "exit", "0")
	command.Argv[0] = filepath.Base(os.Args[0])
	command.Env["PATH"] = filepath.Dir(os.Args[0])
	if _, err := (Runner{}).Run(context.Background(), command); err != nil {
		t.Fatalf("Run(bare executable) error = %v", err)
	}

	delete(command.Env, "PATH")
	_, err := (Runner{}).Run(context.Background(), command)
	var startErr *StartError
	if !errors.As(err, &startErr) {
		t.Fatalf("Run(without PATH) error = %T, want *StartError", err)
	}
}

func TestRunnerReturnsTypedExitAndRedactedPartialOutput(t *testing.T) {
	secret := "exit-canary-73+/"
	command := helperCommand(t, t.TempDir(), "emit-exit", secret, "23")
	command.Secrets = []Secret{{Value: secret}}
	result, err := (Runner{}).Run(context.Background(), command)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 23 || result.ExitCode != 23 {
		t.Fatalf("Run(emit-exit) error = %T, exit = %d, want typed code 23", err, result.ExitCode)
	}
	assertNoSecretForms(t, secret, result, err, nil)
	if !bytes.Contains(result.Stdout, []byte(redactionReplacement)) || !bytes.Contains(result.Stderr, []byte(redactionReplacement)) {
		t.Fatal("partial stdout/stderr did not contain redaction markers")
	}
}

func TestRunnerRedactsExactAndTransformedFormsBeforeCaptureAndSinks(t *testing.T) {
	secret := "canary value+/7"
	forms := secretForms(secret)
	command := helperCommand(t, t.TempDir(), "emit-forms", forms...)
	command.Secrets = []Secret{{Value: secret}}
	var stdout, stderr lockedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	result, err := (Runner{}).Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run(emit-forms) error = %v", err)
	}
	sinkOutput := append(stdout.bytes(), stderr.bytes()...)
	assertNoSecretForms(t, secret, result, err, sinkOutput)
	if count := bytes.Count(append(result.Stdout, result.Stderr...), []byte(redactionReplacement)); count < len(forms) {
		t.Fatalf("redaction markers = %d, want at least %d", count, len(forms))
	}
}

func TestRunnerReturnsTypedOutputLimitAndDrainsBlockedChild(t *testing.T) {
	command := helperCommand(t, t.TempDir(), "flood")
	started := time.Now()
	result, err := (Runner{CaptureLimit: 1024, TermGrace: 10 * time.Millisecond}).Run(context.Background(), command)
	var limitErr *OutputLimitError
	if !errors.As(err, &limitErr) || limitErr.Stream != Stdout {
		t.Fatalf("Run(flood) error = %T, want stdout OutputLimitError", err)
	}
	if len(result.Stdout) > 1024 || !result.StdoutTruncated {
		t.Fatalf("Run(flood) captured %d bytes, truncated=%v", len(result.Stdout), result.StdoutTruncated)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Run(flood) took %s; output pipe was not drained and terminated", elapsed)
	}
}

func TestRunnerReturnsTypedSinkFailureWithoutLeakingSinkError(t *testing.T) {
	secret := "sink-canary-91"
	command := helperCommand(t, t.TempDir(), "emit-sleep", secret)
	command.Secrets = []Secret{{Value: secret}}
	sink := &failingWriter{err: errors.New("sink rejected " + secret)}
	command.Stdout = sink

	result, err := (Runner{TermGrace: 10 * time.Millisecond}).Run(context.Background(), command)
	var sinkErr *SinkError
	if !errors.As(err, &sinkErr) || sinkErr.Stream != Stdout {
		t.Fatalf("Run(sink failure) error = %T, want stdout SinkError", err)
	}
	assertNoSecretForms(t, secret, result, err, sink.data())
}

func TestRunnerCancellationKillsOwnedDescendantProcessGroup(t *testing.T) {
	command := helperCommand(t, t.TempDir(), "spawn-descendant")
	pidSink := newPIDWriter()
	command.Stdout = pidSink
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan runOutcome, 1)
	go func() {
		result, err := (Runner{TermGrace: 20 * time.Millisecond}).Run(ctx, command)
		resultCh <- runOutcome{result: result, err: err}
	}()

	var pid int
	select {
	case pid = <-pidSink.pid:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("descendant PID was not reported")
	}
	cancel()
	outcome := <-resultCh
	var cancellationErr *CancellationError
	if !errors.As(outcome.err, &cancellationErr) || !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("Run(canceled) error = %T, want CancellationError wrapping context.Canceled", outcome.err)
	}
	assertProcessGone(t, pid)
}

func TestRunnerCancellationRaceDoesNotLeaveDescendants(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		command := helperCommand(t, t.TempDir(), "spawn-descendant")
		pidSink := newPIDWriter()
		command.Stdout = pidSink
		ctx, cancel := context.WithCancel(context.Background())
		resultCh := make(chan error, 1)
		go func() {
			_, err := (Runner{TermGrace: time.Millisecond}).Run(ctx, command)
			resultCh <- err
		}()
		pid := <-pidSink.pid
		cancel()
		err := <-resultCh
		var cancellationErr *CancellationError
		if !errors.As(err, &cancellationErr) {
			t.Fatalf("iteration %d error = %T, want CancellationError", iteration, err)
		}
		assertProcessGone(t, pid)
	}
}

func TestRunnerRemovesDescendantAfterGroupLeaderExits(t *testing.T) {
	command := helperCommand(t, t.TempDir(), "spawn-descendant-exit")
	pidSink := newPIDWriter()
	command.Stdout = pidSink
	result, err := (Runner{TermGrace: 10 * time.Millisecond}).Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run(spawn-descendant-exit) error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run(spawn-descendant-exit) exit = %d, want 0", result.ExitCode)
	}
	select {
	case pid := <-pidSink.pid:
		assertProcessGone(t, pid)
	default:
		t.Fatal("descendant PID was not streamed")
	}
}

func TestRunnerRejectsOversizedSecretRegistryBeforeStart(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	command := helperCommand(t, t.TempDir(), "touch", marker)
	for index := 0; index < maxSecrets+1; index++ {
		command.Secrets = append(command.Secrets, Secret{Value: fmt.Sprintf("secret-%d", index)})
	}
	_, err := (Runner{}).Run(context.Background(), command)
	if !errors.Is(err, errInvalidSecrets) {
		t.Fatalf("Run(oversized registry) error = %v, want errInvalidSecrets", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("process started before secret registry validation")
	}
}

func TestStreamRedactorHandlesOneByteChunksAndOverlappingSecrets(t *testing.T) {
	factory, err := compileSecrets([]Secret{{Value: "abcde"}, {Value: "bc"}, {Value: "cde"}})
	if err != nil {
		t.Fatal(err)
	}
	redactor := factory.new()
	var output bytes.Buffer
	emit := func(data []byte) error {
		_, _ = output.Write(data)
		return nil
	}
	for _, value := range []byte("xabcdeybczcde") {
		if err := redactor.Write([]byte{value}, emit); err != nil {
			t.Fatal(err)
		}
	}
	if err := redactor.Flush(emit); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "x[REDACTED]y[REDACTED]z[REDACTED]"; got != want {
		t.Fatalf("redacted output = %q, want %q", got, want)
	}
}

func helperCommand(t *testing.T, dir, mode string, arguments ...string) Command {
	t.Helper()
	argv := []string{os.Args[0], "-test.run=^TestProcessHelper$", "--", mode}
	argv = append(argv, arguments...)
	return Command{
		Argv: argv,
		Dir:  dir,
		Env:  map[string]string{"ABOX_PROCESS_HELPER": "1"},
	}
}

func TestProcessHelper(t *testing.T) {
	if os.Getenv("ABOX_PROCESS_HELPER") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(90)
	}
	arguments := os.Args[separator+1:]
	switch arguments[0] {
	case "inspect":
		workingDirectory, _ := os.Getwd()
		_, _ = fmt.Fprintln(os.Stdout, arguments[1])
		_, _ = fmt.Fprintln(os.Stdout, os.Getenv("ALLOWED_VALUE"))
		_, _ = fmt.Fprintln(os.Stdout, os.Getenv(arguments[2]))
		_, _ = fmt.Fprintln(os.Stdout, workingDirectory)
	case "exit":
		code, _ := strconv.Atoi(arguments[1])
		os.Exit(code)
	case "emit-exit":
		writeOneByte(os.Stdout, arguments[1])
		writeOneByte(os.Stderr, arguments[1])
		code, _ := strconv.Atoi(arguments[2])
		os.Exit(code)
	case "emit-forms":
		for index, form := range arguments[1:] {
			writer := os.Stdout
			if index%2 == 1 {
				writer = os.Stderr
			}
			writeOneByte(writer, form+"\n")
		}
	case "flood":
		block := bytes.Repeat([]byte("x"), 64*1024)
		for {
			if _, err := os.Stdout.Write(block); err != nil {
				os.Exit(0)
			}
		}
	case "emit-sleep":
		writeOneByte(os.Stdout, arguments[1])
		time.Sleep(10 * time.Second)
	case "spawn-descendant", "spawn-descendant-exit":
		child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$", "--", "descendant")
		child.Env = os.Environ()
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(92)
		}
		_, _ = fmt.Fprintf(os.Stdout, "PID=%d\n", child.Process.Pid)
		if arguments[0] == "spawn-descendant-exit" {
			os.Exit(0)
		}
		time.Sleep(10 * time.Second)
	case "descendant":
		time.Sleep(10 * time.Second)
	case "touch":
		if err := os.WriteFile(arguments[1], []byte("started"), 0o600); err != nil {
			os.Exit(93)
		}
	default:
		os.Exit(91)
	}
	os.Exit(0)
}

func writeOneByte(writer io.Writer, value string) {
	for index := range len(value) {
		_, _ = writer.Write([]byte{value[index]})
	}
}

func secretForms(secret string) []string {
	return []string{
		secret,
		url.PathEscape(secret),
		url.QueryEscape(secret),
		base64.StdEncoding.EncodeToString([]byte(secret)),
		base64.RawStdEncoding.EncodeToString([]byte(secret)),
		base64.URLEncoding.EncodeToString([]byte(secret)),
		base64.RawURLEncoding.EncodeToString([]byte(secret)),
	}
}

func assertNoSecretForms(t *testing.T, secret string, result Result, err error, extra []byte) {
	t.Helper()
	all := append(append(append([]byte{}, result.Stdout...), result.Stderr...), extra...)
	if err != nil {
		all = append(all, err.Error()...)
	}
	for _, form := range secretForms(secret) {
		if bytes.Contains(all, []byte(form)) {
			t.Fatal("registered secret form leaked")
		}
	}
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}

type failingWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	err    error
}

func (w *failingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(data) > 0 {
		_, _ = w.buffer.Write(data[:1])
	}
	return 1, w.err
}

func (w *failingWriter) data() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buffer.Bytes()...)
}

type pidWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	pid    chan int
	once   sync.Once
}

func newPIDWriter() *pidWriter {
	return &pidWriter{pid: make(chan int, 1)}
}

func (w *pidWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buffer.Write(data)
	text := w.buffer.String()
	if newline := strings.IndexByte(text, '\n'); newline >= 0 && strings.HasPrefix(text, "PID=") {
		pid, err := strconv.Atoi(strings.TrimPrefix(text[:newline], "PID="))
		if err == nil {
			w.once.Do(func() { w.pid <- pid })
		}
	}
	return len(data), nil
}

type runOutcome struct {
	result Result
	err    error
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant process %d still exists", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
