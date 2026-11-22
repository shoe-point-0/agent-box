package machinegit

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDiagnosticClientUsesFixedReadOnlyProfile(t *testing.T) {
	root := t.TempDir()
	gitPath := filepath.Join(root, "git")
	if err := os.WriteFile(gitPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	executor := &recordingExecutor{}
	client, err := NewDiagnosticClient(root, root, root, executor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Observe(t.Context(), ProbeVersion); err != nil {
		t.Fatal(err)
	}
	gitPath, err = filepath.EvalSymlinks(gitPath)
	if err != nil {
		t.Fatal(err)
	}
	command := executor.last
	if command.Argv[0] != gitPath || command.Argv[len(command.Argv)-1] != "--version" {
		t.Fatalf("version argv = %#v", command.Argv)
	}
	joined := strings.Join(command.Argv, "\x00")
	for _, required := range []string{"core.hooksPath=/dev/null", "core.fsmonitor=false", "credential.helper=", "protocol.allow=never"} {
		if !strings.Contains(joined, required) {
			t.Errorf("diagnostic profile lacks %q: %#v", required, command.Argv)
		}
	}
	for name, want := range map[string]string{
		"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null", "GIT_OPTIONAL_LOCKS": "0", "GIT_TERMINAL_PROMPT": "0",
	} {
		if command.Env[name] != want {
			t.Errorf("%s = %q, want %q", name, command.Env[name], want)
		}
	}

	if _, err := client.Observe(t.Context(), ProbeRelativeWorktrees); err != nil {
		t.Fatal(err)
	}
	if got := executor.last.Argv[len(executor.last.Argv)-3:]; !slices.Equal(got, []string{"worktree", "add", "-h"}) {
		t.Fatalf("relative-worktree argv suffix = %#v", got)
	}
	calls := executor.calls
	if _, err := client.Observe(context.Background(), DiagnosticProbe("mutate")); err == nil || executor.calls != calls {
		t.Fatalf("unsupported probe error/calls = %v/%d, before %d", err, executor.calls, calls)
	}
}
