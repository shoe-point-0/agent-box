package up

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/boxplan"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/testutil"
)

func TestPrepareAttemptSelectsColdGenerationFromPersistedIntent(t *testing.T) {
	fixture := newPreparationFixture(t)
	prepared, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Warm || prepared.Generation != 1 {
		t.Fatalf("cold generation = %d warm=%t, want 1/false", prepared.Generation, prepared.Warm)
	}
	if prepared.Intent != fixture.store.intent || !reflect.DeepEqual(prepared.Repository, factsFromPlan(fixture.plan.Repositories[0])) {
		t.Fatal("prepared intent or repository facts differ from durable/Plan identity")
	}
	wantPlan, err := plancompiler.CanonicalJSON(fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared.CanonicalPlan, wantPlan) {
		t.Fatal("prepared canonical Plan differs")
	}
	if prepared.PlanPath != filepath.Join(prepared.BoxRoot, "plan.json") ||
		prepared.ContextPath != filepath.Join(prepared.BoxRoot, "context.json") ||
		prepared.LimaConfigPath != filepath.Join(prepared.BoxRoot, "lima.yaml") {
		t.Fatalf("generated paths are not centralized under Box root: %+v", prepared)
	}
}

func TestPrepareAttemptSelectsOnlyExactWarmAuthority(t *testing.T) {
	fixture := newPreparationFixture(t)
	fixture.store.intent.CurrentGeneration = 4
	fixture.store.snapshot = state.RemovalSnapshot{
		BoxID: fixture.plan.BoxID, Generation: 4, PlanDigest: fixture.plan.PlanDigest,
		Repositories: []state.RepositoryRemovalRecord{{Facts: factsFromPlan(fixture.plan.Repositories[0])}},
	}
	fixture.store.snapshotFound = true
	prepared, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Warm || prepared.Generation != 4 || prepared.Current.Generation != 4 {
		t.Fatalf("warm preparation = generation %d warm=%t current=%d", prepared.Generation, prepared.Warm, prepared.Current.Generation)
	}

	fixture.store.snapshot.Repositories[0].Facts.Branch = "abox/other"
	_, err = PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
	var coded interface{ ErrorCode() contract.ErrorCode }
	if !errors.As(err, &coded) || coded.ErrorCode() != contract.CodeOwnershipConflict {
		t.Fatalf("drifted warm authority error = %v, want ownership conflict", err)
	}
}

func TestPrepareAttemptRefusesCurrentGenerationWithoutOwnership(t *testing.T) {
	fixture := newPreparationFixture(t)
	fixture.store.intent.CurrentGeneration = 1
	_, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
	var coded interface{ ErrorCode() contract.ErrorCode }
	if !errors.As(err, &coded) || coded.ErrorCode() != contract.CodeOwnershipConflict {
		t.Fatalf("missing authority error = %v, want ownership conflict", err)
	}
}

type preparationFixture struct {
	provider  *paths.Provider
	store     *preparationStore
	plan      contract.Plan
	operation journal.Operation
}

func newPreparationFixture(t *testing.T) *preparationFixture {
	t.Helper()
	fixtureRoot, err := filepath.Abs(filepath.Join("..", "plan", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(fixtureRoot, "workspace", ".abox", "box.yaml")
	loader, err := manifest.NewLoader([]string{fixtureRoot}, manifest.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-67")
	if err != nil {
		t.Fatal(err)
	}
	root := testutil.TempDir(t)
	provider, err := paths.New(filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := provider.WorkspacePath(boxID, workspace.Spec.Repositories[0].WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	storePath, err := provider.GitStore(boxID, workspace.Spec.Repositories[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: workspace.Spec.Repositories[0].Name, SourceRoot: workspace.Spec.Repositories[0].CanonicalLocal, Clean: true,
		BaseCommit: "0123456789abcdef0123456789abcdef01234567", Branch: "abox/TASK-67/repo",
		WorktreePath: worktreePath, GitStorePath: storePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: workspace.Spec.Repositories[0].CanonicalLocal},
	}
	compiled, _, err := plancompiler.Compile(plancompiler.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-67", ManifestPath: manifestPath,
		Repositories: []repository.Facts{facts}, Runtime: plancompiler.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "devel", Git: "2.51.0", Lima: "2.0.0", Docker: "28.3.3", Compose: "2.39.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &preparationStore{intent: state.BoxIntent{
		ID: boxID.String(), WorkspaceID: workspaceID.String(), TaskKey: "TASK-67",
		ManifestPath: manifestPath, ManifestDigest: string(compiled.Manifest.Digest), RecordedState: "planning",
	}}
	return &preparationFixture{
		provider: provider, store: store, plan: compiled,
		operation: journal.Operation{ID: "operation-67", BoxID: boxID.String(), Kind: "up", State: journal.StateRunning},
	}
}

func (f *preparationFixture) config() PreparationConfig {
	return PreparationConfig{
		Intents: f.store, Snapshots: f.store, Paths: f.provider,
		Compile: func(_ context.Context, request boxplan.Request) (contract.PlanOutput, error) {
			if request.WorkspaceID.String() != f.plan.WorkspaceID || request.BoxID.String() != f.plan.BoxID ||
				request.TaskKey != f.plan.TaskKey || request.ManifestPath != f.plan.Manifest.Path {
				return contract.PlanOutput{}, errors.New("unexpected Plan request")
			}
			return contract.PlanOutput{SchemaVersion: contract.PlanOutputSchemaVersion, Plan: f.plan, Classification: "in-place"}, nil
		},
	}
}

type preparationStore struct {
	intent        state.BoxIntent
	snapshot      state.RemovalSnapshot
	snapshotFound bool
}

func (s *preparationStore) BoxIntentByID(_ context.Context, boxID string) (state.BoxIntent, bool, error) {
	if boxID != s.intent.ID {
		return state.BoxIntent{}, false, nil
	}
	return s.intent, true, nil
}

func (s *preparationStore) CurrentRemovalSnapshot(_ context.Context, boxID string) (state.RemovalSnapshot, bool, error) {
	if boxID != s.intent.ID || !s.snapshotFound {
		return state.RemovalSnapshot{}, false, nil
	}
	return s.snapshot, true, nil
}
