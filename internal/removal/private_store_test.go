package removal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/removal"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestPrivateStoreStepBindsPathAndInodeAndRefusesDrift(t *testing.T) {
	t.Parallel()
	log := []string{}
	facts := repository.Facts{Name: "repo"}
	store := gitstore.Record{
		Repository: "repo", GitCommonDir: "/state/boxes/selected/git/repo.git",
		BaseCommit: "1111111111111111111111111111111111111111",
		Identity:   ownership.FileIdentity{Device: 7, Inode: 11},
	}
	observer := &stubPrivateStore{
		observation: gitstore.RemovalObservation{
			Repository: "repo", CanonicalPath: store.GitCommonDir,
			Identity: store.Identity, Present: true,
		},
	}
	step, err := removal.NewPrivateStoreStep(removal.PrivateStoreStepConfig{
		Facts: facts, Plan: contract.Plan{}, Store: store, Observer: observer, Remover: observer,
	})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: fakeGuard{&log}, Quiescer: fakeQuiescer{&log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), "selected")
	if err != nil {
		t.Fatal(err)
	}
	resource := preview.Resources[0]
	if resource.Key != "private-store/repo" || resource.Stage != removal.StagePrivateStore ||
		resource.Evidence["path"] != store.GitCommonDir || resource.Evidence["storeIdentity"] != "7:11" ||
		resource.Evidence["baseCommit"] != store.BaseCommit || len(resource.Losses) != 0 {
		t.Fatalf("resource = %#v", resource)
	}

	observer.observation.Identity.Inode++
	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest, Yes: true})
	if !errors.Is(err, removal.ErrInventoryDrift) || observer.removes != 0 {
		t.Fatalf("drifted Execute() = %v, remove calls = %d", err, observer.removes)
	}
}

type stubPrivateStore struct {
	observation gitstore.RemovalObservation
	removes     int
}

func (s *stubPrivateStore) ObserveRemoval(context.Context, repository.Facts, contract.Plan, gitstore.Record) (gitstore.RemovalObservation, error) {
	return s.observation, nil
}

func (s *stubPrivateStore) Remove(context.Context, repository.Facts, contract.Plan, gitstore.Record, gitstore.RemovalObservation) error {
	s.removes++
	return nil
}
