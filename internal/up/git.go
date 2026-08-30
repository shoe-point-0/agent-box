package up

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

// GitAuthorityStore retains the exact external identities established during
// an up attempt. Loaded records are recovery authority, never observations.
type GitAuthorityStore interface {
	ReconciliationGitAuthorityFor(context.Context, string, int64, string) (state.ReconciliationGitAuthority, bool, error)
	SaveReconciliationGitAuthority(context.Context, state.ReconciliationGitAuthority, time.Time) error
}

var _ GitAuthorityStore = (*state.Store)(nil)

// PrivateStoreMaterializer creates one Box-private store after the durable
// intent claim has been written. It must refuse an existing destination.
type PrivateStoreMaterializer interface {
	Materialize(context.Context, repository.Facts, contract.Plan) (gitstore.Record, error)
}

var _ PrivateStoreMaterializer = gitstore.Materializer{}

// WorktreeReconciler is the concrete linked-worktree boundary. Observe is used
// when recovery authority says creation already completed; Ensure is used only
// when no worktree identity has yet been retained.
type WorktreeReconciler interface {
	Observe(context.Context, repository.Facts, contract.Plan, gitstore.Record) (worktree.Observation, error)
	Ensure(context.Context, repository.Facts, contract.Plan, gitstore.Record) (worktree.Observation, error)
}

var _ WorktreeReconciler = worktree.Manager{}

// GitConfig binds a prepared immutable attempt to concrete Git adapters.
type GitConfig struct {
	Authority GitAuthorityStore
	Stores    PrivateStoreMaterializer
	Worktrees WorktreeReconciler
	Now       func() time.Time
}

// GitResult contains freshly observed identities for later readiness and READY
// publication.
type GitResult struct {
	Store    gitstore.Record
	Worktree worktree.Observation
}

// ReconcileGit materializes the private store and locked worktree in ownership
// order. It writes intent before mutation and each external identity immediately
// after successful apply. Recovery may resume only through an exact retained
// inode/registration proof; deterministic paths alone are never adopted.
func ReconcileGit(ctx context.Context, config GitConfig, attempt PreparedAttempt) (GitResult, error) {
	if config.Authority == nil || config.Stores == nil || config.Worktrees == nil {
		return GitResult{}, errors.New("reconcile Git: authority store, private-store materializer, and worktree reconciler are required")
	}
	if attempt.Operation.ID == "" || attempt.Operation.BoxID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID ||
		attempt.Generation < 1 || attempt.Repository.Name == "" {
		return GitResult{}, errors.New("reconcile Git: prepared attempt identity is incomplete")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	authority, found, err := config.Authority.ReconciliationGitAuthorityFor(
		ctx, attempt.Plan.BoxID, attempt.Generation, attempt.Repository.Name,
	)
	if err != nil {
		return GitResult{}, fmt.Errorf("reconcile Git: load recovery authority: %w", err)
	}
	desired := state.ReconciliationGitAuthority{
		BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, Repository: attempt.Repository,
		PlanDigest: attempt.Plan.PlanDigest, OperationID: attempt.Operation.ID,
	}
	if !found {
		authority = desired
		if attempt.Warm {
			if len(attempt.Current.Repositories) != 1 || !reflect.DeepEqual(attempt.Current.Repositories[0].Facts, attempt.Repository) {
				return GitResult{}, errors.New("reconcile Git: warm repository authority is incomplete")
			}
			authority.Store = cloneStore(attempt.Current.Repositories[0].Store)
		}
	} else {
		observedBase := authority
		observedBase.OperationID, observedBase.Store, observedBase.Worktree = "", nil, nil
		desired.OperationID = ""
		if !reflect.DeepEqual(observedBase, desired) {
			return GitResult{}, errors.New("reconcile Git: recovery authority differs from prepared attempt")
		}
		authority.OperationID = attempt.Operation.ID
	}
	// This transition validates immutable identity and durably transfers an old
	// operation's partial authority to the currently running operation before any
	// external observation can lead to mutation.
	if err := config.Authority.SaveReconciliationGitAuthority(ctx, authority, now().UTC()); err != nil {
		return GitResult{}, fmt.Errorf("reconcile Git: persist intent: %w", err)
	}

	result := GitResult{}
	if authority.Store == nil {
		store, err := config.Stores.Materialize(ctx, attempt.Repository, attempt.Plan)
		if err != nil {
			return GitResult{}, fmt.Errorf("reconcile Git: materialize private store: %w", err)
		}
		authority.Store = cloneStore(store)
		if err := config.Authority.SaveReconciliationGitAuthority(ctx, authority, now().UTC()); err != nil {
			return GitResult{}, fmt.Errorf("reconcile Git: retain private-store identity: %w", err)
		}
	}
	result.Store = *cloneStore(*authority.Store)

	var observed worktree.Observation
	if authority.Worktree == nil {
		observed, err = config.Worktrees.Ensure(ctx, attempt.Repository, attempt.Plan, result.Store)
	} else {
		observed, err = config.Worktrees.Observe(ctx, attempt.Repository, attempt.Plan, result.Store)
	}
	if err != nil {
		return GitResult{}, fmt.Errorf("reconcile Git: reconcile linked worktree: %w", err)
	}
	worktreeAuthority := gitWorktreeAuthority(observed)
	if authority.Worktree != nil && !reflect.DeepEqual(*authority.Worktree, worktreeAuthority) {
		return GitResult{}, errors.New("reconcile Git: linked-worktree identity differs from recovery authority")
	}
	authority.Worktree = &worktreeAuthority
	if err := config.Authority.SaveReconciliationGitAuthority(ctx, authority, now().UTC()); err != nil {
		return GitResult{}, fmt.Errorf("reconcile Git: retain linked-worktree identity: %w", err)
	}
	result.Worktree = observed
	return result, nil
}

func gitWorktreeAuthority(observed worktree.Observation) state.GitWorktreeAuthority {
	return state.GitWorktreeAuthority{
		CanonicalPath: observed.CanonicalPath, GitCommonDir: observed.CanonicalCommon,
		Branch: observed.Branch, LockReason: observed.LockReason, PathIdentity: observed.WorktreeIdentity,
	}
}

func cloneStore(record gitstore.Record) *gitstore.Record {
	cloned := record
	return &cloned
}
