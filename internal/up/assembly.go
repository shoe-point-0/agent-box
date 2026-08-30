package up

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/shoe-point-0/agent-box/internal/boxplan"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
)

// IntentStore supplies daemon-persisted request identity. It cannot establish
// external ownership; that authority comes from the current removal snapshot.
type IntentStore interface {
	BoxIntentByID(context.Context, string) (state.BoxIntent, bool, error)
}

// SnapshotStore supplies complete current-generation ownership when a warm
// reconciliation resumes an already READY Box.
type SnapshotStore interface {
	CurrentRemovalSnapshot(context.Context, string) (state.RemovalSnapshot, bool, error)
}

// CompilePlan is the read-only production planning boundary.
type CompilePlan func(context.Context, boxplan.Request) (contract.PlanOutput, error)

// PreparationConfig contains only read-only dependencies. PrepareAttempt does
// not create directories, Git resources, generated files, VMs, or containers.
type PreparationConfig struct {
	Intents   IntentStore
	Snapshots SnapshotStore
	Paths     *paths.Provider
	Compile   CompilePlan
}

// PreparedAttempt is the immutable input for the concrete operation DAG. A
// warm attempt carries exact current-generation ownership; a cold attempt does
// not manufacture ownership from deterministic names or paths.
type PreparedAttempt struct {
	Operation      journal.Operation
	Intent         state.BoxIntent
	Plan           contract.Plan
	CanonicalPlan  []byte
	Generation     int64
	Repository     repository.Facts
	BoxRoot        string
	WorkspaceRoot  string
	PlanPath       string
	ContextPath    string
	LimaConfigPath string
	Current        state.RemovalSnapshot
	Warm           bool
}

type preparationError struct {
	code contract.ErrorCode
	err  error
}

func (e *preparationError) Error() string                 { return e.err.Error() }
func (e *preparationError) Unwrap() error                 { return e.err }
func (e *preparationError) ErrorCode() contract.ErrorCode { return e.code }

// PrepareAttempt reconstructs one running operation from durable identity,
// recompiles desired state from live repository observations, and selects a
// cold or exact warm generation. Plan changes deliberately fail closed until a
// replacement operation can preserve dirty-worktree and runtime teardown rules.
func PrepareAttempt(ctx context.Context, config PreparationConfig, operation journal.Operation) (PreparedAttempt, error) {
	if config.Intents == nil || config.Snapshots == nil || config.Paths == nil || config.Compile == nil {
		return PreparedAttempt{}, errors.New("prepare Box up: intents, snapshots, paths, and Plan compiler are required")
	}
	if operation.ID == "" || operation.BoxID == "" || operation.Kind != "up" || operation.State != journal.StateRunning {
		return PreparedAttempt{}, errors.New("prepare Box up: running up operation is required")
	}
	intent, found, err := config.Intents.BoxIntentByID(ctx, operation.BoxID)
	if err != nil {
		return PreparedAttempt{}, fmt.Errorf("prepare Box up: load intent: %w", err)
	}
	if !found || intent.ID != operation.BoxID {
		return PreparedAttempt{}, &preparationError{code: contract.CodeNotFound, err: errors.New("prepare Box up: persisted Box intent is absent")}
	}
	workspaceID := identity.WorkspaceID(intent.WorkspaceID)
	boxID := identity.BoxID(intent.ID)
	output, err := config.Compile(ctx, boxplan.Request{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: intent.TaskKey, ManifestPath: intent.ManifestPath,
	})
	if err != nil {
		return PreparedAttempt{}, fmt.Errorf("prepare Box up: compile Plan: %w", err)
	}
	if output.SchemaVersion != contract.PlanOutputSchemaVersion || output.Plan.WorkspaceID != intent.WorkspaceID ||
		output.Plan.BoxID != intent.ID || output.Plan.TaskKey != intent.TaskKey || output.Plan.Manifest.Path != intent.ManifestPath ||
		string(output.Plan.Manifest.Digest) != intent.ManifestDigest {
		return PreparedAttempt{}, errors.New("prepare Box up: compiled Plan identity differs from persisted intent")
	}
	if output.Classification == "conflict" {
		return PreparedAttempt{}, &preparationError{code: contract.CodeIdentityConflict, err: errors.New("prepare Box up: Plan contains a conflict")}
	}
	canonical, err := plancompiler.CanonicalJSON(output.Plan)
	if err != nil {
		return PreparedAttempt{}, fmt.Errorf("prepare Box up: validate immutable Plan: %w", err)
	}
	if len(output.Plan.Repositories) != 1 {
		return PreparedAttempt{}, &preparationError{code: contract.CodeFeatureNotSupported, err: errors.New("prepare Box up: executable Plan must contain exactly one repository")}
	}
	facts := factsFromPlan(output.Plan.Repositories[0])
	prepared := PreparedAttempt{
		Operation: operation, Intent: intent, Plan: output.Plan, CanonicalPlan: canonical,
		Generation: 1, Repository: facts,
	}
	prepared.BoxRoot, err = config.Paths.BoxRoot(boxID)
	if err == nil {
		prepared.WorkspaceRoot, err = config.Paths.WorkspaceRoot(boxID)
	}
	if err == nil {
		prepared.PlanPath, err = config.Paths.Plan(boxID)
	}
	if err == nil {
		prepared.ContextPath, err = config.Paths.Context(boxID)
	}
	if err == nil {
		prepared.LimaConfigPath, err = config.Paths.LimaConfig(boxID)
	}
	if err != nil {
		return PreparedAttempt{}, fmt.Errorf("prepare Box up: construct generated paths: %w", err)
	}

	current, currentFound, err := config.Snapshots.CurrentRemovalSnapshot(ctx, operation.BoxID)
	if err != nil {
		return PreparedAttempt{}, fmt.Errorf("prepare Box up: load current ownership: %w", err)
	}
	if !currentFound {
		if intent.CurrentGeneration != 0 {
			return PreparedAttempt{}, &preparationError{code: contract.CodeOwnershipConflict, err: errors.New("prepare Box up: current generation lacks complete ownership authority")}
		}
		return prepared, nil
	}
	if current.BoxID != intent.ID || current.Generation != intent.CurrentGeneration || current.PlanDigest != output.Plan.PlanDigest {
		return PreparedAttempt{}, &preparationError{code: contract.CodeIdentityConflict, err: errors.New("prepare Box up: current generation differs from desired Plan")}
	}
	if len(current.Repositories) != 1 || !reflect.DeepEqual(current.Repositories[0].Facts, facts) {
		return PreparedAttempt{}, &preparationError{code: contract.CodeOwnershipConflict, err: errors.New("prepare Box up: current repository authority differs from desired Plan")}
	}
	prepared.Generation = current.Generation
	prepared.Current = current
	prepared.Warm = true
	return prepared, nil
}

func factsFromPlan(planned contract.PlanRepository) repository.Facts {
	return repository.Facts{
		Name: planned.Name, SourceRoot: planned.SourcePath, Clean: true,
		BaseCommit: planned.BaseCommit, Branch: planned.Branch,
		WorktreePath: planned.WorktreePath, GitStorePath: planned.GitCommonDir,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: planned.SourcePath},
	}
}
