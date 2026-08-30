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
	guest := GuestResult{}
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
		guest: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult) (GuestResult, error) {
			calls = append(calls, "guest")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) {
				t.Fatal("guest did not receive fresh foundation result")
			}
			return guest, nil
		},
		composeInputs: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult, gotGuest GuestResult) (compose.RuntimeSpec, error) {
			calls = append(calls, "compose-inputs")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) || !reflect.DeepEqual(gotGuest, guest) {
				t.Fatal("Compose inputs did not receive fresh prerequisite results")
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
		ready: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult, gotGuest GuestResult, gotResult ComposeResult) error {
			calls = append(calls, "ready")
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) || !reflect.DeepEqual(gotGuest, guest) || !reflect.DeepEqual(gotResult, result) {
				t.Fatal("READY did not receive complete fresh results")
			}
			return nil
		},
	}

	if err := pipeline.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "foundation", "guest", "compose-inputs", "compose-apply", "ready"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("pipeline calls = %v, want %v", calls, want)
	}
}

func TestPipelineStopsAtFirstFailedBoundary(t *testing.T) {
	stages := []string{"prepare", "foundation", "guest", "compose-inputs", "compose-apply", "ready"}
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
				guest: func(context.Context, PreparedAttempt, FoundationResult) (GuestResult, error) {
					return GuestResult{}, fail("guest")
				},
				composeInputs: func(context.Context, PreparedAttempt, FoundationResult, GuestResult) (compose.RuntimeSpec, error) {
					return compose.RuntimeSpec{}, fail("compose-inputs")
				},
				composeApply: func(context.Context, PreparedAttempt, compose.RuntimeSpec) (ComposeResult, error) {
					return ComposeResult{}, fail("compose-apply")
				},
				ready: func(context.Context, PreparedAttempt, FoundationResult, GuestResult, ComposeResult) error {
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

func TestNewPipelineDelaysRuntimeSpecificBuilders(t *testing.T) {
	if _, err := NewPipeline(PipelineConfig{}); err == nil || !strings.Contains(err.Error(), "guest config builder") {
		t.Fatalf("NewPipeline() missing guest builder error = %v", err)
	}
	if _, err := NewPipeline(PipelineConfig{
		BuildGuest: func(context.Context, PreparedAttempt, FoundationResult) (GuestConfig, error) {
			return GuestConfig{}, nil
		},
	}); err == nil || !strings.Contains(err.Error(), "Compose config builder") {
		t.Fatalf("NewPipeline() missing Compose builder error = %v", err)
	}
	if _, err := NewPipeline(PipelineConfig{
		BuildGuest: func(context.Context, PreparedAttempt, FoundationResult) (GuestConfig, error) {
			return GuestConfig{}, nil
		},
		BuildCompose: func(context.Context, PreparedAttempt, FoundationResult, GuestResult) (ComposeConfig, error) {
			return ComposeConfig{}, nil
		},
	}); err == nil || !strings.Contains(err.Error(), "READY config builder") {
		t.Fatalf("NewPipeline() missing READY builder error = %v", err)
	}

	attempt := PreparedAttempt{Operation: journal.Operation{ID: "operation-build-compose"}}
	foundation := FoundationResult{Lima: LimaResult{}}
	wantErr := errors.New("injected runtime binding failure")
	pipeline, err := NewPipeline(PipelineConfig{
		BuildGuest: func(context.Context, PreparedAttempt, FoundationResult) (GuestConfig, error) {
			return GuestConfig{}, nil
		},
		BuildCompose: func(_ context.Context, got PreparedAttempt, gotFoundation FoundationResult, _ GuestResult) (ComposeConfig, error) {
			if !reflect.DeepEqual(got, attempt) || !reflect.DeepEqual(gotFoundation, foundation) {
				t.Fatal("Compose builder did not receive fresh foundation evidence")
			}
			return ComposeConfig{}, wantErr
		},
		BuildReady: func(context.Context, PreparedAttempt, FoundationResult, GuestResult, ComposeResult) (ReadyConfig, error) {
			return ReadyConfig{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.composeInputs(t.Context(), attempt, foundation, GuestResult{}); !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "build Compose config") {
		t.Fatalf("composeInputs() error = %v, want wrapped builder failure", err)
	}

	if _, err := AssemblePipelineCoordinator(Config{}, nil); err == nil || !strings.Contains(err.Error(), "pipeline is required") {
		t.Fatalf("AssemblePipelineCoordinator() error = %v", err)
	}
}
