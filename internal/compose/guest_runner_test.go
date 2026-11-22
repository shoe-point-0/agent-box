package compose_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestGuestRunnerBindsFreshRuntimeAndStructuredCommand(t *testing.T) {
	spec := lima.Spec{RuntimeHost: contract.RuntimeHostPlan{InstanceName: "box-runtime"}}
	invoker := &recordingGuestInvoker{result: process.Result{
		Stdout: []byte("stdout"), Stderr: []byte("stderr"), StdoutTruncated: true,
	}}
	runner, err := compose.NewGuestRunner(invoker, spec)
	if err != nil {
		t.Fatal(err)
	}
	command := compose.Command{
		Argv: []string{"/usr/bin/docker", "compose", "version"}, Dir: "/runtime/box",
		Env: map[string]string{"HOME": "/runtime/home"}, Secrets: []compose.Secret{{Value: "canary"}},
	}
	result, err := runner.Run(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	want := lima.GuestCommand{
		Argv: command.Argv, Workdir: command.Dir, Environment: command.Env,
		Secrets: []process.Secret{{Value: "canary"}},
	}
	if !reflect.DeepEqual(invoker.spec, spec) || !reflect.DeepEqual(invoker.command, want) {
		t.Fatalf("InvokeCommand() spec/command = %#v/%#v, want %#v/%#v", invoker.spec, invoker.command, spec, want)
	}
	if string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" || !result.StdoutTruncated {
		t.Fatalf("Run() result = %#v", result)
	}
	command.Argv[0] = "mutated"
	command.Env["HOME"] = "mutated"
	if invoker.command.Argv[0] != "/usr/bin/docker" || invoker.command.Environment["HOME"] != "/runtime/home" {
		t.Fatal("GuestRunner retained caller-owned command storage")
	}
}

func TestGuestRunnerPreservesInvokerErrorAndPartialResult(t *testing.T) {
	wantErr := errors.New("injected guest failure")
	invoker := &recordingGuestInvoker{result: process.Result{Stderr: []byte("partial")}, err: wantErr}
	runner, err := compose.NewGuestRunner(invoker, lima.Spec{RuntimeHost: contract.RuntimeHostPlan{InstanceName: "box-runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(t.Context(), compose.Command{Argv: []string{"/bin/false"}, Dir: "/runtime"})
	if !errors.Is(err, wantErr) || string(result.Stderr) != "partial" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
}

type recordingGuestInvoker struct {
	spec    lima.Spec
	command lima.GuestCommand
	result  process.Result
	err     error
}

func (i *recordingGuestInvoker) InvokeCommand(_ context.Context, spec lima.Spec, command lima.GuestCommand) (process.Result, error) {
	i.spec = spec
	i.command = command
	return i.result, i.err
}

func TestGuestRunnerTransfersOnlyOwnedGeneratedFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, compose.RootFileName)
	content := []byte("include: []\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	invoker := &recordingGuestInvoker{}
	runner, err := compose.NewGuestRunner(invoker, lima.Spec{RuntimeHost: contract.RuntimeHostPlan{InstanceName: "runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	command := compose.Command{Argv: []string{"/usr/bin/docker", "compose", "-f", path, "config"}, Dir: root}
	if _, err := runner.Run(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if len(invoker.command.InputFiles) != 1 || invoker.command.InputFiles[0].ArgIndex != 3 || string(invoker.command.InputFiles[0].Data) != string(content) {
		t.Fatalf("input transfer = %#v", invoker.command.InputFiles)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	invoker.command = lima.GuestCommand{}
	if _, err := runner.Run(t.Context(), command); err == nil || len(invoker.command.Argv) != 0 {
		t.Fatal("unsafe input reached the guest")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "other.yaml")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(t.Context(), command); err == nil {
		t.Fatal("symlinked input accepted")
	}
}
