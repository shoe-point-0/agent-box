package main

import (
	"bytes"
	"strings"
	"testing"
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
			if got := stdout.String(); !strings.HasPrefix(got, "abox devel (commit unknown, built unknown)\n") {
				t.Fatalf("run(%q) stdout = %q, want development build identity", argument, got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("run(%q) stderr = %q, want empty", argument, stderr.String())
			}
		})
	}
}

func TestRunRejectsUnimplementedCommands(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"up"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(up) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run(up) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "abox: no product commands are implemented yet; use --version\n"; got != want {
		t.Fatalf("run(up) stderr = %q, want %q", got, want)
	}
}
