package state

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

func TestRemovalSnapshotRoundTripsAndIsImmutablePerGeneration(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	snapshot := testRemovalSnapshot()
	seedCurrentGeneration(t, store, snapshot.BoxID, snapshot.Generation, string(snapshot.PlanDigest))

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

	drifted := cloneRemovalSnapshot(snapshot)
	drifted.Compose.Resources[0].ID = "replacement-container-id"
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
	snapshot := testRemovalSnapshot()
	seedCurrentGeneration(t, store, snapshot.BoxID, snapshot.Generation, string(snapshot.PlanDigest))
	if err := store.SaveRemovalSnapshot(t.Context(), snapshot); err != nil {
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

func TestRemovalSnapshotRejectsRuntimeSpecificationsThatCannotBeReconstructed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*RemovalSnapshot)
	}{
		{name: "Lima executable", mutate: func(snapshot *RemovalSnapshot) { snapshot.LimaArtifact.Version = "2.0.1" }},
		{name: "Lima generation", mutate: func(snapshot *RemovalSnapshot) { snapshot.LimaSpec.Generation++ }},
		{name: "Lima path evidence", mutate: func(snapshot *RemovalSnapshot) { snapshot.LimaSpec.PathEvidence.Paths = nil }},
		{name: "Compose Plan", mutate: func(snapshot *RemovalSnapshot) { snapshot.ComposeSpec.Plan.BoxID = "other-box" }},
		{name: "Compose runtime host", mutate: func(snapshot *RemovalSnapshot) { snapshot.ComposeSpec.RuntimeHost.ImagePath = "/other/image" }},
		{name: "Compose model", mutate: func(snapshot *RemovalSnapshot) {
			snapshot.ComposeSpec.Compiled.ResolvedModel = []byte(`{"services":{}}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := testRemovalSnapshot()
			test.mutate(&snapshot)
			store := openTestStore(t)
			seedCurrentGeneration(t, store, snapshot.BoxID, snapshot.Generation, string(snapshot.PlanDigest))
			if err := store.SaveRemovalSnapshot(t.Context(), snapshot); err == nil {
				t.Fatal("SaveRemovalSnapshot() accepted a runtime specification that cannot reconstruct removal")
			}
		})
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
	planDigest := contract.Digest("sha256:" + strings.Repeat("a", 64))
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
	labels := map[string]string{
		ownership.ComposeBoxIDFact: boxID, "com.docker.compose.project": boxID,
		"dev.abox.plan": string(planDigest), "dev.abox.generation": "1",
	}
	immutable := contract.Plan{
		SchemaVersion: contract.PlanSchemaVersion, BoxID: boxID, PlanDigest: planDigest,
		RuntimeHost: contract.RuntimeHostPlan{Driver: "lima-vz", InstanceName: boxID},
		Compose:     contract.ComposePlan{ProjectName: boxID},
		Mounts:      []contract.MountPlan{{Path: facts.WorktreePath, Services: []string{"workspace"}, Writable: true}},
	}
	limaSpec := lima.Spec{
		RuntimeHost: immutable.RuntimeHost, Mounts: immutable.Mounts, BoxID: boxID,
		PlanDigest: planDigest, Generation: 1,
		GuestImage: toolchain.Identity{
			Component: toolchain.GuestImage, Version: "test",
			Digest: contract.Digest("sha256:" + strings.Repeat("b", 64)),
		},
		ImagePath: "/tools/guest.img", PathEvidence: pathSet,
	}
	resolvedModel := []byte(`{"services":{"workspace":{"image":"example.invalid/workspace@sha256:deadbeef"}}}`)
	resolvedDigest := sha256.Sum256(resolvedModel)
	composeSpec := compose.RuntimeSpec{
		RuntimeHost: limaSpec, Plan: immutable, Generation: 1, DockerPath: "/usr/bin/docker",
		Workdir: "/state/boxes/box-a", PathEvidence: pathSet,
		Compiled: compose.Result{
			ProjectName: boxID, RootPath: "/state/boxes/box-a/compose.root.yaml",
			OverridePath:      "/state/boxes/box-a/compose.abox.yaml",
			ResolvedModelPath: "/state/boxes/box-a/compose.resolved.redacted.json",
			ResolvedModel:     resolvedModel, ResolvedModelDigest: contract.Digest(fmt.Sprintf("sha256:%x", resolvedDigest)),
		},
	}
	return RemovalSnapshot{
		BoxID: boxID, Generation: 1, PlanDigest: planDigest,
		BoxRoot: generatedstate.Record{BoxID: boxID, Root: ownership.PathRecord{CanonicalPath: "/state/boxes/box-a", Identity: identity}},
		Repositories: []RepositoryRemovalRecord{{
			Facts: facts,
			Store: gitstore.Record{Repository: facts.Name, GitCommonDir: facts.GitStorePath, Identity: ownership.FileIdentity{Device: 7, Inode: 20}, BaseCommit: facts.BaseCommit, PublishDestination: destination},
		}},
		LimaArtifact: toolchain.Identity{
			Component: toolchain.Lima, Version: lima.SupportedVersion,
			Digest: contract.Digest("sha256:" + strings.Repeat("a", 64)),
		},
		LimaSpec: limaSpec,
		Lima: ownership.LimaInstanceRecord{Name: boxID, Metadata: map[string]string{
			ownership.BoxIDFact: boxID, "dev.abox.plan-digest": string(planDigest), "dev.abox.generation": "1",
		}},
		ComposeSpec: composeSpec,
		Compose: compose.RuntimeRecords{
			Resources: []ownership.ComposeResourceRecord{{Kind: "container", ID: "container-id", Labels: labels}},
			Volumes:   []ownership.VolumeRecord{{ID: "volume-id", Labels: labels}},
		},
		LimaPathEvidence: pathSet, ComposePathEvidence: pathSet,
	}
}
