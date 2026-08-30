package lima

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

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
