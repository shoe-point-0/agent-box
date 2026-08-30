package gitstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestRemovalManagerRemovesOnlyObservedPrivateStore(t *testing.T) {
	fixture := newFixture(t, false)
	record, err := (gitstore.Materializer{Git: fixture.client}).Materialize(t.Context(), fixture.facts, fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	boxRoot := filepath.Dir(filepath.Dir(fixture.facts.GitStorePath))
	unrelated := filepath.Join(filepath.Dir(boxRoot), "unrelated-box", "keep.txt")
	mustMkdir(t, filepath.Dir(unrelated))
	mustWrite(t, unrelated, "keep me\n")
	manager := gitstore.RemovalManager{BoxRoot: boxRoot, Paths: gitstore.OSPathRemover{}}

	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, record)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Present || observed.Repository != fixture.facts.Name || observed.CanonicalPath != record.GitCommonDir || observed.Identity != record.Identity {
		t.Fatalf("observation = %#v", observed)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, record, observed); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, record.GitCommonDir)
	contents, err := os.ReadFile(unrelated)
	if err != nil || string(contents) != "keep me\n" {
		t.Fatalf("unrelated Box marker = %q, %v", contents, err)
	}
	if _, err := os.Stat(fixture.source); err != nil {
		t.Fatalf("source repository changed: %v", err)
	}
}

func TestRemovalManagerRefusesReplacementAfterObservation(t *testing.T) {
	fixture := newFixture(t, false)
	record, err := (gitstore.Materializer{Git: fixture.client}).Materialize(t.Context(), fixture.facts, fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	manager := gitstore.RemovalManager{
		BoxRoot: filepath.Dir(filepath.Dir(fixture.facts.GitStorePath)),
		Paths:   gitstore.OSPathRemover{},
	}
	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, record)
	if err != nil {
		t.Fatal(err)
	}
	moved := record.GitCommonDir + "-moved"
	if err := os.Rename(record.GitCommonDir, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, record.GitCommonDir)
	marker := filepath.Join(record.GitCommonDir, "replacement.txt")
	mustWrite(t, marker, "do not remove\n")

	err = manager.Remove(t.Context(), fixture.facts, fixture.plan, record, observed)
	var conflict *gitstore.StoreConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Remove() error = %T %v, want StoreConflictError", err, err)
	}
	if contents, readErr := os.ReadFile(marker); readErr != nil || string(contents) != "do not remove\n" {
		t.Fatalf("replacement changed: %q, %v", contents, readErr)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("original inode was changed: %v", err)
	}
}

func TestRemovalManagerFreshRetryConvergesAfterInterruptedDelete(t *testing.T) {
	fixture := newFixture(t, false)
	record, err := (gitstore.Materializer{Git: fixture.client}).Materialize(t.Context(), fixture.facts, fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	paths := &deleteThenFail{}
	manager := gitstore.RemovalManager{
		BoxRoot: filepath.Dir(filepath.Dir(fixture.facts.GitStorePath)),
		Paths:   paths,
	}
	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, record, observed); err == nil || !strings.Contains(err.Error(), "injected interruption") {
		t.Fatalf("first Remove() error = %v", err)
	}
	terminal, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, record)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Present || terminal.Identity != (ownership.FileIdentity{}) {
		t.Fatalf("terminal observation = %#v", terminal)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, record, terminal); err != nil {
		t.Fatalf("retry Remove() error = %v", err)
	}
	if paths.calls != 1 {
		t.Fatalf("path remover calls = %d, want 1", paths.calls)
	}
}

type deleteThenFail struct {
	calls int
}

func (r *deleteThenFail) Remove(_ context.Context, proof ownership.PathProof) error {
	r.calls++
	if err := os.RemoveAll(proof.Path()); err != nil {
		return err
	}
	return errors.New("injected interruption")
}
