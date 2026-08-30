package up

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func TestReconcileReadyMaterializesContextAndPublishesFreshCompleteAuthority(t *testing.T) {
	fixture := newReadyFixture(t)
	stateStore, err := state.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })
	seedReadyIntent(t, stateStore, fixture.attempt)
	store := &recordingReadyStore{delegate: stateStore}
	config := fixture.config(store)

	if err := ReconcileReady(t.Context(), config, fixture.attempt, fixture.foundation, fixture.compose); err != nil {
		t.Fatal(err)
	}
	if fixture.gate.calls != 1 || store.calls != 1 {
		t.Fatalf("calls gate=%d store=%d", fixture.gate.calls, store.calls)
	}
	published := store.publication
	if published.PlanPath != fixture.attempt.PlanPath || published.ContextPath != fixture.attempt.ContextPath ||
		published.Generation != fixture.attempt.Generation || published.ObservedAt != time.Unix(101, 0).UTC() ||
		published.PublishedAt != time.Unix(102, 0).UTC() {
		t.Fatalf("publication identity = %#v", published)
	}
	if !reflect.DeepEqual(published.RemovalSnapshot, fixture.wantSnapshot()) {
		t.Fatalf("removal snapshot differs\ngot:  %#v\nwant: %#v", published.RemovalSnapshot, fixture.wantSnapshot())
	}
	retained, found, err := stateStore.CurrentRemovalSnapshot(t.Context(), fixture.attempt.Plan.BoxID)
	if err != nil || !found {
		t.Fatalf("retained READY authority: found=%t error=%v", found, err)
	}
	if retained.BoxID != fixture.attempt.Plan.BoxID || retained.Generation != fixture.attempt.Generation ||
		retained.PlanDigest != fixture.attempt.Plan.PlanDigest || retained.Lima.Name != fixture.foundation.Lima.Instance.Name ||
		len(retained.Compose.Resources) != len(fixture.compose.Records.Resources) ||
		retained.Compose.Resources[0].ID != fixture.compose.Records.Resources[0].ID ||
		len(retained.Repositories) != 1 ||
		retained.Repositories[0].Store.Identity != fixture.foundation.Git.Store.Identity {
		t.Fatalf("retained READY authority identity differs")
	}
	if len(published.Repositories) != 1 || published.Repositories[0].CurrentCommit != fixture.foundation.Git.Worktree.Commit ||
		published.Repositories[0].Dirty != fixture.foundation.Git.Worktree.Dirty {
		t.Fatalf("repository publication = %#v", published.Repositories)
	}
	contextBytes, err := os.ReadFile(fixture.attempt.ContextPath)
	if err != nil {
		t.Fatal(err)
	}
	if published.ContextDigest != contentDigest(contextBytes) {
		t.Fatalf("context digest = %q, want %q", published.ContextDigest, contentDigest(contextBytes))
	}
	runtimeBytes, err := os.ReadFile(fixture.attempt.LimaConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if published.RuntimeConfigDigest != contentDigest(runtimeBytes) {
		t.Fatalf("runtime digest = %q, want %q", published.RuntimeConfigDigest, contentDigest(runtimeBytes))
	}
	if !strings.Contains(string(contextBytes), `"state": "READY"`) || !strings.Contains(string(contextBytes), `"name": "workspace"`) {
		t.Fatalf("context does not carry fresh READY service evidence: %s", contextBytes)
	}
	contextIdentity, err := ownership.InspectPath(fixture.attempt.ContextPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReconcileReady(t.Context(), config, fixture.attempt, fixture.foundation, fixture.compose); err != nil {
		t.Fatalf("warm ReconcileReady() error = %v", err)
	}
	warmIdentity, err := ownership.InspectPath(fixture.attempt.ContextPath)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.gate.calls != 2 || store.calls != 2 || warmIdentity != contextIdentity {
		t.Fatalf("warm publication mutated context: gate=%d store=%d before=%#v after=%#v", fixture.gate.calls, store.calls, contextIdentity, warmIdentity)
	}
}

func TestReconcileReadyKeepsPublicationInsideReadinessAndContextBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*readyFixture, *ReadyConfig)
		want      string
		wantFile  bool
	}{
		{
			name: "readiness failure",
			configure: func(fixture *readyFixture, _ *ReadyConfig) {
				fixture.gate.err = errors.New("injected readiness failure")
			},
			want: "injected readiness failure",
		},
		{
			name: "context redaction failure",
			configure: func(_ *readyFixture, config *ReadyConfig) {
				config.Commands = []contract.CommandContext{{Name: "leak", Argv: []string{"run", "secret-canary"}}}
				config.SensitiveValues = []string{"secret-canary"}
			},
			want: "sensitive value 0 survived projection",
		},
		{
			name:      "READY transaction failure",
			configure: func(_ *readyFixture, _ *ReadyConfig) {},
			want:      "injected READY transaction failure",
			wantFile:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReadyFixture(t)
			store := &recordingReadyStore{}
			if test.name == "READY transaction failure" {
				store.err = errors.New(test.want)
			}
			config := fixture.config(store)
			test.configure(&fixture, &config)

			err := ReconcileReady(t.Context(), config, fixture.attempt, fixture.foundation, fixture.compose)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReconcileReady() error = %v, want %q", err, test.want)
			}
			if store.calls != boolInt(test.wantFile) {
				t.Fatalf("READY calls = %d, want %d", store.calls, boolInt(test.wantFile))
			}
			_, statErr := os.Lstat(fixture.attempt.ContextPath)
			if test.wantFile && statErr != nil {
				t.Fatalf("context missing after transaction failure: %v", statErr)
			}
			if !test.wantFile && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("context escaped failed boundary: %v", statErr)
			}
		})
	}
}

func TestReconcileReadyRefusesToReplaceRetainedContext(t *testing.T) {
	fixture := newReadyFixture(t)
	retained := []byte("retained-generation-context\n")
	if err := os.WriteFile(fixture.attempt.ContextPath, retained, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &recordingReadyStore{}

	err := ReconcileReady(t.Context(), fixture.config(store), fixture.attempt, fixture.foundation, fixture.compose)
	if err == nil || !strings.Contains(err.Error(), "retained context differs") {
		t.Fatalf("ReconcileReady() error = %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("READY publication escaped retained-context refusal: %d calls", store.calls)
	}
	got, err := os.ReadFile(fixture.attempt.ContextPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, retained) {
		t.Fatalf("retained context changed: %q", got)
	}
}

func TestReadyServicesRejectsNonReadyOrAmbiguousWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		containers []compose.ContainerObservation
	}{
		{"stopped", []compose.ContainerObservation{{Service: "workspace", State: "exited"}}},
		{"unhealthy", []compose.ContainerObservation{{Service: "workspace", State: "running", Health: "unhealthy"}}},
		{"missing workspace", []compose.ContainerObservation{{Service: "support", State: "running"}}},
		{"duplicate service", []compose.ContainerObservation{{Service: "workspace", State: "running"}, {Service: "workspace", State: "running"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readyServices("workspace", test.containers); err == nil {
				t.Fatal("readyServices() succeeded")
			}
		})
	}
}

type readyFixture struct {
	attempt    PreparedAttempt
	foundation FoundationResult
	compose    ComposeResult
	gate       *recordingFilesystemGate
}

func newReadyFixture(t *testing.T) readyFixture {
	t.Helper()
	gitFixture := newGitReconcileFixtureWithDriver(t, "lima-vz")
	boxRoot := filepath.Dir(gitFixture.workspaceRoot)
	for _, path := range []string{boxRoot, gitFixture.workspaceRoot, gitFixture.facts.GitStorePath, gitFixture.facts.WorktreePath} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	canonicalPlan, err := plancompiler.CanonicalJSON(gitFixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := PreparedAttempt{
		Operation: gitFixture.attempt("operation-ready").Operation,
		Plan:      gitFixture.plan, CanonicalPlan: canonicalPlan, Generation: 1, Repository: gitFixture.facts,
		BoxRoot: boxRoot, WorkspaceRoot: gitFixture.workspaceRoot,
		PlanPath: filepath.Join(boxRoot, "plan.json"), ContextPath: filepath.Join(boxRoot, "context.json"),
		LimaConfigPath: filepath.Join(boxRoot, "lima.yaml"),
	}
	if err := os.WriteFile(attempt.PlanPath, canonicalPlan, 0o600); err != nil {
		t.Fatal(err)
	}
	rootIdentity, err := ownership.InspectPath(boxRoot)
	if err != nil {
		t.Fatal(err)
	}
	storeIdentity, err := ownership.InspectPath(attempt.Repository.GitStorePath)
	if err != nil {
		t.Fatal(err)
	}
	worktreeIdentity, err := ownership.InspectPath(attempt.Repository.WorktreePath)
	if err != nil {
		t.Fatal(err)
	}
	storeRecord := gitstore.Record{
		Repository: attempt.Repository.Name, GitCommonDir: attempt.Repository.GitStorePath,
		BaseCommit: attempt.Repository.BaseCommit, PublishDestination: attempt.Repository.PublishDestination,
		Identity: storeIdentity,
	}
	worktreeObservation := worktree.Observation{
		Repository: attempt.Repository.Name, CanonicalPath: attempt.Repository.WorktreePath,
		CanonicalCommon: attempt.Repository.GitStorePath, Branch: attempt.Repository.Branch,
		Commit: attempt.Repository.BaseCommit, LockReason: "owned by Agent Box " + attempt.Plan.BoxID,
		Registered: true, BranchPresent: true, WorktreeIdentity: worktreeIdentity, StoreIdentity: storeIdentity,
	}

	imagePath := filepath.Join(t.TempDir(), "guest.img")
	if err := os.WriteFile(imagePath, []byte("guest image\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	limaSpec := lima.Spec{
		RuntimeHost: attempt.Plan.RuntimeHost, Mounts: attempt.Plan.Mounts,
		BoxID: attempt.Plan.BoxID, PlanDigest: attempt.Plan.PlanDigest, Generation: attempt.Generation,
		GuestImage: toolchain.Identity{Component: toolchain.GuestImage, Version: "fixture", Digest: repeatedDigest("d")},
		ImagePath:  imagePath,
	}
	limaSpec.PathEvidence, err = lima.RecordRuntimePaths(limaSpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := lima.WriteConfig(attempt.LimaConfigPath, limaSpec); err != nil {
		t.Fatal(err)
	}
	configEvidence, err := ownership.RecordPathSet([]string{attempt.LimaConfigPath})
	if err != nil {
		t.Fatal(err)
	}
	instance := ownership.LimaInstanceRecord{Name: attempt.Plan.RuntimeHost.InstanceName, Metadata: map[string]string{
		ownership.BoxIDFact: attempt.Plan.BoxID, "dev.abox.plan-digest": string(attempt.Plan.PlanDigest), "dev.abox.generation": "1",
	}}

	includePath := attempt.Plan.Compose.Includes[0].Path
	if err := os.WriteFile(includePath, []byte("services:\n  workspace:\n    image: fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := []byte(`{"services":{"workspace":{"image":"fixture"}}}`)
	resolvedSum := sha256.Sum256(resolved)
	compiled := compose.Result{
		ProjectName: attempt.Plan.Compose.ProjectName,
		RootPath:    filepath.Join(boxRoot, "compose.root.yaml"), OverridePath: filepath.Join(boxRoot, "compose.abox.yaml"),
		ResolvedModelPath: filepath.Join(boxRoot, "compose.resolved.redacted.json"), ResolvedModel: resolved,
		ResolvedModelDigest: contract.Digest(fmt.Sprintf("sha256:%x", resolvedSum)),
	}
	for path, data := range map[string][]byte{
		compiled.RootPath: []byte("include: []\n"), compiled.OverridePath: []byte("services: {}\n"), compiled.ResolvedModelPath: resolved,
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	composeEvidence, err := compose.RecordRuntimePaths(attempt.Plan, compiled)
	if err != nil {
		t.Fatal(err)
	}
	runtimeSpec := compose.RuntimeSpec{
		RuntimeHost: limaSpec, Plan: attempt.Plan, Generation: attempt.Generation,
		DockerPath: "/usr/bin/docker", Workdir: boxRoot, Compiled: compiled, PathEvidence: composeEvidence,
	}
	labels := map[string]string{
		"com.docker.compose.project": attempt.Plan.Compose.ProjectName,
		ownership.ComposeBoxIDFact:   attempt.Plan.BoxID, "dev.abox.plan": string(attempt.Plan.PlanDigest), "dev.abox.generation": "1",
	}
	mounts := make([]compose.MountObservation, 0, len(attempt.Plan.Mounts))
	for _, mount := range attempt.Plan.Mounts {
		mounts = append(mounts, compose.MountObservation{Type: "bind", Source: mount.Path, Destination: mount.Path, ReadWrite: mount.Writable})
	}
	observation := compose.RuntimeObservation{
		ProjectName: attempt.Plan.Compose.ProjectName,
		Containers: []compose.ContainerObservation{{
			ID: "container-ready", Service: attempt.Plan.WorkspaceService.Name, State: "running", Health: "healthy",
			Labels: cloneStringMap(labels), Mounts: mounts,
		}},
		Networks: []compose.NetworkObservation{{ID: "network-ready", Labels: cloneStringMap(labels)}},
	}
	composeResult := ComposeResult{Spec: runtimeSpec, Records: compose.RecordsFromObservation(observation), Observation: observation}
	gate := &recordingFilesystemGate{observation: filesystemObservation(observation)}
	foundation := FoundationResult{
		Generated: GeneratedPlanResult{BoxRoot: generatedstate.Record{BoxID: attempt.Plan.BoxID, Root: ownership.PathRecord{CanonicalPath: boxRoot, Identity: rootIdentity}}},
		Git:       GitResult{Store: storeRecord, Worktree: worktreeObservation},
		Lima: LimaResult{
			Artifact: toolchain.Identity{Component: toolchain.Lima, Version: lima.SupportedVersion, Digest: repeatedDigest("c")},
			Spec:     limaSpec, ConfigEvidence: configEvidence, Instance: instance,
			Observation: lima.Observation{Name: instance.Name, State: lima.Running, Metadata: cloneStringMap(instance.Metadata)},
		},
	}
	return readyFixture{attempt: attempt, foundation: foundation, compose: composeResult, gate: gate}
}

func (f readyFixture) config(store ReadyStore) ReadyConfig {
	return ReadyConfig{
		Filesystem: FilesystemConfig{Gate: f.gate}, Store: store,
		Commands: []contract.CommandContext{{Name: "status", Argv: []string{"abox", "status", f.attempt.Plan.TaskKey}}},
		Now:      monotonicNow(),
	}
}

func (f readyFixture) wantSnapshot() state.RemovalSnapshot {
	return state.RemovalSnapshot{
		BoxID: f.attempt.Plan.BoxID, Generation: f.attempt.Generation, PlanDigest: f.attempt.Plan.PlanDigest,
		BoxRoot:      f.foundation.Generated.BoxRoot,
		Repositories: []state.RepositoryRemovalRecord{{Facts: f.attempt.Repository, Store: f.foundation.Git.Store}},
		LimaArtifact: f.foundation.Lima.Artifact, LimaSpec: f.foundation.Lima.Spec, Lima: f.foundation.Lima.Instance,
		ComposeSpec: f.compose.Spec, Compose: f.compose.Records,
		LimaPathEvidence: f.foundation.Lima.Spec.PathEvidence, ComposePathEvidence: f.compose.Spec.PathEvidence,
	}
}

func filesystemObservation(observation compose.RuntimeObservation) filesystem.Observation {
	return filesystem.Observation{Docker: observation}
}

type recordingReadyStore struct {
	publication state.ReadyGeneration
	delegate    ReadyStore
	err         error
	calls       int
}

func (s *recordingReadyStore) PublishReady(ctx context.Context, publication state.ReadyGeneration) error {
	s.calls++
	s.publication = publication
	if s.delegate != nil {
		return s.delegate.PublishReady(ctx, publication)
	}
	return s.err
}

func seedReadyIntent(t *testing.T, store *state.Store, attempt PreparedAttempt) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at) VALUES (?, ?, ?, 1, 1)`,
			attempt.Plan.WorkspaceID, attempt.Plan.Manifest.Path, attempt.Plan.Manifest.Digest); err != nil {
			return err
		}
		_, err := tx.ExecContext(t.Context(), `INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, created_at, updated_at)
			VALUES (?, ?, ?, 'planning', 1, 1)`, attempt.Plan.BoxID, attempt.Plan.WorkspaceID, attempt.Plan.TaskKey)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func repeatedDigest(value string) contract.Digest {
	return contract.Digest("sha256:" + strings.Repeat(value, 64))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
