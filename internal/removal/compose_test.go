package removal_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

func TestComposeStepFeedsExactOwnedVolumesIntoDefaultRefusal(t *testing.T) {
	t.Parallel()
	observed := composeRemovalObservation()
	runtime := &stubComposeRuntime{observation: observed}
	step := newComposeRemovalStep(t, runtime, observed)
	log := []string{}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: fakeGuard{&log}, Quiescer: fakeQuiescer{&log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(preview.Losses, []removal.Loss{{Kind: removal.LossVolume, Value: "volume-one"}}) {
		t.Fatalf("Compose losses = %#v", preview.Losses)
	}
	if preview.Resources[0].Evidence["container.0"] == "" || preview.Resources[0].Evidence["network.0"] == "" ||
		preview.Resources[0].Evidence["volume.0"] == "" || preview.Resources[0].Evidence["volumePolicy"] == "" {
		t.Fatalf("Compose evidence is incomplete: %#v", preview.Resources[0].Evidence)
	}
	log = nil
	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest})
	if !errors.Is(err, removal.ErrSafetyRefusal) || runtime.removes != 0 || len(log) != 0 {
		t.Fatalf("default Execute = %v, removes = %d, lifecycle = %v", err, runtime.removes, log)
	}
}

func TestComposeStepRefusesLabelOrVolumeDriftBeforeRuntimeRemoval(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*compose.RuntimeObservation)
	}{
		{"ownership label", func(observed *compose.RuntimeObservation) {
			observed.Volumes[0].Labels = cloneComposeLabels(observed.Volumes[0].Labels)
			observed.Volumes[0].Labels[ownership.ComposeBoxIDFact] = "replacement-box"
		}},
		{"volume inventory", func(observed *compose.RuntimeObservation) { observed.Volumes = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := composeRemovalObservation()
			runtime := &stubComposeRuntime{observation: initial}
			step := newComposeRemovalStep(t, runtime, initial)
			expected, err := step.Observe(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			changed := initial
			test.mutate(&changed)
			runtime.observation = changed
			err = step.Remove(t.Context(), expected)
			if !errors.Is(err, removal.ErrInventoryDrift) && !errors.Is(err, ownership.ErrNotOwned) {
				t.Fatalf("Remove(drift) error = %v", err)
			}
			if runtime.removes != 0 {
				t.Fatalf("runtime Remove calls = %d", runtime.removes)
			}
		})
	}
}

func TestComposeRemovalRecordsAllowOnlyOwnedRemainingSubset(t *testing.T) {
	t.Parallel()
	complete := composeRemovalObservation()
	records := compose.RecordsFromObservation(complete)
	remaining := complete
	remaining.Containers = nil
	if err := compose.VerifyRemovalRecords(records, remaining); err != nil {
		t.Fatalf("VerifyRemovalRecords(remaining subset) error = %v", err)
	}
	absent := compose.RuntimeObservation{ProjectName: complete.ProjectName}
	if err := compose.VerifyRemovalRecords(records, absent); err != nil {
		t.Fatalf("VerifyRemovalRecords(absent) error = %v", err)
	}
	replacement := remaining
	replacement.Networks = append([]compose.NetworkObservation(nil), remaining.Networks...)
	replacement.Networks[0].ID = "replacement-network"
	if err := compose.VerifyRemovalRecords(records, replacement); err == nil {
		t.Fatal("VerifyRemovalRecords accepted an unrecorded replacement")
	}
}

func newComposeRemovalStep(t *testing.T, runtime *stubComposeRuntime, observed compose.RuntimeObservation) *removal.ComposeStep {
	t.Helper()
	project := observed.ProjectName
	step, err := removal.NewComposeStep(removal.ComposeStepConfig{
		Spec: compose.RuntimeSpec{
			Plan:     contract.Plan{Compose: contract.ComposePlan{ProjectName: project}},
			Compiled: compose.Result{ProjectName: project},
		},
		Records: compose.RecordsFromObservation(observed), Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func composeRemovalObservation() compose.RuntimeObservation {
	labels := map[string]string{
		"com.docker.compose.project": "project-one", ownership.ComposeBoxIDFact: "box-one",
		"dev.abox.workspace": "workspace-one", "dev.abox.plan": "sha256:plan", "dev.abox.generation": "1",
	}
	return compose.RuntimeObservation{
		ProjectName: "project-one",
		Containers:  []compose.ContainerObservation{{ID: "container-one", Name: "project-one-workspace-1", Labels: cloneComposeLabels(labels)}},
		Networks:    []compose.NetworkObservation{{ID: "network-one", Name: "project-one_default", Labels: cloneComposeLabels(labels)}},
		Volumes:     []compose.VolumeObservation{{ID: "volume-one", Name: "volume-one", Driver: "local", Labels: cloneComposeLabels(labels)}},
	}
}

func cloneComposeLabels(labels map[string]string) map[string]string {
	clone := make(map[string]string, len(labels))
	for key, value := range labels {
		clone[key] = value
	}
	return clone
}

type stubComposeRuntime struct {
	observation compose.RuntimeObservation
	removes     int
}

func (s *stubComposeRuntime) Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	return s.observation, nil
}

func (s *stubComposeRuntime) Remove(context.Context, compose.RuntimeSpec, compose.RuntimeRecords) (compose.RuntimeObservation, error) {
	s.removes++
	return compose.RuntimeObservation{ProjectName: s.observation.ProjectName}, nil
}
