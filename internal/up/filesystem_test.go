package up

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
)

func TestReconcileFilesystemPublishesOnlyFreshRetainedRuntime(t *testing.T) {
	attempt, _, runtimeObservation := composeReconcileFixture(t)
	composeResult := ComposeResult{
		Spec:        compose.RuntimeSpec{Plan: attempt.Plan, Generation: attempt.Generation, Workdir: attempt.BoxRoot},
		Records:     compose.RecordsFromObservation(runtimeObservation),
		Observation: runtimeObservation,
	}
	fresh := runtimeObservation
	fresh.Containers = append([]compose.ContainerObservation(nil), runtimeObservation.Containers...)
	fresh.Containers[0].State = "running"
	gate := &recordingFilesystemGate{observation: filesystem.Observation{Docker: fresh}}
	config := FilesystemConfig{Gate: gate, CatPath: "/usr/bin/cat", TestPath: "/usr/bin/test", CanaryRoot: "/tmp"}
	published := 0

	err := ReconcileFilesystem(t.Context(), config, attempt, composeResult, func(_ context.Context, result FilesystemResult) error {
		published++
		if !reflect.DeepEqual(result.Observation.Docker, fresh) || !reflect.DeepEqual(result.Compose.Observation, fresh) {
			t.Fatalf("published stale observation: %#v", result)
		}
		if !reflect.DeepEqual(result.Compose.Records, composeResult.Records) {
			t.Fatalf("published records = %#v, want %#v", result.Compose.Records, composeResult.Records)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gate.calls != 1 || published != 1 || !reflect.DeepEqual(gate.spec.Runtime, composeResult.Spec) {
		t.Fatalf("calls gate=%d publish=%d spec=%#v", gate.calls, published, gate.spec)
	}
}

func TestReconcileFilesystemRefusesStaleOrDriftedRuntimeIdentity(t *testing.T) {
	attempt, spec, runtimeObservation := composeReconcileFixture(t)
	records := compose.RecordsFromObservation(runtimeObservation)
	tests := []struct {
		name          string
		initial       compose.RuntimeObservation
		fresh         compose.RuntimeObservation
		wantGateCalls int
		want          string
	}{
		{
			name: "stale Compose result",
			initial: func() compose.RuntimeObservation {
				value := runtimeObservation
				value.Containers = append([]compose.ContainerObservation(nil), value.Containers...)
				value.Containers[0].ID = "stale-container"
				return value
			}(),
			fresh: runtimeObservation,
			want:  "prove Compose result",
		},
		{
			name:    "fresh observation drift",
			initial: runtimeObservation,
			fresh: func() compose.RuntimeObservation {
				value := runtimeObservation
				value.Containers = append([]compose.ContainerObservation(nil), value.Containers...)
				value.Containers[0].ID = "replacement-container"
				return value
			}(),
			wantGateCalls: 1,
			want:          "prove fresh runtime",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gate := &recordingFilesystemGate{observation: filesystem.Observation{Docker: test.fresh}}
			published := false
			err := ReconcileFilesystem(t.Context(), FilesystemConfig{Gate: gate}, attempt, ComposeResult{
				Spec: spec, Records: records, Observation: test.initial,
			}, func(context.Context, FilesystemResult) error {
				published = true
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReconcileFilesystem() error = %v, want %q", err, test.want)
			}
			if gate.calls != test.wantGateCalls || published {
				t.Fatalf("calls gate=%d publish=%t", gate.calls, published)
			}
		})
	}
}

func TestReconcileFilesystemPropagatesGateFailureAndRequiresDependencies(t *testing.T) {
	attempt, spec, runtimeObservation := composeReconcileFixture(t)
	composeResult := ComposeResult{Spec: spec, Records: compose.RecordsFromObservation(runtimeObservation), Observation: runtimeObservation}
	gateError := errors.New("injected readiness failure")
	gate := &recordingFilesystemGate{err: gateError}
	published := false

	err := ReconcileFilesystem(t.Context(), FilesystemConfig{Gate: gate}, attempt, composeResult, func(context.Context, FilesystemResult) error {
		published = true
		return nil
	})
	if !errors.Is(err, gateError) || published {
		t.Fatalf("gate failure = %v, published=%t", err, published)
	}
	if err := ReconcileFilesystem(t.Context(), FilesystemConfig{}, attempt, composeResult, func(context.Context, FilesystemResult) error { return nil }); err == nil {
		t.Fatal("ReconcileFilesystem(nil gate) succeeded")
	}
	if err := ReconcileFilesystem(t.Context(), FilesystemConfig{Gate: gate}, attempt, composeResult, nil); err == nil {
		t.Fatal("ReconcileFilesystem(nil publisher) succeeded")
	}
}

type recordingFilesystemGate struct {
	spec        filesystem.LiveSpec
	observation filesystem.Observation
	err         error
	calls       int
}

func (g *recordingFilesystemGate) VerifyAndPublish(ctx context.Context, spec filesystem.LiveSpec, publish filesystem.ReadyPublisher) error {
	g.calls++
	g.spec = spec
	if g.err != nil {
		return g.err
	}
	return publish(ctx, g.observation)
}
