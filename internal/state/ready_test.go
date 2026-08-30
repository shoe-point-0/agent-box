package state

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestPublishReadyCommitsCompleteGenerationAtomicallyAndIsIdempotent(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)

	if err := store.PublishReady(t.Context(), publication); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishReady(t.Context(), publication); err != nil {
		t.Fatalf("idempotent PublishReady() error = %v", err)
	}

	var boxState, generationState, contextPath, contextDigest, runtimeState, composeState string
	var generation int64
	var repositories, worktrees, snapshots int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT recorded_state, current_generation FROM boxes WHERE id = ?`, publication.Plan.BoxID).
			Scan(&boxState, &generation); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT recorded_state, context_path, context_digest FROM box_generations
			WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation).
			Scan(&generationState, &contextPath, &contextDigest); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT recorded_state FROM runtime_hosts WHERE box_id = ?`, publication.Plan.BoxID).Scan(&runtimeState); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT recorded_state FROM compose_projects WHERE box_id = ?`, publication.Plan.BoxID).Scan(&composeState); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT
			(SELECT count(*) FROM repositories WHERE box_id = ?),
			(SELECT count(*) FROM worktrees WHERE box_id = ?),
			(SELECT count(*) FROM removal_ownership_snapshots WHERE box_id = ?)`,
			publication.Plan.BoxID, publication.Plan.BoxID, publication.Plan.BoxID).
			Scan(&repositories, &worktrees, &snapshots)
	}); err != nil {
		t.Fatal(err)
	}
	if boxState != "ready" || generation != 1 || generationState != "ready" || runtimeState != "running" || composeState != "ready" {
		t.Fatalf("published states = %q/%d/%q/%q/%q", boxState, generation, generationState, runtimeState, composeState)
	}
	if contextPath != publication.ContextPath || contextDigest != string(publication.ContextDigest) {
		t.Fatalf("context evidence = %q %q", contextPath, contextDigest)
	}
	if repositories != 1 || worktrees != 1 || snapshots != 1 {
		t.Fatalf("published row counts = repositories %d, worktrees %d, snapshots %d", repositories, worktrees, snapshots)
	}
	got, found, err := store.CurrentRemovalSnapshot(t.Context(), publication.Plan.BoxID)
	if err != nil || !found || !reflect.DeepEqual(got, publication.RemovalSnapshot) {
		t.Fatalf("CurrentRemovalSnapshot() = %#v, %t, %v", got, found, err)
	}
}

func TestPublishReadyRejectsRebindingAnExistingGeneration(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	if err := store.PublishReady(t.Context(), publication); err != nil {
		t.Fatal(err)
	}

	drifted := publication
	drifted.ContextPath = filepath.Join(filepath.Dir(publication.ContextPath), "replacement-context.json")
	if err := store.PublishReady(t.Context(), drifted); err == nil || !strings.Contains(err.Error(), "generation evidence is immutable") {
		t.Fatalf("PublishReady(drifted generation) error = %v", err)
	}

	var boxState string
	var generation int64
	var generationRows, repositories, snapshots int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT recorded_state, current_generation FROM boxes WHERE id = ?`, publication.Plan.BoxID).
			Scan(&boxState, &generation); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT
			(SELECT count(*) FROM box_generations WHERE box_id = ?),
			(SELECT count(*) FROM repositories WHERE box_id = ?),
			(SELECT count(*) FROM removal_ownership_snapshots WHERE box_id = ?)`,
			publication.Plan.BoxID, publication.Plan.BoxID, publication.Plan.BoxID).
			Scan(&generationRows, &repositories, &snapshots)
	}); err != nil {
		t.Fatal(err)
	}
	if boxState != "ready" || generation != 1 || generationRows != 1 || repositories != 1 || snapshots != 1 {
		t.Fatalf("refused replacement changed state: %q/%d rows=%d/%d/%d", boxState, generation, generationRows, repositories, snapshots)
	}
}

func TestPublishReadyRollsBackGenerationWhenAComponentCacheConflicts(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at)
			VALUES ('workspace-other', '/other/.abox/box.yaml', 'sha256:other', 1, 1)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, created_at, updated_at)
			VALUES ('box-other', 'workspace-other', 'OTHER', 'ready', 1, 1)`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO repositories
			(box_id, name, source_path, git_store_path, base_commit, branch)
			VALUES ('box-other', 'repo', '/source', ?, '0123456789abcdef0123456789abcdef01234567', 'other')`,
			publication.Repositories[0].Facts.GitStorePath)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.PublishReady(t.Context(), publication); err == nil {
		t.Fatal("PublishReady() succeeded despite a conflicting Git-store cache identity")
	}
	var boxState string
	var generation int64
	var generationRows, repositories, snapshots int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT recorded_state, current_generation FROM boxes WHERE id = ?`, publication.Plan.BoxID).
			Scan(&boxState, &generation); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT
			(SELECT count(*) FROM box_generations WHERE box_id = ?),
			(SELECT count(*) FROM repositories WHERE box_id = ?),
			(SELECT count(*) FROM removal_ownership_snapshots WHERE box_id = ?)`,
			publication.Plan.BoxID, publication.Plan.BoxID, publication.Plan.BoxID).
			Scan(&generationRows, &repositories, &snapshots)
	}); err != nil {
		t.Fatal(err)
	}
	if boxState != "planning" || generation != 0 || generationRows != 0 || repositories != 0 || snapshots != 0 {
		t.Fatalf("failed publication leaked state: %q/%d rows=%d/%d/%d", boxState, generation, generationRows, repositories, snapshots)
	}
}

func readyGenerationFixture(t *testing.T) ReadyGeneration {
	t.Helper()
	root, err := filepath.Abs("../plan/testdata")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
	loader, err := manifest.NewLoader([]string{root}, manifest.DefaultLimits())
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
	boxID, err := identity.Box(workspaceID, "TASK-READY")
	if err != nil {
		t.Fatal(err)
	}
	applicationRoot := t.TempDir()
	provider, err := paths.New(applicationRoot, filepath.Join(applicationRoot, "cache"), filepath.Join(applicationRoot, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := provider.WorkspacePath(boxID, workspace.Spec.Repositories[0].WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	gitStorePath, err := provider.GitStore(boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repo", SourceRoot: workspace.Spec.Repositories[0].CanonicalLocal, Clean: true,
		BaseCommit: "0123456789abcdef0123456789abcdef01234567", Branch: "abox/TASK-READY/repo",
		WorktreePath: worktreePath, GitStorePath: gitStorePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: workspace.Spec.Repositories[0].CanonicalLocal},
	}
	immutable, _, err := plan.Compile(plan.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-READY",
		ManifestPath: manifestPath, Repositories: []repository.Facts{facts}, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: "2.0.0", Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := testRemovalSnapshot()
	snapshot.BoxID, snapshot.PlanDigest = immutable.BoxID, immutable.PlanDigest
	snapshot.BoxRoot.BoxID = immutable.BoxID
	snapshot.Repositories[0].Facts = facts
	snapshot.Repositories[0].Store.Repository = facts.Name
	snapshot.Repositories[0].Store.GitCommonDir = facts.GitStorePath
	snapshot.Repositories[0].Store.BaseCommit = facts.BaseCommit
	snapshot.Repositories[0].Store.PublishDestination = facts.PublishDestination
	snapshot.LimaSpec.RuntimeHost = immutable.RuntimeHost
	snapshot.LimaSpec.Mounts = immutable.Mounts
	snapshot.LimaSpec.BoxID = immutable.BoxID
	snapshot.LimaSpec.PlanDigest = immutable.PlanDigest
	snapshot.Lima.Name = immutable.RuntimeHost.InstanceName
	snapshot.Lima.Metadata = map[string]string{
		ownership.BoxIDFact: immutable.BoxID, "dev.abox.plan-digest": string(immutable.PlanDigest), "dev.abox.generation": "1",
	}
	snapshot.ComposeSpec.RuntimeHost = snapshot.LimaSpec
	snapshot.ComposeSpec.Plan = immutable
	snapshot.ComposeSpec.Generation = 1
	snapshot.ComposeSpec.Compiled.ProjectName = immutable.Compose.ProjectName
	for index := range snapshot.Compose.Resources {
		snapshot.Compose.Resources[index].Labels = readyLabels(immutable)
	}
	for index := range snapshot.Compose.Volumes {
		snapshot.Compose.Volumes[index].Labels = readyLabels(immutable)
	}

	planPath, err := provider.Plan(boxID)
	if err != nil {
		t.Fatal(err)
	}
	contextPath, err := provider.Context(boxID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	return ReadyGeneration{
		Plan: immutable, Generation: 1, PlanPath: planPath, ContextPath: contextPath,
		ContextDigest:       contract.Digest("sha256:" + strings.Repeat("c", 64)),
		RuntimeConfigDigest: contract.Digest("sha256:" + strings.Repeat("d", 64)),
		Repositories:        []ReadyRepository{{Facts: facts, Store: snapshot.Repositories[0].Store, CurrentCommit: facts.BaseCommit}},
		RemovalSnapshot:     snapshot, ObservedAt: now, PublishedAt: now.Add(time.Second),
	}
}

func readyLabels(immutable contract.Plan) map[string]string {
	return map[string]string{
		ownership.ComposeBoxIDFact:   immutable.BoxID,
		"com.docker.compose.project": immutable.Compose.ProjectName,
		"dev.abox.plan":              string(immutable.PlanDigest), "dev.abox.generation": "1",
	}
}

func seedReadyBox(t *testing.T, store *Store, immutable contract.Plan) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at) VALUES (?, ?, ?, 1, 1)`,
			immutable.WorkspaceID, immutable.Manifest.Path, immutable.Manifest.Digest); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, created_at, updated_at)
			VALUES (?, ?, ?, 'planning', 1, 1)`, immutable.BoxID, immutable.WorkspaceID, immutable.TaskKey)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
