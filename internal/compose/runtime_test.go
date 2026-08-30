package compose_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	aboxprocess "github.com/shoe-point-0/agent-box/internal/process"
)

func TestRuntimePullBuildAndPolicyPreflightCommandContract(t *testing.T) {
	compiled, outputDirectory, _ := fixturePlan(t, "2.39.1")
	baseline := []byte(`{"services":{"workspace":{},"worker":{}}}`)
	adapted := adaptedModel(t, compiled, nil)
	generated, err := compose.Compile(t.Context(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{"/usr/bin/docker", "compose"},
		Runner:         &fakeRunner{outputs: [][]byte{[]byte("2.39.1\n"), baseline, adapted}},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := compose.RuntimeSpec{
		Plan: compiled, Generation: generation, DockerPath: "/usr/bin/docker", Workdir: outputDirectory, Compiled: generated,
	}
	spec.PathEvidence, err = compose.RecordRuntimePaths(compiled, generated)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		run  func(*compose.Runtime) (aboxprocess.Result, error)
		want []string
	}{
		{"pull", func(runtime *compose.Runtime) (aboxprocess.Result, error) { return runtime.Pull(t.Context(), spec) }, []string{"pull", "--ignore-buildable"}},
		{"build", func(runtime *compose.Runtime) (aboxprocess.Result, error) { return runtime.Build(t.Context(), spec) }, []string{"build"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &contractRuntimeInvoker{normalized: generated.ResolvedModel}
			runtime, runtimeErr := compose.NewRuntime(invoker)
			if runtimeErr != nil {
				t.Fatal(runtimeErr)
			}
			if _, runErr := test.run(runtime); runErr != nil {
				t.Fatal(runErr)
			}
			if len(invoker.calls) != 2 {
				t.Fatalf("calls = %d, want preflight plus %s", len(invoker.calls), test.name)
			}
			assertRuntimePrefix(t, invoker.calls[0], spec)
			if !slices.Equal(invoker.calls[0][len(invoker.calls[0])-3:], []string{"config", "--format", "json"}) {
				t.Fatalf("preflight argv = %q", invoker.calls[0])
			}
			assertRuntimePrefix(t, invoker.calls[1], spec)
			if !slices.Equal(invoker.calls[1][len(invoker.calls[1])-len(test.want):], test.want) {
				t.Fatalf("%s argv = %q", test.name, invoker.calls[1])
			}
		})
	}

	var changed map[string]any
	if err := json.Unmarshal(generated.ResolvedModel, &changed); err != nil {
		t.Fatal(err)
	}
	changed["services"].(map[string]any)["workspace"].(map[string]any)["privileged"] = true
	drifted, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	invoker := &contractRuntimeInvoker{normalized: drifted}
	runtime, _ := compose.NewRuntime(invoker)
	if _, err := runtime.Pull(t.Context(), spec); err == nil || len(invoker.calls) != 1 {
		t.Fatalf("Pull(policy drift) error = %v calls = %d, want refusal before mutation", err, len(invoker.calls))
	}

	if err := os.Rename(generated.RootPath, generated.RootPath+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated.RootPath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invoker = &contractRuntimeInvoker{normalized: generated.ResolvedModel}
	runtime, _ = compose.NewRuntime(invoker)
	if _, err := runtime.Pull(t.Context(), spec); !errors.Is(err, ownership.ErrNotOwned) || len(invoker.calls) != 0 {
		t.Fatalf("Pull(replaced path) error = %v calls = %d, want ownership refusal before guest invocation", err, len(invoker.calls))
	}
}

type contractRuntimeInvoker struct {
	normalized []byte
	calls      [][]string
}

func (i *contractRuntimeInvoker) Invoke(_ context.Context, _ lima.Spec, _ string, argv []string, _, _ io.Writer) (aboxprocess.Result, error) {
	i.calls = append(i.calls, append([]string(nil), argv...))
	if slices.Contains(argv, "config") {
		return aboxprocess.Result{ExitCode: 0, Stdout: append([]byte(nil), i.normalized...)}, nil
	}
	return aboxprocess.Result{ExitCode: 0}, nil
}

func assertRuntimePrefix(t *testing.T, argv []string, spec compose.RuntimeSpec) {
	t.Helper()
	want := []string{spec.DockerPath, "compose", "--project-name", spec.Plan.Compose.ProjectName}
	for _, profile := range spec.Plan.Compose.Profiles {
		want = append(want, "--profile", profile)
	}
	want = append(want, "-f", spec.Compiled.RootPath, "-f", spec.Compiled.OverridePath)
	if len(argv) < len(want) || !slices.Equal(argv[:len(want)], want) {
		t.Fatalf("runtime argv prefix = %q, want %q", argv, want)
	}
}
