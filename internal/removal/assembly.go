package removal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/publication"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

var ErrRemovalSnapshotAbsent = errors.New("removal ownership snapshot is absent")

// SnapshotStore supplies the immutable ownership authority for the selected
// Box's current generation. Implementations must validate the persisted row
// identity against the decoded snapshot before returning it.
type SnapshotStore interface {
	CurrentRemovalSnapshot(context.Context, string) (state.RemovalSnapshot, bool, error)
}

// MachineGit is the complete read-only and removal-safe Git boundary needed to
// reconstruct worktree and publication checks after daemon restart.
type MachineGit interface {
	worktree.Git
	publication.Git
}

// AssemblyConfig contains daemon-incarnation dependencies that are deliberately
// excluded from the persisted ownership snapshot.
type AssemblyConfig struct {
	Snapshots   SnapshotStore
	Journal     LeaseJournal
	Operation   journal.Operation
	Owner       journal.Owner
	Quiescer    Quiescer
	Paths       *paths.Provider
	MachineGit  MachineGit
	Lima        LimaRuntime
	Compose     ComposeRuntime
	AppendEvent AppendEventFunc
	Now         func() time.Time
}

// AssembleCurrent reconstructs a complete removal coordinator only from the
// selected current-generation snapshot and explicit daemon-owned adapters. It
// performs no external observation or mutation; Preview remains the read-only
// preflight boundary.
func AssembleCurrent(ctx context.Context, config AssemblyConfig) (*Coordinator, error) {
	if config.Snapshots == nil || config.Journal == nil || config.Quiescer == nil ||
		config.Paths == nil || config.MachineGit == nil || config.Lima == nil ||
		config.Compose == nil || config.AppendEvent == nil {
		return nil, errors.New("assemble Box removal: snapshots, journal, quiescer, paths, and runtime adapters are required")
	}
	if config.Operation.ID == "" || config.Operation.BoxID == "" || config.Operation.Kind != "remove" ||
		config.Operation.State != journal.StateRunning {
		return nil, errors.New("assemble Box removal: running remove operation is required")
	}

	snapshot, found, err := config.Snapshots.CurrentRemovalSnapshot(ctx, config.Operation.BoxID)
	if err != nil {
		return nil, fmt.Errorf("assemble Box removal: load ownership snapshot: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("assemble Box removal: %w", ErrRemovalSnapshotAbsent)
	}
	if err := validateAssemblyIdentity(snapshot, config.Operation); err != nil {
		return nil, err
	}

	steps, err := assembleSteps(snapshot, config)
	if err != nil {
		return nil, err
	}
	lease, err := NewJournalLeaseGuard(JournalLeaseGuardConfig{
		Journal: config.Journal, Operation: config.Operation, Owner: config.Owner, Now: config.Now,
	})
	if err != nil {
		return nil, err
	}
	recorder, err := NewEventRecorder(config.Operation.ID, config.AppendEvent, config.Now)
	if err != nil {
		return nil, err
	}
	coordinator, err := New(Config{
		Steps: steps, Lease: lease, Quiescer: config.Quiescer, Recorder: recorder,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble Box removal: %w", err)
	}
	return coordinator, nil
}

func validateAssemblyIdentity(snapshot state.RemovalSnapshot, operation journal.Operation) error {
	immutable := snapshot.ComposeSpec.Plan
	if snapshot.BoxID != operation.BoxID || immutable.BoxID != snapshot.BoxID ||
		snapshot.LimaSpec.BoxID != snapshot.BoxID || snapshot.LimaSpec.RuntimeHost.InstanceName != snapshot.BoxID ||
		snapshot.ComposeSpec.RuntimeHost.BoxID != snapshot.BoxID ||
		snapshot.PlanDigest != immutable.PlanDigest || snapshot.LimaSpec.PlanDigest != snapshot.PlanDigest ||
		snapshot.Generation != snapshot.LimaSpec.Generation || snapshot.Generation != snapshot.ComposeSpec.Generation {
		return errors.New("assemble Box removal: ownership snapshot identity differs from operation")
	}
	if len(snapshot.Repositories) != len(immutable.Repositories) || len(snapshot.Repositories) != 1 {
		return errors.New("assemble Box removal: repository ownership differs from the executable Plan")
	}
	return nil
}

func assembleSteps(snapshot state.RemovalSnapshot, config AssemblyConfig) ([]Step, error) {
	immutable := snapshot.ComposeSpec.Plan
	boxID := identity.BoxID(snapshot.BoxID)
	boxRoot, err := config.Paths.BoxRoot(boxID)
	if err != nil {
		return nil, fmt.Errorf("assemble Box removal: Box root: %w", err)
	}
	if snapshot.BoxRoot.BoxID != snapshot.BoxID || snapshot.BoxRoot.Root.CanonicalPath != boxRoot {
		return nil, errors.New("assemble Box removal: persisted Box-root identity differs from configured paths")
	}
	workspaceRoot, err := config.Paths.WorkspaceRoot(boxID)
	if err != nil {
		return nil, fmt.Errorf("assemble Box removal: workspace root: %w", err)
	}

	composeStep, err := NewComposeStep(ComposeStepConfig{
		Spec: snapshot.ComposeSpec, Records: snapshot.Compose, Runtime: config.Compose,
	})
	if err != nil {
		return nil, err
	}
	limaStep, err := NewLimaStep(LimaStepConfig{
		Spec: snapshot.LimaSpec, Record: snapshot.Lima, Runtime: config.Lima,
	})
	if err != nil {
		return nil, err
	}
	steps := []Step{composeStep, limaStep}

	worktrees := worktree.Manager{Git: config.MachineGit, WorkspaceRoot: workspaceRoot}
	publications := publication.Checker{Git: config.MachineGit}
	for index, record := range snapshot.Repositories {
		if immutable.Repositories[index].Name != record.Facts.Name {
			return nil, errors.New("assemble Box removal: repository order differs from immutable Plan")
		}
		step, stepErr := NewWorktreeStep(WorktreeStepConfig{
			Facts: record.Facts, Plan: immutable, Store: record.Store,
			Destinations: []repository.Destination{record.Facts.PublishDestination},
			Worktrees:    worktrees, Publications: publications, Remover: worktrees,
		})
		if stepErr != nil {
			return nil, stepErr
		}
		steps = append(steps, step)
	}

	stores := gitstore.RemovalManager{BoxRoot: boxRoot, Paths: gitstore.OSPathRemover{}}
	for _, record := range snapshot.Repositories {
		step, stepErr := NewPrivateStoreStep(PrivateStoreStepConfig{
			Facts: record.Facts, Plan: immutable, Store: record.Store,
			Observer: stores, Remover: stores,
		})
		if stepErr != nil {
			return nil, stepErr
		}
		steps = append(steps, step)
	}

	generated, err := generatedstate.NewManager(config.Paths, generatedstate.SystemOperations())
	if err != nil {
		return nil, err
	}
	generatedStep, err := NewGeneratedStateStep(GeneratedStateStepConfig{
		Plan: immutable, Record: snapshot.BoxRoot, Runtime: generated,
	})
	if err != nil {
		return nil, err
	}
	return append(steps, generatedStep), nil
}
