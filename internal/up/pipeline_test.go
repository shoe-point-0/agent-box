package up

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/journal"
)

func TestPipelineExecutesCompleteDependencyOrder(t *testing.T) {
	operation := journal.Operation{ID: "operation-pipeline", BoxID: "box-pipeline", Kind: "up", State: journal.StateRunning}
	attempt := PreparedAttempt{Operation: operation}
	foundation := FoundationResult{}
	spec := compose.RuntimeSpec{DockerPath: "/usr/bin/docker"}
	result := ComposeResult{Spec: spec}
	var calls []string
	pipeline := &Pipeline{
		prepare: func(_ context.Context, got journal.Operation) (PreparedAttempt, error) {
			calls = append(calls, "prepare")
			if got != operation {
				t.Fatalf("prepare operation = %#v", got)
			}
			return attempt, nil
		},
		foundation: func(_ context.Context, got PreparedAttempt) (FoundationResult, error) {
			calls = append(calls, "foundation")
			if !reflect.DeepEqual(got, attempt) {
				t.Fatal("foundation did not receive prepared attempt")
			}
			return foundation, nil
		},
		composeInputs: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult) (compose.RuntimeSpec, error) {
			calls = append(calls, "compose-inputs")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) {
				t.Fatal("Compose inputs did not receive fresh foundation result")
			}
			return spec, nil
		},
		composeApply: func(_ context.Context, got PreparedAttempt, gotSpec compose.RuntimeSpec) (ComposeResult, error) {
			calls = append(calls, "compose-apply")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotSpec, spec) {
				t.Fatal("Compose apply did not receive freshly compiled spec")
			}
			return result, nil
		},
		ready: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult, gotResult ComposeResult) error {
			calls = append(calls, "ready")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) || !reflect.DeepEqual(gotResult, result) {
				t.Fatal("READY did not receive complete fresh results")
			}
			return nil
		},
	}

	if err := pipeline.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "foundation", "compose-inputs", "compose-apply", "ready"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("pipeline calls = %v, want %v", calls, want)
	}
}

func TestPipelineStopsAtFirstFailedBoundary(t *testing.T) {
	stages := []string{"prepare", "foundation", "compose-inputs", "compose-apply", "ready"}
	for failedIndex, failedStage := range stages {
		t.Run(failedStage, func(t *testing.T) {
			var calls []string
			fail := func(stage string) error {
				calls = append(calls, stage)
				if stage == failedStage {
					return errors.New("injected " + stage + " failure")
				}
				return nil
			}
			pipeline := &Pipeline{
				prepare: func(context.Context, journal.Operation) (PreparedAttempt, error) {
					return PreparedAttempt{}, fail("prepare")
				},
				foundation: func(context.Context, PreparedAttempt) (FoundationResult, error) {
					return FoundationResult{}, fail("foundation")
				},
				composeInputs: func(context.Context, PreparedAttempt, FoundationResult) (compose.RuntimeSpec, error) {
					return compose.RuntimeSpec{}, fail("compose-inputs")
				},
				composeApply: func(context.Context, PreparedAttempt, compose.RuntimeSpec) (ComposeResult, error) {
					return ComposeResult{}, fail("compose-apply")
				},
				ready: func(context.Context, PreparedAttempt, FoundationResult, ComposeResult) error {
					return fail("ready")
				},
			}
			err := pipeline.Execute(t.Context(), journal.Operation{})
			if err == nil || !strings.Contains(err.Error(), "injected "+failedStage+" failure") {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := stages[:failedIndex+1]; !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls after %s failure = %v, want %v", failedStage, calls, want)
			}
		})
	}
}

func TestNewPipelineRequiresRuntimeSpecificReadyBuilder(t *testing.T) {
	if _, err := NewPipeline(PipelineConfig{}); err == nil || !strings.Contains(err.Error(), "READY config builder") {
		t.Fatalf("NewPipeline() error = %v", err)
	}
	if _, err := AssemblePipelineCoordinator(Config{}, nil); err == nil || !strings.Contains(err.Error(), "pipeline is required") {
		t.Fatalf("AssemblePipelineCoordinator() error = %v", err)
	}
}
