package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/spike"
)

func TestRunListsHarnessSmoke(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(list) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "compose-include\ngit-path\nharness-smoke\n"; got != want {
		t.Fatalf("run(list) stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run(list) stderr = %q, want empty", stderr.String())
	}
}

func TestRunExecutesInspectsAndCleansSelectedSpike(t *testing.T) {
	t.Parallel()

	registry := spike.NewRegistry()
	if err := registry.Register("fixture", func(_ context.Context, env *spike.Environment) error {
		_, err := env.WriteFile("fixture.txt", []byte("owned\n"))
		return err
	}); err != nil {
		t.Fatalf("Register(fixture) error = %v", err)
	}

	parent := t.TempDir()
	var runOutput, runErrors bytes.Buffer
	if code := runWithRegistry(context.Background(), []string{"run", "--parent", parent, "fixture"}, &runOutput, &runErrors, registry); code != 0 {
		t.Fatalf("run(fixture) code = %d, want 0; stderr = %q", code, runErrors.String())
	}
	var result spike.RunResult
	if err := json.Unmarshal(runOutput.Bytes(), &result); err != nil {
		t.Fatalf("decode run result: %v; output = %q", err, runOutput.String())
	}
	if result.Status != "passed" || result.Root == "" {
		t.Fatalf("run result = %+v, want passed retained root", result)
	}
	if _, err := os.Stat(result.DiagnosticsDir); err != nil {
		t.Fatalf("Stat(diagnostics) error = %v", err)
	}

	var inspectOutput, inspectErrors bytes.Buffer
	if code := runWithRegistry(context.Background(), []string{"inspect", result.Root}, &inspectOutput, &inspectErrors, registry); code != 0 {
		t.Fatalf("inspect code = %d, want 0; stderr = %q", code, inspectErrors.String())
	}
	var inspection spike.Inspection
	if err := json.Unmarshal(inspectOutput.Bytes(), &inspection); err != nil {
		t.Fatalf("decode inspection: %v", err)
	}
	if inspection.Result == nil || inspection.Result.Status != "passed" || len(inspection.Inventory) != 2 {
		t.Fatalf("inspection = %+v, want result and planned/created inventory", inspection)
	}

	var cleanupOutput, cleanupErrors bytes.Buffer
	if code := runWithRegistry(context.Background(), []string{"cleanup", result.Root}, &cleanupOutput, &cleanupErrors, registry); code != 0 {
		t.Fatalf("cleanup code = %d, want 0; stderr = %q", code, cleanupErrors.String())
	}
	if _, err := os.Stat(result.Root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(cleaned root) error = %v, want not exist", err)
	}
}

func TestRunReturnsMachineReadableFailureAndRetainsRoot(t *testing.T) {
	t.Parallel()

	registry := spike.NewRegistry()
	if err := registry.Register("failure", func(context.Context, *spike.Environment) error {
		return errors.New("fixture failed")
	}); err != nil {
		t.Fatalf("Register(failure) error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := runWithRegistry(context.Background(), []string{"run", "--parent", t.TempDir(), "failure"}, &stdout, &stderr, registry); code != 1 {
		t.Fatalf("run(failure) code = %d, want 1", code)
	}
	var result spike.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode failed result: %v", err)
	}
	if result.Status != "failed" || result.Error != "fixture failed" {
		t.Fatalf("failed result = %+v", result)
	}
	if _, err := os.Stat(result.Root); err != nil {
		t.Fatalf("failed run root was not retained: %v", err)
	}
	if err := spike.CleanupWorkspace(result.Root); err != nil {
		t.Fatalf("CleanupWorkspace(failed root) error = %v", err)
	}
}

func TestRunRejectsUnknownArguments(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(unknown) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run(unknown) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "usage: abox-spike list [--json] | run [--parent DIR] [--max-output BYTES] <name> | inspect <root> | cleanup <root>\n"; got != want {
		t.Fatalf("run(unknown) stderr = %q, want %q", got, want)
	}
}
