package toolchain

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/process"
)

type diagnosticRunner struct {
	calls []process.Command
}

func (runner *diagnosticRunner) Run(_ context.Context, command process.Command) (process.Result, error) {
	runner.calls = append(runner.calls, command)
	return process.Result{ExitCode: 0, Stdout: []byte("test\n")}, nil
}

func TestDiagnosticsExposeOnlyFixedReadOnlyProbes(t *testing.T) {
	root := t.TempDir()
	diagnostics, err := NewDiagnostics(root, root, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &diagnosticRunner{}
	diagnostics.runner = runner
	tests := []struct {
		probe DiagnosticProbe
		argv  []string
	}{
		{ProbeLimaVersion, []string{"limactl", "--version"}},
		{ProbeDockerVersion, []string{"docker", "--version"}},
		{ProbeComposeVersion, []string{"docker", "compose", "version", "--short"}},
		{ProbeDockerRuntime, []string{"docker", "version", "--format", "{{.Client.Version}}/{{.Server.Version}}"}},
	}
	for _, test := range tests {
		if _, err := diagnostics.Observe(t.Context(), test.probe); err != nil {
			t.Fatalf("Observe(%s) error = %v", test.probe, err)
		}
		command := runner.calls[len(runner.calls)-1]
		if !slices.Equal(command.Argv, test.argv) || command.Dir != root || command.Env["HOME"] != root || command.Env["PATH"] != root {
			t.Fatalf("Observe(%s) command = %#v", test.probe, command)
		}
	}
	calls := len(runner.calls)
	if _, err := diagnostics.Observe(t.Context(), DiagnosticProbe("apply")); err == nil || len(runner.calls) != calls {
		t.Fatalf("unsupported probe error/calls = %v/%d, before %d", err, len(runner.calls), calls)
	}
	if _, err := NewDiagnostics("relative", root, filepath.Clean(root)); err == nil {
		t.Fatal("NewDiagnostics accepted a relative PATH")
	}
}

func TestDiagnosticsKeepsStrictPathValidation(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name string
		path string
	}{
		{"unset", ""},
		{"leading empty", ":" + root},
		{"middle empty", root + "::" + root},
		{"trailing empty", root + ":"},
		{"current directory", root + ":."},
		{"relative directory", root + ":private-relative-bin"},
		{"trailing slash", root + "/"},
		{"unclean directory", root + "/../bin"},
		{"control character", root + "\nprivate-path-value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostics, err := NewDiagnostics(test.path, root, root)
			if err == nil || diagnostics != nil {
				t.Fatal("unsafe PATH accepted")
			}
			if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), "private-") {
				t.Fatal("diagnostic error disclosed a PATH value")
			}
		})
	}
}

func TestDiagnosticsExecutesFromAbsolutePathContainingSpaces(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "tools with spaces")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "limactl"), []byte("#!/bin/sh\nprintf 'limactl version 1.0.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := NewDiagnostics(bin, root, root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := diagnostics.Observe(t.Context(), ProbeLimaVersion)
	if err != nil || string(result.Stdout) != "limactl version 1.0.0\n" {
		t.Fatalf("space-containing PATH probe = %q, %v", result.Stdout, err)
	}
}
