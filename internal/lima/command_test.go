package lima

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestGuestInvocationTransportsExactEnvironmentOverStdin(t *testing.T) {
	const (
		literal = "line one\nline 'two' $HOME; $(false)"
		secret  = "guest-secret-canary"
	)
	argv, stdin, err := compileGuestInvocation(GuestCommand{
		Argv:        []string{"/bin/sh", "-c", `printf '%s\n%s\n%s' "$LITERAL" "$SECRET" "${AMBIENT-unset}"`},
		Environment: map[string]string{"LITERAL": literal, "SECRET": secret},
		Secrets:     []process.Secret{{Value: secret}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, argument := range argv {
		if strings.Contains(argument, literal) || strings.Contains(argument, secret) {
			t.Fatalf("guest argv contains transported environment value: %q", argv)
		}
	}

	result, err := (process.Runner{}).Run(context.Background(), process.Command{
		Argv: argv, Dir: t.TempDir(), Env: map[string]string{}, Secrets: []process.Secret{{Value: secret}}, Stdin: stdin,
	})
	if err != nil {
		t.Fatalf("execute guest bootstrap: %v", err)
	}
	want := literal + "\n[REDACTED]\nunset"
	if got := string(result.Stdout); got != want {
		t.Fatalf("bootstrap output = %q, want %q", got, want)
	}
}

func TestGuestInvocationRejectsSecretArgvAndInvalidEnvironment(t *testing.T) {
	secret := process.Secret{Value: "secret-canary"}
	for _, command := range []GuestCommand{
		{Argv: []string{"/bin/echo", secret.Value}, Secrets: []process.Secret{secret}},
		{Argv: []string{"/bin/true"}, Workdir: "/runtime/" + secret.Value, Secrets: []process.Secret{secret}},
		{Argv: []string{"/bin/true"}, Environment: map[string]string{"INVALID-NAME": "value"}},
		{Argv: []string{"/bin/true"}, Environment: map[string]string{"VALID": "nul\x00value"}},
		{Argv: []string{"/bin/true"}, Environment: map[string]string{"VALID": strings.Repeat("'", maxGuestEnvironmentBytes)}},
	} {
		if _, _, err := compileGuestInvocation(command); err == nil {
			t.Fatalf("compileGuestInvocation(%#v) succeeded", command)
		}
	}
}

func TestGuestInvocationIsDeterministic(t *testing.T) {
	command := GuestCommand{
		Argv: []string{"/bin/true"}, Environment: map[string]string{"ZED": "last", "ALPHA": "first"},
	}
	firstArgv, firstInput, err := compileGuestInvocation(command)
	if err != nil {
		t.Fatal(err)
	}
	secondArgv, secondInput, err := compileGuestInvocation(command)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(firstArgv, secondArgv) || !bytes.Equal(firstInput, secondInput) ||
		bytes.Index(firstInput, []byte("ALPHA")) > bytes.Index(firstInput, []byte("ZED")) {
		t.Fatalf("guest invocation is not deterministic: %q / %q", firstArgv, firstInput)
	}
}

func TestGuestInputsPreserveBytesAndCleanUpOnExit(t *testing.T) {
	const content = "services:\n  app: 'literal $HOME; $(false)'\nsecret-canary\n"
	for _, exitCode := range []int{0, 7} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			command := GuestCommand{Argv: []string{"/bin/sh", "-c", `printf '%s\n' "$1"; /bin/cat -- "$1"; exit "$EXIT_CODE"`, "probe", "unmounted.yaml"}, Environment: map[string]string{"EXIT_CODE": strconv.Itoa(exitCode)}, InputFiles: []GuestInputFile{{ArgIndex: 4, Data: []byte(content)}}, Secrets: []process.Secret{{Value: "secret-canary"}}}
			argv, stdin, err := compileGuestInvocation(command)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Join(argv, " "), "secret-canary") {
				t.Fatal("file contents leaked into argv")
			}
			result, runErr := (process.Runner{}).Run(t.Context(), process.Command{Argv: argv, Dir: t.TempDir(), Env: map[string]string{}, Stdin: stdin, Secrets: command.Secrets})
			if result.ExitCode != exitCode || (runErr == nil) != (exitCode == 0) {
				t.Fatalf("exit = %d, %v", result.ExitCode, runErr)
			}
			path, got, ok := strings.Cut(string(result.Stdout), "\n")
			if !ok || got != strings.ReplaceAll(content, "secret-canary", "[REDACTED]") {
				t.Fatalf("unexpected input output %q", result.Stdout)
			}
			if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatalf("guest input directory survived: %v", err)
			}
		})
	}
}

func TestGuestInputsRejectInvalidOrUnboundedPayloads(t *testing.T) {
	for _, files := range [][]GuestInputFile{
		{{ArgIndex: 0, Data: []byte("replace-executable")}},
		{{ArgIndex: 2, Data: []byte("outside-argv")}},
		{{ArgIndex: 1, Data: []byte("one")}, {ArgIndex: 1, Data: []byte("two")}},
		{{ArgIndex: 1, Data: []byte{'a', 0, 'b'}}},
		{{ArgIndex: 1, Data: make([]byte, MaxGuestInputBytes+1)}},
	} {
		if _, _, err := compileGuestInvocation(GuestCommand{Argv: []string{"/bin/cat", "input"}, InputFiles: files}); err == nil {
			t.Fatal("invalid input files were accepted")
		}
	}
}

func TestGuestInputsPreserveEnvironmentAndRestrictiveModes(t *testing.T) {
	command := GuestCommand{
		Argv:        []string{"/bin/sh", "-c", `set -eu; test "$stage" = literal; test "$(/usr/bin/stat -f '%Lp' "$1")" = 600; test "$(/usr/bin/stat -f '%Lp' "${1%/*}")" = 700`, "probe", "input"},
		Environment: map[string]string{"stage": "literal"},
		InputFiles:  []GuestInputFile{{ArgIndex: 4, Data: []byte("content")}},
	}
	if runtime.GOOS != "darwin" {
		command.Argv[2] = strings.ReplaceAll(command.Argv[2], "-f '%Lp'", "-c '%a'")
	}
	argv, stdin, err := compileGuestInvocation(command)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (process.Runner{}).Run(t.Context(), process.Command{Argv: argv, Dir: t.TempDir(), Env: map[string]string{}, Stdin: stdin})
	if err != nil {
		t.Fatalf("guest file/environment contract failed: %v (%s)", err, result.Stderr)
	}
}

func TestGuestInputsCleanUpOnCancellation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "stage-path")
	command := GuestCommand{
		Argv:       []string{"/bin/sh", "-c", `printf '%s' "${1%/*}" > "$2"; exec /bin/sleep 10`, "probe", "input", marker},
		InputFiles: []GuestInputFile{{ArgIndex: 4, Data: []byte("content")}},
	}
	argv, stdin, err := compileGuestInvocation(command)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, runErr := (process.Runner{}).Run(ctx, process.Command{Argv: argv, Dir: t.TempDir(), Env: map[string]string{}, Stdin: stdin})
		done <- runErr
	}()

	var stageDir string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(marker)
		if readErr == nil && len(data) > 0 {
			stageDir = string(data)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stageDir == "" {
		t.Fatal("guest input stage directory was never created")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled guest invocation succeeded")
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("guest input stage directory survived cancellation: %v", err)
	}
}
