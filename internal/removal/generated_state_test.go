package removal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

func TestGeneratedStateStepBindsExactArtifactsAndRefusesDrift(t *testing.T) {
	t.Parallel()
	observation := generatedstate.Observation{
		BoxID: "box-one", Root: "/state/boxes/box-one", RootIdentity: ownership.FileIdentity{Device: 7, Inode: 11},
		Artifacts: []generatedstate.Artifact{{
			Path: "/state/boxes/box-one/context.json", Identity: ownership.FileIdentity{Device: 7, Inode: 12}, Kind: "context",
		}},
		Present: true,
	}
	runtime := &stubGeneratedState{observation: observation}
	step, err := removal.NewGeneratedStateStep(removal.GeneratedStateStepConfig{
		Plan: contract.Plan{BoxID: "box-one", PlanDigest: "sha256:plan"},
		Record: generatedstate.Record{
			BoxID: "box-one", Root: ownership.PathRecord{CanonicalPath: observation.Root, Identity: observation.RootIdentity},
		},
		Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := step.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !fact.Present || fact.Evidence["rootIdentity"] != "7:11" || fact.Evidence["artifacts"] == "" ||
		fact.Evidence["retainedState"] != "plan.json,diagnostics" {
		t.Fatalf("fact = %#v", fact)
	}

	runtime.observation.Artifacts[0].Identity.Inode++
	if err := step.Remove(t.Context(), fact); !errors.Is(err, removal.ErrInventoryDrift) {
		t.Fatalf("Remove(drift) error = %v", err)
	}
	if runtime.removes != 0 {
		t.Fatalf("remove calls = %d", runtime.removes)
	}
}

func TestGeneratedStateStepFreshAbsentObservationConvergesWithoutReplay(t *testing.T) {
	t.Parallel()
	root := "/state/boxes/box-one"
	runtime := &stubGeneratedState{observation: generatedstate.Observation{BoxID: "box-one", Root: root}}
	step, err := removal.NewGeneratedStateStep(removal.GeneratedStateStepConfig{
		Plan: contract.Plan{BoxID: "box-one", PlanDigest: "sha256:plan"},
		Record: generatedstate.Record{
			BoxID: "box-one", Root: ownership.PathRecord{CanonicalPath: root, Identity: ownership.FileIdentity{Device: 7, Inode: 11}},
		},
		Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := step.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if fact.Present {
		t.Fatalf("fact = %#v", fact)
	}
	if err := step.Remove(t.Context(), fact); err != nil {
		t.Fatal(err)
	}
	if runtime.removes != 1 {
		t.Fatalf("remove calls = %d, want one idempotent terminal call", runtime.removes)
	}
}

type stubGeneratedState struct {
	observation generatedstate.Observation
	removes     int
}

func (s *stubGeneratedState) ObserveRemoval(context.Context, contract.Plan, generatedstate.Record) (generatedstate.Observation, error) {
	observed := s.observation
	observed.Artifacts = append([]generatedstate.Artifact(nil), s.observation.Artifacts...)
	return observed, nil
}

func (s *stubGeneratedState) Remove(context.Context, contract.Plan, generatedstate.Record, generatedstate.Observation) error {
	s.removes++
	return nil
}
