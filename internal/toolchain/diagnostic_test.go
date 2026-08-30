package toolchain

import (
	"context"
	"path/filepath"
	"slices"
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
