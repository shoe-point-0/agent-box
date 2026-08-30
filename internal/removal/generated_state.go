package removal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
