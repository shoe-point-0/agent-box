package state

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestRemovalSnapshotRoundTripsAndIsImmutablePerGeneration(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	seedCurrentGeneration(t, store, "box-a", 1, "sha256:plan")
	snapshot := testRemovalSnapshot()

	if err := store.SaveRemovalSnapshot(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemovalSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("idempotent save: %v", err)
	}
	got, found, err := store.CurrentRemovalSnapshot(t.Context(), snapshot.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("CurrentRemovalSnapshot() = %#v, %t, want %#v, true", got, found, snapshot)
	}

	drifted := snapshot
	drifted.Lima.Metadata = map[string]string{ownership.BoxIDFact: snapshot.BoxID, "generation": "replacement"}
	if err := store.SaveRemovalSnapshot(t.Context(), drifted); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("drifted SaveRemovalSnapshot() error = %v, want immutable refusal", err)
	}
	got, found, err = store.CurrentRemovalSnapshot(t.Context(), snapshot.BoxID)
	if err != nil || !found || !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("snapshot after refused replacement = %#v, %t, %v", got, found, err)
	}
}

func TestRemovalSnapshotRequiresExactCurrentPlanAndFailsClosedWhenMissing(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	seedCurrentGeneration(t, store, "box-a", 1, "sha256:other")

	if err := store.SaveRemovalSnapshot(t.Context(), testRemovalSnapshot()); err == nil || !strings.Contains(err.Error(), "current Plan differs") {
		t.Fatalf("SaveRemovalSnapshot() error = %v, want Plan refusal", err)
	}
	got, found, err := store.CurrentRemovalSnapshot(t.Context(), "box-a")
	if err != nil || found || !reflect.DeepEqual(got, RemovalSnapshot{}) {
		t.Fatalf("CurrentRemovalSnapshot() = %#v, %t, %v, want zero, false, nil", got, found, err)
	}
}

func TestCurrentRemovalSnapshotRejectsPayloadIdentityDrift(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	seedCurrentGeneration(t, store, "box-a", 1, "sha256:plan")
	if err := store.SaveRemovalSnapshot(t.Context(), testRemovalSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE removal_ownership_snapshots
			SET plan_digest = 'sha256:forged' WHERE box_id = 'box-a' AND generation = 1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if _, found, err := store.CurrentRemovalSnapshot(t.Context(), "box-a"); err == nil || found || !strings.Contains(err.Error(), "row identity differs") {
		t.Fatalf("CurrentRemovalSnapshot() = found %t, error %v, want identity refusal", found, err)
	}
}

func seedCurrentGeneration(t *testing.T, store *Store, boxID string, generation int64, digest string) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at)
			VALUES ('workspace-a', '/workspace/a/.abox/box.yaml', 'sha256:manifest', 1, 1)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, current_generation, created_at, updated_at)
			VALUES (?, 'workspace-a', 'TASK-A', 'ready', ?, 1, 1)`, boxID, generation); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO box_generations
			(box_id, generation, plan_digest, plan_path, recorded_state, created_at)
			VALUES (?, ?, ?, '/state/boxes/box-a/plan.json', 'ready', 1)`, boxID, generation, digest)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func testRemovalSnapshot() RemovalSnapshot {
	boxID := "box-a"
	identity := ownership.FileIdentity{Device: 7, Inode: 11}
	pathSet := ownership.PathSetRecord{Paths: []ownership.PathChainRecord{{
		CanonicalPath: "/state/boxes/box-a/plan.json",
		Ancestors: []ownership.PathRecord{
			{CanonicalPath: "/", Identity: ownership.FileIdentity{Device: 7, Inode: 1}},
			{CanonicalPath: "/state", Identity: ownership.FileIdentity{Device: 7, Inode: 2}},
			{CanonicalPath: "/state/boxes", Identity: ownership.FileIdentity{Device: 7, Inode: 3}},
			{CanonicalPath: "/state/boxes/box-a", Identity: identity},
			{CanonicalPath: "/state/boxes/box-a/plan.json", Identity: ownership.FileIdentity{Device: 7, Inode: 12}},
		},
	}}}
	destination := repository.Destination{Kind: repository.DestinationLocal, URL: "/source/repo"}
	facts := repository.Facts{
		Name: "repo", SourceRoot: "/source/repo", Clean: true,
		BaseCommit: strings.Repeat("1", 40), Branch: "abox/task/repo",
		WorktreePath: "/state/boxes/box-a/workspace/repo", GitStorePath: "/state/boxes/box-a/git/repo.git",
		PublishDestination: destination,
	}
	labels := map[string]string{ownership.ComposeBoxIDFact: boxID, "dev.abox.plan": "sha256:plan"}
	return RemovalSnapshot{
		BoxID: boxID, Generation: 1, PlanDigest: contract.Digest("sha256:plan"),
		BoxRoot: generatedstate.Record{BoxID: boxID, Root: ownership.PathRecord{CanonicalPath: "/state/boxes/box-a", Identity: identity}},
		Repositories: []RepositoryRemovalRecord{{
			Facts: facts,
			Store: gitstore.Record{Repository: facts.Name, GitCommonDir: facts.GitStorePath, Identity: ownership.FileIdentity{Device: 7, Inode: 20}, BaseCommit: facts.BaseCommit, PublishDestination: destination},
		}},
		Lima: ownership.LimaInstanceRecord{Name: boxID, Metadata: map[string]string{ownership.BoxIDFact: boxID, "dev.abox.plan-digest": "sha256:plan"}},
		Compose: compose.RuntimeRecords{
			Resources: []ownership.ComposeResourceRecord{{Kind: "container", ID: "container-id", Labels: labels}},
			Volumes:   []ownership.VolumeRecord{{ID: "volume-id", Labels: labels}},
		},
		LimaPathEvidence: pathSet, ComposePathEvidence: pathSet,
	}
}
