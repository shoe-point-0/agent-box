package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

func TestRunReportsVersion(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{"--version", "version"} {
		argument := argument
		t.Run(argument, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run([]string{argument}, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%q) code = %d, want 0; stderr = %q", argument, code, stderr.String())
			}
			if got := stdout.String(); !strings.HasPrefix(got, "aboxd devel (commit unknown, built unknown)\n") {
				t.Fatalf("run(%q) stdout = %q, want development build identity", argument, got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("run(%q) stderr = %q, want empty", argument, stderr.String())
			}
		})
	}
}

func TestRunRejectsDaemonStart(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != contract.ExitUsage {
		t.Fatalf("run() code = %d, want %d", code, contract.ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "aboxd: the daemon is not implemented yet; use --version\n"; got != want {
		t.Fatalf("run() stderr = %q, want %q", got, want)
	}
}
