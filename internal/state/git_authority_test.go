package state

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestReconciliationGitAuthorityAdvancesMonotonicallyAcrossOperations(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-one")

	authority := gitAuthorityFixture(publication, "operation-one")
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := store.ReconciliationGitAuthorityFor(t.Context(), authority.BoxID, authority.Generation, authority.Repository.Name)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("claim load = %#v, %t, %v", loaded, found, err)
	}

	authority.Store = &publication.Repositories[0].Store
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	authority.Worktree = &GitWorktreeAuthority{
		CanonicalPath: authority.Repository.WorktreePath, GitCommonDir: authority.Repository.GitStorePath,
		Branch: authority.Repository.Branch, LockReason: "owned by Agent Box " + authority.BoxID,
		PathIdentity: ownership.FileIdentity{Device: 9, Inode: 10},
	}
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}

	finishOperation(t, store, "operation-one")
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-two")
	authority.OperationID = "operation-two"
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(4, 0)); err != nil {
		t.Fatalf("resume from a later operation: %v", err)
	}
	loaded, found, err = store.ReconciliationGitAuthorityFor(t.Context(), authority.BoxID, authority.Generation, authority.Repository.Name)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("resumed authority = %#v, %t, %v", loaded, found, err)
	}
}

func TestReconciliationGitAuthorityRefusesDriftAndRegression(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-drift")
	authority := gitAuthorityFixture(publication, "operation-drift")
	authority.Store = &publication.Repositories[0].Store
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}

	regressed := authority
	regressed.Store = nil
	if err := store.SaveReconciliationGitAuthority(t.Context(), regressed, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "cannot regress") {
		t.Fatalf("regression error = %v", err)
	}
	drifted := authority
	drifted.Repository.Branch = "abox/other"
	if err := store.SaveReconciliationGitAuthority(t.Context(), drifted, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "claimed identity is immutable") {
		t.Fatalf("drift error = %v", err)
	}
	loaded, found, err := store.ReconciliationGitAuthorityFor(t.Context(), authority.BoxID, authority.Generation, authority.Repository.Name)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("refused writes changed authority = %#v, %t, %v", loaded, found, err)
	}
}

func TestPublishReadyRetiresOnlyMatchingPartialGitAuthority(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-ready")
	authority := gitAuthorityFixture(publication, "operation-ready")
	authority.Store = &publication.Repositories[0].Store
	authority.Worktree = &GitWorktreeAuthority{
		CanonicalPath: authority.Repository.WorktreePath, GitCommonDir: authority.Repository.GitStorePath,
		Branch: authority.Repository.Branch, LockReason: "owned by Agent Box " + authority.BoxID,
		PathIdentity: ownership.FileIdentity{Device: 9, Inode: 10},
	}
	if err := store.SaveReconciliationGitAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishReady(t.Context(), publication); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReconciliationGitAuthorityFor(t.Context(), authority.BoxID, authority.Generation, authority.Repository.Name); err != nil || found {
		t.Fatalf("partial authority after READY found=%t error=%v", found, err)
	}
}

func gitAuthorityFixture(publication ReadyGeneration, operationID string) ReconciliationGitAuthority {
	return ReconciliationGitAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation,
		Repository: publication.Repositories[0].Facts, PlanDigest: publication.Plan.PlanDigest,
		OperationID: operationID,
	}
}

func seedRunningUpOperation(t *testing.T, store *Store, boxID, operationID string) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO operations
			(id, box_id, idempotency_key, kind, recorded_state, started_at, request_digest)
			VALUES (?, ?, ?, 'up', 'running', 1, ?)`, operationID, boxID, operationID, "sha256:"+strings.Repeat("1", 64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func finishOperation(t *testing.T, store *Store, operationID string) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE operations SET recorded_state = 'failed', finished_at = 2 WHERE id = ?`, operationID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
