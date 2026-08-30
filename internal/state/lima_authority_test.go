package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestReconciliationLimaAuthorityAdvancesAcrossOperations(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-lima-one")

	authority := limaAuthorityFixture(t, publication, "operation-lima-one")
	if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	authority.ConfigEvidence = recordConfigEvidence(t, authority.ConfigPath)
	if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	instance := publication.RemovalSnapshot.Lima
	authority.Instance = &instance
	if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}

	finishOperation(t, store, "operation-lima-one")
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-lima-two")
	authority.OperationID = "operation-lima-two"
	if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(4, 0)); err != nil {
		t.Fatalf("resume from later operation: %v", err)
	}
	loaded, found, err := store.ReconciliationLimaAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
	if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
		t.Fatalf("loaded authority = %#v, %t, %v", loaded, found, err)
	}
}

func TestReconciliationLimaAuthorityRefusesDriftRegressionAndReplacement(t *testing.T) {
	store := openTestStore(t)
	publication := readyGenerationFixture(t)
	seedReadyBox(t, store, publication.Plan)
	seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-lima-drift")
	authority := limaAuthorityFixture(t, publication, "operation-lima-drift")
	authority.ConfigEvidence = recordConfigEvidence(t, authority.ConfigPath)
	instance := publication.RemovalSnapshot.Lima
	authority.Instance = &instance
	if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}

	regressed := authority
	regressed.Instance = nil
	if err := store.SaveReconciliationLimaAuthority(t.Context(), regressed, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "cannot regress") {
		t.Fatalf("regression error = %v", err)
	}
	drifted := authority
	drifted.Spec.RuntimeHost.MemoryBytes++
	if err := store.SaveReconciliationLimaAuthority(t.Context(), drifted, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "claimed identity is immutable") {
		t.Fatalf("drift error = %v", err)
	}
	replaced := authority
	configEvidence := *authority.ConfigEvidence
	configEvidence.Paths = append([]ownership.PathChainRecord(nil), configEvidence.Paths...)
	configEvidence.Paths[0].Ancestors = append([]ownership.PathRecord(nil), configEvidence.Paths[0].Ancestors...)
	configEvidence.Paths[0].Ancestors[len(configEvidence.Paths[0].Ancestors)-1].Identity.Inode++
	replaced.ConfigEvidence = &configEvidence
	if err := store.SaveReconciliationLimaAuthority(t.Context(), replaced, time.Unix(2, 0)); err == nil || !strings.Contains(err.Error(), "config proof is immutable") {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestPublishReadyRetiresOnlyMatchingLimaAuthority(t *testing.T) {
	t.Run("matching", func(t *testing.T) {
		store := openTestStore(t)
		publication := readyGenerationFixture(t)
		seedReadyBox(t, store, publication.Plan)
		seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-lima-ready")
		authority := completeLimaAuthority(t, publication, "operation-lima-ready")
		if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if err := store.PublishReady(t.Context(), publication); err != nil {
			t.Fatal(err)
		}
		if _, found, err := store.ReconciliationLimaAuthorityFor(t.Context(), authority.BoxID, authority.Generation); err != nil || found {
			t.Fatalf("partial authority after READY found=%t error=%v", found, err)
		}
	})

	t.Run("mismatched artifact", func(t *testing.T) {
		store := openTestStore(t)
		publication := readyGenerationFixture(t)
		seedReadyBox(t, store, publication.Plan)
		seedRunningUpOperation(t, store, publication.Plan.BoxID, "operation-lima-mismatch")
		authority := completeLimaAuthority(t, publication, "operation-lima-mismatch")
		authority.LimaArtifact.Digest = contract.Digest("sha256:" + strings.Repeat("e", 64))
		if err := store.SaveReconciliationLimaAuthority(t.Context(), authority, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if err := store.PublishReady(t.Context(), publication); err == nil || !strings.Contains(err.Error(), "partial Lima authority differs") {
			t.Fatalf("mismatched READY error = %v", err)
		}
		loaded, found, err := store.ReconciliationLimaAuthorityFor(t.Context(), authority.BoxID, authority.Generation)
		if err != nil || !found || !reflect.DeepEqual(loaded, authority) {
			t.Fatalf("refused READY changed authority = %#v, %t, %v", loaded, found, err)
		}
	})
}

func limaAuthorityFixture(t *testing.T, publication ReadyGeneration, operationID string) ReconciliationLimaAuthority {
	t.Helper()
	return ReconciliationLimaAuthority{
		BoxID: publication.Plan.BoxID, Generation: publication.Generation, PlanDigest: publication.Plan.PlanDigest,
		OperationID: operationID, LimaArtifact: publication.RemovalSnapshot.LimaArtifact,
		Spec: publication.RemovalSnapshot.LimaSpec, ConfigPath: filepath.Join(filepath.Dir(publication.PlanPath), "lima.yaml"),
	}
}

func completeLimaAuthority(t *testing.T, publication ReadyGeneration, operationID string) ReconciliationLimaAuthority {
	t.Helper()
	authority := limaAuthorityFixture(t, publication, operationID)
	authority.ConfigEvidence = recordConfigEvidence(t, authority.ConfigPath)
	instance := publication.RemovalSnapshot.Lima
	authority.Instance = &instance
	return authority
}

func recordConfigEvidence(t *testing.T, path string) *ownership.PathSetRecord {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := ownership.RecordPathSet([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	return &record
}
