package state

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

func TestReconciliationGeneratedAuthorityAdvancesAcrossOperations(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-one")

	authority := ReconciliationGeneratedAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation,
		PlanDigest: publication.Plan.PlanDigest, OperationID: "operation-one",
	}
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	authority.BoxRoot = &publication.RemovalSnapshot.BoxRoot
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	finishOperation(t, store, "operation-one")
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-two")
	authority.OperationID = "operation-two"
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(3, 0)); err != nil {
		t.Fatalf("resume from later operation: %v", err)
	}
	loaded, found, err := store.ReconciliationGeneratedAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("loaded authority = %#v, %t, %v", loaded, found, err)
	}
}

func TestReconciliationGeneratedAuthorityRefusesDriftRegressionAndReplacement(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-generated")
	authority := ReconciliationGeneratedAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation, PlanDigest: publication.Plan.PlanDigest,
		OperationID: "operation-generated", BoxRoot: &publication.RemovalSnapshot.BoxRoot,
	}
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}

	regressed := authority
	regressed.BoxRoot = nil
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), regressed, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "cannot regress") {
		t.Fatalf("regression error = %v", err)
	}
	drifted := authority
	drifted.PlanDigest = contract.Digest("sha256:" + strings.Repeat("e", 64))
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), drifted, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "claimed identity is immutable") {
		t.Fatalf("drift error = %v", err)
	}
	replaced := authority
	root := *authority.BoxRoot
	root.Root.Identity.Inode++
	replaced.BoxRoot = &root
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), replaced, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "proof is immutable") {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestPublishReadyRetiresMatchingGeneratedAuthority(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-ready-generated")
	authority := ReconciliationGeneratedAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation, PlanDigest: publication.Plan.PlanDigest,
		OperationID: "operation-ready-generated", BoxRoot: &publication.RemovalSnapshot.BoxRoot,
	}
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishReady(t.Context(), publication); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReconciliationGeneratedAuthorityFor(t.Context(), authority.BoxID, authority.Generation); err != nil || found {
		t.Fatalf("partial authority after READY found=%t error=%v", found, err)
	}
}

func TestPublishReadyRefusesMismatchedGeneratedAuthority(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-ready-generated-drift")
	authorityRoot := publication.RemovalSnapshot.BoxRoot
	authority := ReconciliationGeneratedAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation, PlanDigest: publication.Plan.PlanDigest,
		OperationID: "operation-ready-generated-drift", BoxRoot: &authorityRoot,
	}
	if err := store.SaveReconciliationGeneratedAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	publication.RemovalSnapshot.BoxRoot.Root.Identity.Inode++
	if err := store.PublishReady(t.Context(), publication); err == nil || !strings.Contains(err.Error(), "differs from removal authority") {
		t.Fatalf("mismatched READY error = %v", err)
	}
	loaded, found, err := store.ReconciliationGeneratedAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("refused READY changed authority = %#v, %t, %v", loaded, found, err)
	}
}
