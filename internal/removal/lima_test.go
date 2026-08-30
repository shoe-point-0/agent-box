package removal_test

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

func TestLimaStepFeedsExactOwnedInstanceIntoDefaultRefusal(t *testing.T) {
	t.Parallel()
	observed := limaRemovalObservation()
	runtime := &stubLimaRuntime{observation: observed}
	step := newLimaRemovalStep(t, runtime)
	log := []string{}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: fakeGuard{&log}, Quiescer: fakeQuiescer{&log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), observed.Name)
	if err != nil {
		t.Fatal(err)
	}
	got := preview.Resources[0]
	if !got.Present || got.Evidence["name"] != observed.Name || got.Evidence["state"] != string(lima.Running) ||
		got.Evidence["limaStatus"] != "Running" || got.Evidence["limaVersion"] != lima.SupportedVersion || got.Evidence["metadata"] == "" {
		t.Fatalf("Lima evidence is incomplete: %#v", got)
	}
	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest})
	if !errors.Is(err, removal.ErrSafetyRefusal) || runtime.deletes != 0 || len(log) != 0 {
		t.Fatalf("default Execute = %v, deletes = %d, lifecycle = %v", err, runtime.deletes, log)
	}
}

func TestLimaStepRefusesStateOrMetadataDriftBeforeDelete(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*lima.Observation)
	}{
		{"state", func(observed *lima.Observation) { observed.State, observed.LimaStatus = lima.Stopped, "Stopped" }},
		{"metadata", func(observed *lima.Observation) {
			observed.Metadata = maps.Clone(observed.Metadata)
			observed.Metadata[ownership.BoxIDFact] = "replacement-box"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := limaRemovalObservation()
			runtime := &stubLimaRuntime{observation: initial}
			step := newLimaRemovalStep(t, runtime)
			expected, err := step.Observe(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			changed := cloneLimaObservation(initial)
			test.mutate(&changed)
			runtime.observation = changed
			if err := step.Remove(t.Context(), expected); !errors.Is(err, removal.ErrInventoryDrift) {
				t.Fatalf("Remove(drift) error = %v", err)
			}
			if runtime.deletes != 0 {
				t.Fatalf("Delete calls = %d", runtime.deletes)
			}
		})
	}
}

func TestLimaStepInterruptedDeleteConvergesFromFreshPreview(t *testing.T) {
	t.Parallel()
	runtime := &stubLimaRuntime{observation: limaRemovalObservation(), deleteErr: errors.New("connection lost after delete")}
	step := newLimaRemovalStep(t, runtime)
	log := []string{}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: fakeGuard{&log}, Quiescer: fakeQuiescer{&log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), runtime.observation.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest, Yes: true})
	if err == nil || runtime.deletes != 1 {
		t.Fatalf("interrupted Execute = %v, deletes = %d", err, runtime.deletes)
	}

	retry, err := coordinator.Preview(t.Context(), preview.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Resources[0].Present {
		t.Fatalf("retry preview still reports instance: %#v", retry.Resources[0])
	}
	result, err := coordinator.Execute(t.Context(), retry, removal.Authorization{PreviewDigest: retry.Digest})
	if err != nil || len(result.Remaining) != 0 || runtime.deletes != 1 {
		t.Fatalf("retry Execute = %#v, %v, deletes = %d", result, err, runtime.deletes)
	}
}

func newLimaRemovalStep(t *testing.T, runtime *stubLimaRuntime) *removal.LimaStep {
	t.Helper()
	name := "box-one"
	step, err := removal.NewLimaStep(removal.LimaStepConfig{
		Spec: lima.Spec{RuntimeHost: contract.RuntimeHostPlan{InstanceName: name}, BoxID: name},
		Record: ownership.LimaInstanceRecord{Name: name, Metadata: map[string]string{
			ownership.BoxIDFact: name, "dev.abox.plan-digest": "sha256:plan", "dev.abox.generation": "1",
		}},
		Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func limaRemovalObservation() lima.Observation {
	return lima.Observation{
		Name: "box-one", State: lima.Running, LimaStatus: "Running", LimaVersion: lima.SupportedVersion,
		Metadata: map[string]string{
			ownership.BoxIDFact: "box-one", "dev.abox.plan-digest": "sha256:plan", "dev.abox.generation": "1",
		},
	}
}

func cloneLimaObservation(observed lima.Observation) lima.Observation {
	observed.Metadata = maps.Clone(observed.Metadata)
	return observed
}

type stubLimaRuntime struct {
	observation lima.Observation
	deleteErr   error
	deletes     int
}

func (s *stubLimaRuntime) ObserveRemoval(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error) {
	return cloneLimaObservation(s.observation), nil
}

func (s *stubLimaRuntime) Delete(_ context.Context, _ lima.Spec, _ ownership.LimaInstanceRecord) (lima.Observation, error) {
	s.deletes++
	name := s.observation.Name
	s.observation = lima.Observation{Name: name, State: lima.Absent}
	return cloneLimaObservation(s.observation), s.deleteErr
}
