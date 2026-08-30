package removal

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestAssembleCurrentReconstructsDependencyOrderedCoordinator(t *testing.T) {
	t.Parallel()
	provider := testAssemblyPaths(t)
	snapshot := testAssemblySnapshot(t, provider)
	config := testAssemblyConfig(provider, snapshot)

	coordinator, err := AssembleCurrent(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, step := range coordinator.steps {
		got = append(got, step.Key())
	}
	want := []string{
		"compose/auth-142-itjwb74g",
		"runtime-host/auth-142-itjwb74g",
		"worktree/repository-a",
		"private-store/repository-a",
		"generated-state/auth-142-itjwb74g",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assembled step order = %v, want %v", got, want)
	}
}

func TestAssembleCurrentFailsClosedWithoutCurrentAuthority(t *testing.T) {
	t.Parallel()
	provider := testAssemblyPaths(t)
	config := testAssemblyConfig(provider, state.RemovalSnapshot{})
	config.Snapshots = snapshotStub{}

	coordinator, err := AssembleCurrent(t.Context(), config)
	if coordinator != nil || !errors.Is(err, ErrRemovalSnapshotAbsent) {
		t.Fatalf("AssembleCurrent() = %v, %v, want nil, snapshot-absent error", coordinator, err)
	}
}

func TestAssembleCurrentRejectsOperationAndPathDrift(t *testing.T) {
	t.Parallel()
	provider := testAssemblyPaths(t)
	tests := []struct {
		name   string
		mutate func(*AssemblyConfig, *state.RemovalSnapshot)
	}{
		{name: "operation kind", mutate: func(config *AssemblyConfig, _ *state.RemovalSnapshot) {
			config.Operation.Kind = "up"
		}},
		{name: "snapshot Box", mutate: func(_ *AssemblyConfig, snapshot *state.RemovalSnapshot) {
			snapshot.BoxID = "task-1-abcdefgh"
		}},
		{name: "configured root", mutate: func(_ *AssemblyConfig, snapshot *state.RemovalSnapshot) {
			snapshot.BoxRoot.Root.CanonicalPath = filepath.Join(provider.BoxesRoot(), "task-1-abcdefgh")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := testAssemblySnapshot(t, provider)
			config := testAssemblyConfig(provider, snapshot)
			test.mutate(&config, &snapshot)
			config.Snapshots = snapshotStub{snapshot: snapshot, found: true}
			if coordinator, err := AssembleCurrent(t.Context(), config); err == nil || coordinator != nil {
				t.Fatalf("AssembleCurrent() = %v, %v, want fail-closed refusal", coordinator, err)
			}
		})
	}
}

func testAssemblyPaths(t *testing.T) *paths.Provider {
	t.Helper()
	root := t.TempDir()
	provider, err := paths.New(filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "log"))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func testAssemblySnapshot(t *testing.T, provider *paths.Provider) state.RemovalSnapshot {
	t.Helper()
	boxRoot, err := provider.BoxRoot("auth-142-itjwb74g")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot, err := provider.WorkspaceRoot("auth-142-itjwb74g")
	if err != nil {
		t.Fatal(err)
	}
	gitRoot, err := provider.GitRoot("auth-142-itjwb74g")
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repository-a", SourceRoot: "/source/repository-a", Clean: true,
		BaseCommit: "0123456789012345678901234567890123456789", Branch: "abox/task/repository-a",
		WorktreePath: filepath.Join(workspaceRoot, "repository-a"), GitStorePath: filepath.Join(gitRoot, "repository-a.git"),
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: "/source/repository-a"},
	}
	digest := contract.Digest("sha256:plan")
	immutable := contract.Plan{
		BoxID: "auth-142-itjwb74g", PlanDigest: digest,
		Repositories: []contract.PlanRepository{{
			Name: facts.Name, SourcePath: facts.SourceRoot, BaseCommit: facts.BaseCommit,
			Branch: facts.Branch, WorktreePath: facts.WorktreePath, GitCommonDir: facts.GitStorePath,
		}},
		RuntimeHost: contract.RuntimeHostPlan{InstanceName: "auth-142-itjwb74g"},
		Compose:     contract.ComposePlan{ProjectName: "auth-142-itjwb74g"},
	}
	limaSpec := lima.Spec{BoxID: "auth-142-itjwb74g", PlanDigest: digest, Generation: 1, RuntimeHost: immutable.RuntimeHost}
	return state.RemovalSnapshot{
		BoxID: "auth-142-itjwb74g", Generation: 1, PlanDigest: digest,
		BoxRoot: generatedstate.Record{
			BoxID: "auth-142-itjwb74g", Root: ownership.PathRecord{CanonicalPath: boxRoot, Identity: ownership.FileIdentity{Device: 1, Inode: 2}},
		},
		Repositories: []state.RepositoryRemovalRecord{{
			Facts: facts,
			Store: gitstore.Record{
				Repository: facts.Name, GitCommonDir: facts.GitStorePath,
				Identity: ownership.FileIdentity{Device: 3, Inode: 4}, BaseCommit: facts.BaseCommit,
				PublishDestination: facts.PublishDestination,
			},
		}},
		LimaSpec: limaSpec,
		Lima: ownership.LimaInstanceRecord{
			Name: "auth-142-itjwb74g", Metadata: map[string]string{"dev.abox.box-id": "auth-142-itjwb74g"},
		},
		ComposeSpec: compose.RuntimeSpec{
			RuntimeHost: limaSpec, Plan: immutable, Generation: 1,
			Compiled: compose.Result{ProjectName: "auth-142-itjwb74g"},
		},
	}
}

func testAssemblyConfig(provider *paths.Provider, snapshot state.RemovalSnapshot) AssemblyConfig {
	return AssemblyConfig{
		Snapshots: snapshotStub{snapshot: snapshot, found: true},
		Journal:   leaseJournalStub{},
		Operation: journal.Operation{ID: "operation-a", BoxID: "auth-142-itjwb74g", Kind: "remove", State: journal.StateRunning},
		Owner: journal.Owner{
			InstanceID: "daemon-a", ProcessStartID: "process-a", BootID: "boot-a",
		},
		Quiescer:   quiescerStub{},
		Paths:      provider,
		MachineGit: machinegit.Client{},
		Lima:       limaRuntimeStub{},
		Compose:    composeRuntimeStub{},
		AppendEvent: func(context.Context, string, string, json.RawMessage, time.Time) error {
			return nil
		},
	}
}

type snapshotStub struct {
	snapshot state.RemovalSnapshot
	found    bool
	err      error
}

func (s snapshotStub) CurrentRemovalSnapshot(context.Context, string) (state.RemovalSnapshot, bool, error) {
	return s.snapshot, s.found, s.err
}

type leaseJournalStub struct{}

func (leaseJournalStub) AcquireLease(context.Context, string, string, journal.Owner, time.Time, time.Duration) (journal.Lease, error) {
	panic("not called during assembly")
}
func (leaseJournalStub) RenewLease(context.Context, journal.Lease, time.Time, time.Duration) (journal.Lease, error) {
	panic("not called during assembly")
}
func (leaseJournalStub) ReleaseLease(context.Context, journal.Lease, time.Time) error {
	panic("not called during assembly")
}

type quiescerStub struct{}

func (quiescerStub) Quiesce(context.Context, Lease) error { return nil }
func (quiescerStub) Resume(context.Context, Lease) error  { return nil }

type limaRuntimeStub struct{}

func (limaRuntimeStub) ObserveRemoval(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error) {
	panic("not called during assembly")
}
func (limaRuntimeStub) Delete(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error) {
	panic("not called during assembly")
}

type composeRuntimeStub struct{}

func (composeRuntimeStub) Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	panic("not called during assembly")
}
func (composeRuntimeStub) Remove(context.Context, compose.RuntimeSpec, compose.RuntimeRecords) (compose.RuntimeObservation, error) {
	panic("not called during assembly")
}
