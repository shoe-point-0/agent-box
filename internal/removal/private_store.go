package removal

import (
	"context"
	"errors"
	"fmt"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

// PrivateStoreObserver supplies fresh canonical path and inode evidence without
// mutating the Machine filesystem.
type PrivateStoreObserver interface {
	ObserveRemoval(context.Context, repository.Facts, contract.Plan, gitstore.Record) (gitstore.RemovalObservation, error)
}

// PrivateStoreRemover consumes the exact fresh observation passed by the
// coordinator and performs its own final typed ownership proof.
type PrivateStoreRemover interface {
	Remove(context.Context, repository.Facts, contract.Plan, gitstore.Record, gitstore.RemovalObservation) error
}

// PrivateStoreStepConfig binds one immutable private-store record to its
// concrete observer and destructive leaf operation.
type PrivateStoreStepConfig struct {
	Facts    repository.Facts
	Plan     contract.Plan
	Store    gitstore.Record
	Observer PrivateStoreObserver
	Remover  PrivateStoreRemover
}

// PrivateStoreStep contributes ownership-bound private-store evidence to Box
// removal after its worktree dependency has converged to absence.
type PrivateStoreStep struct {
	facts    repository.Facts
	plan     contract.Plan
	store    gitstore.Record
	observer PrivateStoreObserver
	remover  PrivateStoreRemover
}

// NewPrivateStoreStep rejects incomplete integration wiring before observation.
func NewPrivateStoreStep(config PrivateStoreStepConfig) (*PrivateStoreStep, error) {
	if config.Facts.Name == "" || config.Observer == nil || config.Remover == nil {
		return nil, errors.New("configure private-store removal: facts, observer, and remover are required")
	}
	return &PrivateStoreStep{
		facts: config.Facts, plan: config.Plan, store: config.Store,
		observer: config.Observer, remover: config.Remover,
	}, nil
}

func (s *PrivateStoreStep) Key() string { return "private-store/" + s.facts.Name }
func (*PrivateStoreStep) Stage() Stage  { return StagePrivateStore }
func (s *PrivateStoreStep) Observe(ctx context.Context) (Fact, error) {
	_, fact, err := s.observe(ctx)
	return fact, err
}

// Remove independently reobserves the complete fact to close the
// coordinator-to-leaf gap before invoking the proof-consuming remover.
func (s *PrivateStoreStep) Remove(ctx context.Context, expected Fact) error {
	observation, actual, err := s.observe(ctx)
	if err != nil {
		return err
	}
	if !equalFact(actual, expected) {
		return ErrInventoryDrift
	}
	return s.remover.Remove(ctx, s.facts, s.plan, s.store, observation)
}

func (s *PrivateStoreStep) observe(ctx context.Context) (gitstore.RemovalObservation, Fact, error) {
	observed, err := s.observer.ObserveRemoval(ctx, s.facts, s.plan, s.store)
	if err != nil {
		return gitstore.RemovalObservation{}, Fact{}, fmt.Errorf("observe private-store removal %s: %w", s.facts.Name, err)
	}
	if observed.Repository != s.facts.Name || observed.CanonicalPath != s.store.GitCommonDir ||
		observed.Present != (observed.Identity != (ownership.FileIdentity{})) {
		return gitstore.RemovalObservation{}, Fact{}, errors.New("observe private-store removal: evidence is incomplete")
	}
	if !observed.Present {
		return observed, Fact{Present: false}, nil
	}
	return observed, Fact{Present: true, Evidence: map[string]string{
		"baseCommit":    s.store.BaseCommit,
		"path":          observed.CanonicalPath,
		"storeIdentity": formatIdentity(observed.Identity),
	}}, nil
}
