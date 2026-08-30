package removal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
)

// GeneratedStateRuntime is the ownership-verifying filesystem boundary used by
// the final transient-state removal stage.
type GeneratedStateRuntime interface {
	ObserveRemoval(context.Context, contract.Plan, generatedstate.Record) (generatedstate.Observation, error)
	Remove(context.Context, contract.Plan, generatedstate.Record, generatedstate.Observation) error
}

// GeneratedStateStepConfig binds one immutable Plan and persisted Box-root
// identity to the generated-state filesystem adapter.
type GeneratedStateStepConfig struct {
	Plan    contract.Plan
	Record  generatedstate.Record
	Runtime GeneratedStateRuntime
}

// GeneratedStateStep removes transient generated files only after all external
// resources have converged to absence. The immutable Plan and diagnostics stay
// available for partial-cleanup diagnosis and daemon finalization.
type GeneratedStateStep struct {
	plan    contract.Plan
	record  generatedstate.Record
	runtime GeneratedStateRuntime
}

// NewGeneratedStateStep rejects incomplete integration wiring before preview.
func NewGeneratedStateStep(config GeneratedStateStepConfig) (*GeneratedStateStep, error) {
	if config.Runtime == nil || config.Plan.BoxID == "" || config.Record.BoxID != config.Plan.BoxID {
		return nil, errors.New("configure generated-state removal: Plan, ownership record, and runtime are required")
	}
	return &GeneratedStateStep{plan: config.Plan, record: config.Record, runtime: config.Runtime}, nil
}

func (s *GeneratedStateStep) Key() string { return "generated-state/" + s.record.BoxID }
func (*GeneratedStateStep) Stage() Stage  { return StageGeneratedState }

func (s *GeneratedStateStep) Observe(ctx context.Context) (Fact, error) {
	_, fact, err := s.observe(ctx)
	return fact, err
}

// Remove closes the coordinator-to-leaf gap before the runtime repeats its own
// typed ownership proofs for every exact path.
func (s *GeneratedStateStep) Remove(ctx context.Context, expected Fact) error {
	observation, actual, err := s.observe(ctx)
	if err != nil {
		return err
	}
	if !equalFact(actual, expected) {
		return ErrInventoryDrift
	}
	return s.runtime.Remove(ctx, s.plan, s.record, observation)
}

func (s *GeneratedStateStep) observe(ctx context.Context) (generatedstate.Observation, Fact, error) {
	observed, err := s.runtime.ObserveRemoval(ctx, s.plan, s.record)
	if err != nil {
		return generatedstate.Observation{}, Fact{}, fmt.Errorf("observe generated-state removal: %w", err)
	}
	if observed.BoxID != s.record.BoxID || observed.Root != s.record.Root.CanonicalPath || observed.Present != (len(observed.Artifacts) != 0) {
		return generatedstate.Observation{}, Fact{}, errors.New("observe generated-state removal: evidence is incomplete")
	}
	if !observed.Present {
		return observed, Fact{Present: false}, nil
	}
	artifacts, err := json.Marshal(observed.Artifacts)
	if err != nil {
		return generatedstate.Observation{}, Fact{}, fmt.Errorf("encode generated-state artifacts: %w", err)
	}
	return observed, Fact{Present: true, Evidence: map[string]string{
		"boxId":         observed.BoxID,
		"root":          observed.Root,
		"rootIdentity":  formatIdentity(observed.RootIdentity),
		"planDigest":    string(s.plan.PlanDigest),
		"artifacts":     string(artifacts),
		"retainedState": "plan.json,diagnostics",
	}}, nil
}

// validateDependencyTransition permits only a monotonic shrink of the exact
// generated artifact inventory. Earlier worktree and private-store stages may
// remove their now-empty parent directories after the full preview, but may not
// add or replace an artifact or change the Box-root identity.
func (*GeneratedStateStep) validateDependencyTransition(expected, actual Fact) error {
	if !expected.Present || len(expected.Losses) != 0 || len(actual.Losses) != 0 {
		return ErrInventoryDrift
	}
	if !actual.Present {
		return nil
	}
	expectedEvidence := maps.Clone(expected.Evidence)
	actualEvidence := maps.Clone(actual.Evidence)
	expectedArtifactsJSON, expectedOK := expectedEvidence["artifacts"]
	actualArtifactsJSON, actualOK := actualEvidence["artifacts"]
	delete(expectedEvidence, "artifacts")
	delete(actualEvidence, "artifacts")
	if !expectedOK || !actualOK || !maps.Equal(expectedEvidence, actualEvidence) {
		return ErrInventoryDrift
	}
	var expectedArtifacts, actualArtifacts []generatedstate.Artifact
	if json.Unmarshal([]byte(expectedArtifactsJSON), &expectedArtifacts) != nil ||
		json.Unmarshal([]byte(actualArtifactsJSON), &actualArtifacts) != nil {
		return ErrInventoryDrift
	}
	allowed := make(map[generatedstate.Artifact]struct{}, len(expectedArtifacts))
	for _, artifact := range expectedArtifacts {
		allowed[artifact] = struct{}{}
	}
	for _, artifact := range actualArtifacts {
		if _, ok := allowed[artifact]; !ok {
			return ErrInventoryDrift
		}
	}
	return nil
}
