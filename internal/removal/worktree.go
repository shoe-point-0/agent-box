package removal

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/publication"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

// WorktreeObserver supplies fresh registration, branch, status, ignored-file,
// common-directory, and filesystem identity evidence without mutation.
type WorktreeObserver interface {
	ObserveRemoval(context.Context, repository.Facts, contract.Plan, gitstore.Record) (worktree.Observation, error)
}

// PublicationObserver supplies conservative reachability evidence from trusted
// destinations rather than Box-writable Git configuration.
type PublicationObserver interface {
	Check(context.Context, gitstore.Record, []publication.Reference, []repository.Destination) ([]publication.ReferenceStatus, error)
}

// WorktreeRemover consumes the exact fresh observation passed by the
// coordinator. Its implementation remains responsible for final typed
// ownership proof and idempotent Git cleanup.
type WorktreeRemover interface {
	Remove(context.Context, repository.Facts, contract.Plan, gitstore.Record, worktree.Observation) error
}

// WorktreeStepConfig binds one immutable repository record to its concrete
// read-only observers and destructive leaf operation.
type WorktreeStepConfig struct {
	Facts        repository.Facts
	Plan         contract.Plan
	Store        gitstore.Record
	Destinations []repository.Destination
	Worktrees    WorktreeObserver
	Publications PublicationObserver
	Remover      WorktreeRemover
}

// WorktreeStep converts real Git and publication observations into the exact
// evidence and loss inventory consumed by Coordinator.
type WorktreeStep struct {
	facts        repository.Facts
	plan         contract.Plan
	store        gitstore.Record
	destinations []repository.Destination
	worktrees    WorktreeObserver
	publications PublicationObserver
	remover      WorktreeRemover
}

// NewWorktreeStep rejects incomplete integration wiring before any Git call.
func NewWorktreeStep(config WorktreeStepConfig) (*WorktreeStep, error) {
	if config.Facts.Name == "" || config.Worktrees == nil || config.Publications == nil || config.Remover == nil {
		return nil, errors.New("configure worktree removal: facts, observers, and remover are required")
	}
	if len(config.Destinations) == 0 {
		return nil, errors.New("configure worktree removal: publish destinations are required")
	}
	return &WorktreeStep{
		facts: config.Facts, plan: config.Plan, store: config.Store,
		destinations: slices.Clone(config.Destinations), worktrees: config.Worktrees,
		publications: config.Publications, remover: config.Remover,
	}, nil
}

func (s *WorktreeStep) Key() string { return "worktree/" + s.facts.Name }
func (*WorktreeStep) Stage() Stage  { return StageWorktree }
func (s *WorktreeStep) Observe(ctx context.Context) (Fact, error) {
	_, fact, err := s.observe(ctx)
	return fact, err
}

// Remove reobserves the complete Git loss fact before handing the typed
// worktree observation to the destructive leaf. Coordinator also reobserves
// immediately before this call; this second check closes the adapter/leaf gap.
func (s *WorktreeStep) Remove(ctx context.Context, expected Fact) error {
	observation, actual, err := s.observe(ctx)
	if err != nil {
		return err
	}
	if !equalFact(actual, expected) {
		return ErrInventoryDrift
	}
	return s.remover.Remove(ctx, s.facts, s.plan, s.store, observation)
}

func (s *WorktreeStep) observe(ctx context.Context) (worktree.Observation, Fact, error) {
	observed, err := s.worktrees.ObserveRemoval(ctx, s.facts, s.plan, s.store)
	if err != nil {
		return worktree.Observation{}, Fact{}, fmt.Errorf("observe worktree removal %s: %w", s.facts.Name, err)
	}
	if observed.Repository != s.facts.Name || observed.StoreIdentity == (ownership.FileIdentity{}) ||
		observed.CanonicalPath == "" || !filepath.IsAbs(observed.CanonicalPath) || filepath.Clean(observed.CanonicalPath) != observed.CanonicalPath ||
		observed.CanonicalCommon == "" || !filepath.IsAbs(observed.CanonicalCommon) || filepath.Clean(observed.CanonicalCommon) != observed.CanonicalCommon ||
		observed.Dirty != (len(observed.ChangedPaths)+len(observed.IgnoredPaths) != 0) ||
		observed.Registered != (observed.WorktreeIdentity != (ownership.FileIdentity{})) ||
		observed.Registered && !observed.BranchPresent || !observed.BranchPresent && observed.Commit != "" {
		return worktree.Observation{}, Fact{}, errors.New("observe worktree removal: worktree evidence is incomplete")
	}
	if !observed.BranchPresent {
		return observed, Fact{Present: false}, nil
	}
	statuses, err := s.publications.Check(ctx, s.store, []publication.Reference{{
		Name: "refs/heads/" + observed.Branch, Commit: observed.Commit,
	}}, s.destinations)
	if err != nil {
		return worktree.Observation{}, Fact{}, fmt.Errorf("observe worktree publication %s: %w", s.facts.Name, err)
	}
	if len(statuses) != 1 || statuses[0].Reference.Name != "refs/heads/"+observed.Branch || statuses[0].Reference.Commit != observed.Commit {
		return worktree.Observation{}, Fact{}, errors.New("observe worktree publication: evidence does not match worktree")
	}

	losses := make([]Loss, 0, len(observed.ChangedPaths)+len(observed.IgnoredPaths)+len(statuses[0].UnpublishedCommits))
	for _, relative := range append(slices.Clone(observed.ChangedPaths), observed.IgnoredPaths...) {
		path, pathErr := exactLossPath(observed.CanonicalPath, relative)
		if pathErr != nil {
			return worktree.Observation{}, Fact{}, pathErr
		}
		losses = append(losses, Loss{Kind: LossWorktreeChange, Value: path})
	}
	for _, commit := range statuses[0].UnpublishedCommits {
		losses = append(losses, Loss{Kind: LossUnpublishedCommit, Value: commit})
	}
	slices.SortFunc(losses, compareLoss)
	losses = slices.CompactFunc(losses, func(left, right Loss) bool { return compareLoss(left, right) == 0 })

	evidence := map[string]string{
		"branch": observed.Branch, "commit": observed.Commit,
		"commonDir": observed.CanonicalCommon, "path": observed.CanonicalPath,
		"lockReason":    observed.LockReason,
		"storeIdentity": formatIdentity(observed.StoreIdentity),
	}
	if observed.Registered {
		evidence["worktreeIdentity"] = formatIdentity(observed.WorktreeIdentity)
	}
	fact := Fact{Present: true, Evidence: evidence, Losses: losses}
	return observed, fact, nil
}

func exactLossPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.IndexByte(relative, 0) >= 0 {
		return "", errors.New("observe worktree removal: Git returned an invalid loss path")
	}
	path := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("observe worktree removal: Git loss path escapes worktree")
	}
	return path, nil
}

func formatIdentity(identity ownership.FileIdentity) string {
	return strconv.FormatUint(identity.Device, 10) + ":" + strconv.FormatUint(identity.Inode, 10)
}

func equalFact(left, right Fact) bool {
	return left.Present == right.Present && maps.Equal(left.Evidence, right.Evidence) && slices.Equal(left.Losses, right.Losses)
}
