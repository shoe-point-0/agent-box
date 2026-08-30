package state

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReconciliationComposeAuthorityAdvancesAcrossOperations(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-compose-one")

	authority := composeAuthorityFixture(publication, "operation-compose-one")
	if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	records := publication.RemovalSnapshot.Compose
	authority.Records = &records
	if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}

	finishOperation(t, store, "operation-compose-one")
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-compose-two")
	authority.OperationID = "operation-compose-two"
	if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(3, 0)); err != nil {
		t.Fatalf("resume from later operation: %v", err)
	}
	loaded, found, err := store.ReconciliationComposeAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("loaded authority = %#v, %t, %v", loaded, found, err)
	}
}

func TestReconciliationComposeAuthorityRefusesDriftRegressionAndReplacement(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-compose-drift")
	authority := completeComposeAuthority(publication, "operation-compose-drift")
	if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}

	regressed := authority
	regressed.Records = nil
	if err := store.SaveReconciliationComposeAuthority(t.Context(), regressed, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "cannot regress") {
		t.Fatalf("regression error = %v", err)
	}
	drifted := authority
	drifted.Spec.DockerPath = "/other/docker"
	if err := store.SaveReconciliationComposeAuthority(t.Context(), drifted, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "claimed identity is immutable") {
		t.Fatalf("drift error = %v", err)
	}
	replaced := completeComposeAuthority(publication, authority.OperationID)
	replaced.Records.Resources[0].ID = "replacement-container"
	if err := store.SaveReconciliationComposeAuthority(t.Context(), replaced, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "resource proof is immutable") {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestPublishReadyRetiresOnlyMatchingComposeAuthority(t *testing.T) {
	t.Run("matching", func(t *testing.T) {
		store := openTestStore(t)
		publication := readyGenerationFixture(t)
		seedReadyBox(t, store, publication.Plan)
		seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-compose-ready")
		authority := completeComposeAuthority(publication, "operation-compose-ready")
		if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if err := store.PublishReady(t.Context(), publication); err != nil {
			t.Fatal(err)
		}
		if _, found, err := store.ReconciliationComposeAuthorityFor(t.Context(), authority.BoxID, authority.Generation); err != nil || found {
			t.Fatalf("partial authority after READY found=%t error=%v", found, err)
		}
	})

	t.Run("mismatched resources", func(t *testing.T) {
		store := openTestStore(t)
		publication := readyGenerationFixture(t)
		seedReadyBox(t, store, publication.Plan)
		seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-compose-mismatch")
		authority := completeComposeAuthority(publication, "operation-compose-mismatch")
		authority.Records.Resources[0].ID = "retained-container"
		if err := store.SaveReconciliationComposeAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if err := store.PublishReady(t.Context(), publication); err == nil || !strings.Contains(err.Error(), "partial Compose authority differs") {
			t.Fatalf("mismatched READY error = %v", err)
		}
		loaded, found, err := store.ReconciliationComposeAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
		if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
			t.Fatalf("refused READY changed authority = %#v, %t, %v", loaded, found, err)
		}
	})
}

func composeAuthorityFixture(publication ReadyGeneration, operationID string) ReconciliationComposeAuthority {
	return ReconciliationComposeAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation, PlanDigest: publication.Plan.PlanDigest,
		OperationID: operationID, Spec: publication.RemovalSnapshot.ComposeSpec,
	}
}

func completeComposeAuthority(publication ReadyGeneration, operationID string) ReconciliationComposeAuthority {
	authority := composeAuthorityFixture(publication, operationID)
	records := cloneRemovalSnapshot(publication.RemovalSnapshot).Compose
	authority.Records = &records
	return authority
}
