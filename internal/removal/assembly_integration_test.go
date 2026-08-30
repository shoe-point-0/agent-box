package removal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func TestAssembledPersistedRemovalOwnsFullDisposableBoxLifecycle(t *testing.T) {
	t.Run("default refusal and explicit selected-Box removal", func(t *testing.T) {
		fixture := newFullRemovalFixture(t)
		preview, failure := fixture.attempt(OperationInput{}, nil)
		if failure == nil || failure.Code != contract.CodeCleanupRefused {
			t.Fatalf("default removal failure = %#v, want cleanup refusal", failure)
		}
		fixture.assertExactLosses(t, preview)
		fixture.assertAllSelectedResourcesPresent(t)
		fixture.assertSiblingUnchanged(t)

		failure = fixture.authorizedAttempt(preview, nil)
		if failure != nil {
			t.Fatalf("authorized removal failure = %#v", failure)
		}
		fixture.assertSelectedResourcesRemoved(t)
		fixture.assertSiblingUnchanged(t)
	})

	t.Run("post-preview safety drift", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*fullRemovalFixture)
			assert func(*testing.T, *fullRemovalFixture)
		}{
			{
				name: "dirtiness",
				mutate: func(f *fullRemovalFixture) {
					f.write(filepath.Join(f.facts.WorktreePath, "post-preview.txt"), "new dirtiness\n")
				},
			},
			{
				name: "unpublished commit",
				mutate: func(f *fullRemovalFixture) {
					f.write(filepath.Join(f.facts.WorktreePath, "later-commit.txt"), "later\n")
					f.git(f.facts.WorktreePath, "add", "later-commit.txt")
					f.git(f.facts.WorktreePath, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "post-preview")
				},
			},
			{
				name: "worktree inode",
				mutate: func(f *fullRemovalFixture) {
					f.displacedWorktree = f.facts.WorktreePath + "-displaced"
					if err := os.Rename(f.facts.WorktreePath, f.displacedWorktree); err != nil {
						f.t.Fatal(err)
					}
					f.write(filepath.Join(f.facts.WorktreePath, "replacement.txt"), "foreign replacement\n")
				},
				assert: func(t *testing.T, f *fullRemovalFixture) {
					assertFileContents(t, filepath.Join(f.facts.WorktreePath, "replacement.txt"), "foreign replacement\n")
					if _, err := os.Stat(f.displacedWorktree); err != nil {
						t.Fatalf("original worktree inode changed: %v", err)
					}
				},
			},
			{
				name: "Compose ownership labels",
				mutate: func(f *fullRemovalFixture) {
					f.compose.observation.Containers[0].Labels = maps.Clone(f.compose.observation.Containers[0].Labels)
					f.compose.observation.Containers[0].Labels[ownership.ComposeBoxIDFact] = "foreign-box"
				},
			},
			{
				name: "volume driver policy",
				mutate: func(f *fullRemovalFixture) {
					f.compose.observation.Volumes[0].Driver = "changed-after-preview"
				},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture := newFullRemovalFixture(t)
				preview, failure := fixture.attempt(OperationInput{}, nil)
				if failure == nil || failure.Code != contract.CodeCleanupRefused {
					t.Fatalf("preview failure = %#v, want cleanup refusal", failure)
				}
				failure = fixture.authorizedAttempt(preview, func() { test.mutate(fixture) })
				if failure == nil || failure.Code == contract.CodeCLIUsage || failure.Code == contract.CodeInternalInvariant {
					t.Fatalf("drifted removal failure = %#v, want safety or ownership refusal", failure)
				}
				if fixture.compose.removes != 0 || fixture.lima.deletes != 0 {
					t.Fatalf("drift reached mutation: Compose removes=%d Lima deletes=%d", fixture.compose.removes, fixture.lima.deletes)
				}
				fixture.assertSiblingUnchanged(t)
				if test.assert != nil {
					test.assert(t, fixture)
				}
			})
		}
	})

	t.Run("interruption requires a fresh preview and converges", func(t *testing.T) {
		fixture := newFullRemovalFixture(t)
		preview, failure := fixture.attempt(OperationInput{}, nil)
		if failure == nil || failure.Code != contract.CodeCleanupRefused {
			t.Fatalf("preview failure = %#v, want cleanup refusal", failure)
		}
		fixture.lima.deleteThenFail = true
		failure = fixture.authorizedAttempt(preview, nil)
		if failure == nil || failure.Code != contract.CodeInfrastructureApplyFailed {
			t.Fatalf("interrupted removal failure = %#v, want infrastructure failure", failure)
		}
		if fixture.compose.removes != 1 || fixture.lima.deletes != 1 || fixture.compose.present() || fixture.lima.present() {
			t.Fatalf("partial cleanup state: Compose=%d/%t Lima=%d/%t", fixture.compose.removes, fixture.compose.present(), fixture.lima.deletes, fixture.lima.present())
		}
		if _, err := os.Stat(fixture.facts.WorktreePath); err != nil {
			t.Fatalf("later worktree was touched by interruption: %v", err)
		}

		fresh, failure := fixture.attempt(OperationInput{}, nil)
		if failure == nil || failure.Code != contract.CodeCleanupRefused {
			t.Fatalf("fresh preview failure = %#v, want cleanup refusal", failure)
		}
		if fresh.Digest == preview.Digest {
			t.Fatal("partial cleanup did not invalidate the original preview digest")
		}
		fixture.lima.deleteThenFail = false
		failure = fixture.authorizedAttempt(fresh, nil)
		if failure != nil {
			t.Fatalf("retry removal failure = %#v", failure)
		}
		if fixture.compose.removes != 1 || fixture.lima.deletes != 1 {
			t.Fatalf("retry replayed an absent destructive step: Compose=%d Lima=%d", fixture.compose.removes, fixture.lima.deletes)
		}
		fixture.assertSelectedResourcesRemoved(t)
		fixture.assertSiblingUnchanged(t)
	})
}

type fullRemovalFixture struct {
	t                    *testing.T
	root                 string
	provider             *paths.Provider
	store                *state.Store
	journal              *journal.Journal
	workspaceID          identity.WorkspaceID
	boxID                identity.BoxID
	manifestPath         string
	plan                 contract.Plan
	facts                repository.Facts
	gitStore             gitstore.Record
	gitClient            machinegit.Client
	snapshot             state.RemovalSnapshot
	compose              *fixtureComposeRuntime
	lima                 *fixtureLimaRuntime
	siblingPath          string
	siblingContents      string
	sourceMarkerPath     string
	sourceMarkerContents string
	displacedWorktree    string
	attempts             int
}

func newFullRemovalFixture(t *testing.T) *fullRemovalFixture {
	t.Helper()
	gitPath := os.Getenv("HERMES_CRUSHER_REAL_GIT")
	if gitPath == "" {
		var err error
		gitPath, err = exec.LookPath("git")
		if err != nil {
			t.Skip("real Git is unavailable")
		}
	}
	gitPath, err := filepath.Abs(gitPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &fullRemovalFixture{t: t, root: t.TempDir(), siblingContents: "sibling must survive\n", sourceMarkerContents: "source must survive\n"}
	fixture.provider, err = paths.New(filepath.Join(fixture.root, "application"), filepath.Join(fixture.root, "cache"), filepath.Join(fixture.root, "logs"))
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(fixture.root, "source repository")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.gitPathCommand(gitPath, fixture.root, "init", "--initial-branch=main", source)
	fixture.write(filepath.Join(source, "compose.yaml"), "services:\n  workspace:\n    image: example.invalid/workspace:fixture\nvolumes:\n  data: {}\n")
	fixture.sourceMarkerPath = filepath.Join(source, "tracked.txt")
	fixture.write(fixture.sourceMarkerPath, fixture.sourceMarkerContents)
	fixture.gitPathCommand(gitPath, source, "add", "compose.yaml", "tracked.txt")
	fixture.gitPathCommand(gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	baseCommit := strings.TrimSpace(fixture.gitPathCommand(gitPath, source, "rev-parse", "HEAD"))

	fixture.manifestPath = filepath.Join(fixture.root, "workspace", ".abox", "box.yaml")
	fixture.write(fixture.manifestPath, "fixture manifest\n")
	fixture.workspaceID, err = identity.Workspace(fixture.manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.boxID, err = identity.Box(fixture.workspaceID, "REMOVE-FULL")
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := fixture.provider.WorkspacePath(fixture.boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	storePath, err := fixture.provider.GitStore(fixture.boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot, err := fixture.provider.WorkspaceRoot(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.facts = repository.Facts{
		Name: "repo", SourceRoot: source, Clean: true, BaseCommit: baseCommit,
		Branch: "abox/REMOVE-FULL/repo", WorktreePath: worktreePath, GitStorePath: storePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: source},
	}
	workspace := &manifest.Workspace{Spec: manifest.WorkspaceSpec{
		Repositories: []manifest.Repository{{
			Name: "repo", Source: manifest.Source{Local: source}, CanonicalLocal: source,
			Checkout: manifest.Checkout{Ref: baseCommit, Branch: fixture.facts.Branch}, WorkspacePath: "repo",
		}},
		Host:             manifest.Host{CPUMillicores: 1000, MemoryBytes: 1 << 30, DiskBytes: 8 << 30},
		Compose:          manifest.Compose{Includes: []manifest.ComposeInclude{{Repository: "repo", Path: "compose.yaml"}}},
		WorkspaceService: manifest.WorkspaceService{Name: "workspace", Repository: "repo", WorkingDirectory: "repo"},
		Mounts:           []manifest.Mount{{Repository: "repo", Services: []string{"workspace"}, Mode: "read-write"}},
		Policy:           manifest.Policy{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
	}}
	fixture.plan, _, err = plancompiler.Compile(plancompiler.Input{
		Workspace: workspace, WorkspaceID: fixture.workspaceID, BoxID: fixture.boxID, TaskKey: "REMOVE-FULL",
		ManifestPath: fixture.manifestPath, Repositories: []repository.Facts{fixture.facts}, Runtime: plancompiler.RuntimeFacts{Driver: "lima-vz"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: lima.SupportedVersion, Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(fixture.root, "controlled-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.gitClient = machinegit.Client{
		GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(home, "git-excludes"), Executor: process.Runner{},
	}
	if err := fixture.gitClient.Prepare(); err != nil {
		t.Fatal(err)
	}
	fixture.gitStore, err = (gitstore.Materializer{Git: fixture.gitClient}).Materialize(t.Context(), fixture.facts, fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	worktrees := worktree.Manager{Git: fixture.gitClient, WorkspaceRoot: workspaceRoot}
	if _, err := worktrees.Ensure(t.Context(), fixture.facts, fixture.plan, fixture.gitStore); err != nil {
		t.Fatal(err)
	}
	fixture.write(filepath.Join(fixture.facts.WorktreePath, "box-only.txt"), "unpublished\n")
	fixture.git(fixture.facts.WorktreePath, "add", "box-only.txt")
	fixture.git(fixture.facts.WorktreePath, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "Box-only commit")
	fixture.write(filepath.Join(fixture.facts.WorktreePath, "dirty.txt"), "discard dirty\n")
	fixture.write(filepath.Join(fixture.facts.GitStorePath, "info", "exclude"), "ignored-cache/\n")
	fixture.write(filepath.Join(fixture.facts.WorktreePath, "ignored-cache", "artifact.bin"), "discard ignored\n")

	boxRoot, err := fixture.provider.BoxRoot(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	planBytes, err := plancompiler.CanonicalJSON(fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	planPath, err := fixture.provider.Plan(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.writeBytes(planPath, planBytes)
	for _, constructor := range []func(identity.BoxID) (string, error){fixture.provider.Context, fixture.provider.LimaConfig} {
		path, pathErr := constructor(fixture.boxID)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		fixture.write(path, "generated\n")
	}
	compiled := fixture.writeComposeArtifacts()
	guestImage := filepath.Join(fixture.root, "tools", "guest.img")
	fixture.write(guestImage, "guest image\n")
	guestDigest := sha256.Sum256([]byte("guest image\n"))
	limaSpec := lima.Spec{
		RuntimeHost: fixture.plan.RuntimeHost, Mounts: fixture.plan.Mounts, BoxID: fixture.plan.BoxID,
		PlanDigest: fixture.plan.PlanDigest, Generation: 1,
		GuestImage: toolchain.Identity{Component: toolchain.GuestImage, Version: "fixture", Digest: contract.Digest(fmt.Sprintf("sha256:%x", guestDigest))},
		ImagePath:  guestImage,
	}
	limaSpec.PathEvidence, err = lima.RecordRuntimePaths(limaSpec)
	if err != nil {
		t.Fatal(err)
	}
	composeSpec := compose.RuntimeSpec{
		RuntimeHost: limaSpec, Plan: fixture.plan, Generation: 1, DockerPath: "/usr/bin/docker",
		Workdir: boxRoot, Compiled: compiled,
	}
	composeSpec.PathEvidence, err = compose.RecordRuntimePaths(fixture.plan, compiled)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{
		ownership.ComposeBoxIDFact: fixture.boxID.String(), "com.docker.compose.project": fixture.plan.Compose.ProjectName,
		"dev.abox.plan": string(fixture.plan.PlanDigest), "dev.abox.generation": "1",
	}
	fixture.compose = &fixtureComposeRuntime{observation: compose.RuntimeObservation{
		ProjectName: fixture.plan.Compose.ProjectName,
		Containers:  []compose.ContainerObservation{{ID: "selected-container", Name: "workspace", Labels: maps.Clone(labels)}},
		Networks:    []compose.NetworkObservation{{ID: "selected-network", Name: "default", Labels: maps.Clone(labels)}},
		Volumes:     []compose.VolumeObservation{{ID: "selected-volume", Name: "data", Driver: "local", Labels: maps.Clone(labels)}},
	}}
	limaMetadata := map[string]string{
		ownership.BoxIDFact: fixture.boxID.String(), "dev.abox.plan-digest": string(fixture.plan.PlanDigest), "dev.abox.generation": "1",
	}
	fixture.lima = &fixtureLimaRuntime{observation: lima.Observation{
		Name: fixture.plan.RuntimeHost.InstanceName, State: lima.Running, LimaStatus: "Running", LimaVersion: lima.SupportedVersion, Metadata: maps.Clone(limaMetadata),
	}}
	boxRootIdentity, err := ownership.InspectPath(boxRoot)
	if err != nil {
		t.Fatal(err)
	}
	limaArtifactDigest := sha256.Sum256([]byte("fixture limactl"))
	fixture.snapshot = state.RemovalSnapshot{
		BoxID: fixture.boxID.String(), Generation: 1, PlanDigest: fixture.plan.PlanDigest,
		BoxRoot:      generatedstate.Record{BoxID: fixture.boxID.String(), Root: ownership.PathRecord{CanonicalPath: boxRoot, Identity: boxRootIdentity}},
		Repositories: []state.RepositoryRemovalRecord{{Facts: fixture.facts, Store: fixture.gitStore}},
		LimaArtifact: toolchain.Identity{Component: toolchain.Lima, Version: lima.SupportedVersion, Digest: contract.Digest(fmt.Sprintf("sha256:%x", limaArtifactDigest))},
		LimaSpec:     limaSpec, Lima: ownership.LimaInstanceRecord{Name: fixture.plan.RuntimeHost.InstanceName, Metadata: limaMetadata},
		ComposeSpec: composeSpec, Compose: compose.RecordsFromObservation(fixture.compose.observation),
		LimaPathEvidence: limaSpec.PathEvidence, ComposePathEvidence: composeSpec.PathEvidence,
	}

	fixture.store, err = state.Open(t.Context(), filepath.Join(fixture.root, "control", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.store.Close(); err != nil {
			t.Error(err)
		}
	})
	fixture.seedCurrentGeneration()
	if err := fixture.store.SaveRemovalSnapshot(t.Context(), fixture.snapshot); err != nil {
		t.Fatal(err)
	}
	fixture.journal, err = journal.New(fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	fixture.siblingPath = filepath.Join(fixture.provider.BoxesRoot(), "unrelated-box", "keep.txt")
	fixture.write(fixture.siblingPath, fixture.siblingContents)
	return fixture
}

func (f *fullRemovalFixture) writeComposeArtifacts() compose.Result {
	f.t.Helper()
	rootPath, err := f.provider.ComposeRoot(f.boxID)
	if err != nil {
		f.t.Fatal(err)
	}
	overridePath, err := f.provider.ComposeOverride(f.boxID)
	if err != nil {
		f.t.Fatal(err)
	}
	resolvedPath, err := f.provider.ComposeResolved(f.boxID)
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(rootPath, "include: []\n")
	f.write(overridePath, "services: {}\n")
	model := []byte(`{"services":{"workspace":{"image":"example.invalid/workspace@sha256:fixture"}}}`)
	f.writeBytes(resolvedPath, append(append([]byte(nil), model...), '\n'))
	digest := sha256.Sum256(model)
	return compose.Result{
		ProjectName: f.plan.Compose.ProjectName, RootPath: rootPath, OverridePath: overridePath, ResolvedModelPath: resolvedPath,
		ResolvedModel: model, ResolvedModelDigest: contract.Digest(fmt.Sprintf("sha256:%x", digest)),
	}
}

func (f *fullRemovalFixture) seedCurrentGeneration() {
	f.t.Helper()
	err := f.store.Update(f.t.Context(), func(tx *sql.Tx) error {
		now := time.Now().UTC().UnixNano()
		if _, err := tx.Exec(`INSERT INTO workspaces (id, manifest_path, manifest_digest, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			f.workspaceID.String(), f.manifestPath, "sha256:manifest", now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO boxes (id, workspace_id, task_key, recorded_state, current_generation, created_at, updated_at) VALUES (?, ?, ?, 'ready', 1, ?, ?)`,
			f.boxID.String(), f.workspaceID.String(), "REMOVE-FULL", now, now); err != nil {
			return err
		}
		planPath, err := f.provider.Plan(f.boxID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO box_generations (box_id, generation, plan_digest, plan_path, recorded_state, created_at) VALUES (?, 1, ?, ?, 'ready', ?)`,
			f.boxID.String(), string(f.plan.PlanDigest), planPath, now)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fullRemovalFixture) attempt(input OperationInput, afterPreview func()) (Preview, *contract.Failure) {
	f.t.Helper()
	f.attempts++
	operationID := fmt.Sprintf("removal-attempt-%d", f.attempts)
	operation, created, err := f.journal.Begin(f.t.Context(), journal.BeginRequest{
		WorkspaceID: f.workspaceID, BoxID: f.boxID, TaskKey: "REMOVE-FULL", ManifestPath: f.manifestPath,
		ManifestDigest: "sha256:manifest", OperationID: operationID, IdempotencyKey: operationID,
		Kind: "remove", RequestDigest: "sha256:" + operationID, StartedAt: time.Now().UTC(),
	})
	if err != nil || !created {
		f.t.Fatalf("begin removal operation: created=%t error=%v", created, err)
	}
	runner := OperationRunner{Assemble: func(ctx context.Context, operation journal.Operation) (*Coordinator, error) {
		return AssembleCurrent(ctx, AssemblyConfig{
			Snapshots: f.store, Journal: f.journal, Operation: operation,
			Owner:    journal.Owner{InstanceID: "daemon-fixture", ProcessStartID: "process-fixture", BootID: "boot-fixture"},
			Quiescer: fixtureQuiescer{}, Paths: f.provider, MachineGit: f.gitClient,
			LimaArtifact: f.snapshot.LimaArtifact, Lima: f.lima, Compose: f.compose,
			AppendEvent: func(ctx context.Context, operationID, kind string, details json.RawMessage, occurredAt time.Time) error {
				_, appendErr := f.journal.AppendEvent(ctx, operationID, kind, details, occurredAt)
				return appendErr
			},
		})
	}}
	raw, err := json.Marshal(input)
	if err != nil {
		f.t.Fatal(err)
	}
	var preview Preview
	mutated := false
	failure := runner.Run(f.t.Context(), operation, raw, func(kind string, details any) error {
		encoded, marshalErr := json.Marshal(details)
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := f.journal.AppendEvent(f.t.Context(), operation.ID, kind, encoded, time.Now().UTC()); appendErr != nil {
			return appendErr
		}
		if kind == "removal.preview" {
			var ok bool
			preview, ok = details.(Preview)
			if !ok {
				return errors.New("removal preview event has unexpected type")
			}
			if afterPreview != nil && !mutated {
				mutated = true
				afterPreview()
			}
		}
		return nil
	})
	finalState, errorCode := journal.StateSucceeded, ""
	if failure != nil {
		finalState, errorCode = journal.StateFailed, string(failure.Code)
	}
	if err := f.journal.Finish(f.t.Context(), operation.ID, finalState, errorCode, time.Now().UTC()); err != nil {
		f.t.Fatal(err)
	}
	return preview, failure
}

func (f *fullRemovalFixture) authorizedAttempt(preview Preview, afterPreview func()) *contract.Failure {
	f.t.Helper()
	_, failure := f.attempt(OperationInput{
		PreviewDigest: preview.Digest, Yes: true, DiscardWorktreeChanges: true,
		DiscardUnpublishedCommits: true, DeleteVolumes: true,
	}, afterPreview)
	return failure
}

func (f *fullRemovalFixture) assertExactLosses(t *testing.T, preview Preview) {
	t.Helper()
	commit := strings.TrimSpace(f.git(f.facts.WorktreePath, "rev-parse", "HEAD"))
	want := []Loss{
		{Kind: LossUnpublishedCommit, Value: commit},
		{Kind: LossVolume, Value: "selected-volume"},
		{Kind: LossWorktreeChange, Value: filepath.Join(f.facts.WorktreePath, "dirty.txt")},
		{Kind: LossWorktreeChange, Value: filepath.Join(f.facts.WorktreePath, "ignored-cache", "artifact.bin")},
	}
	slices.SortFunc(want, compareLoss)
	if !slices.Equal(preview.Losses, want) {
		t.Fatalf("exact removal losses = %#v, want %#v", preview.Losses, want)
	}
}

func (f *fullRemovalFixture) assertAllSelectedResourcesPresent(t *testing.T) {
	t.Helper()
	for _, path := range []string{f.facts.WorktreePath, f.facts.GitStorePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("selected resource %q changed during refusal: %v", path, err)
		}
	}
	assertFileContents(t, filepath.Join(f.facts.WorktreePath, "dirty.txt"), "discard dirty\n")
	assertFileContents(t, filepath.Join(f.facts.WorktreePath, "ignored-cache", "artifact.bin"), "discard ignored\n")
	if !f.compose.present() || !f.lima.present() || f.compose.removes != 0 || f.lima.deletes != 0 {
		t.Fatalf("runtime resources changed during refusal: Compose=%t/%d Lima=%t/%d", f.compose.present(), f.compose.removes, f.lima.present(), f.lima.deletes)
	}
}

func (f *fullRemovalFixture) assertSelectedResourcesRemoved(t *testing.T) {
	t.Helper()
	for _, path := range []string{f.facts.WorktreePath, f.facts.GitStorePath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("selected resource %q remains: %v", path, err)
		}
	}
	if f.compose.present() || f.lima.present() {
		t.Fatalf("runtime resources remain: Compose=%t Lima=%t", f.compose.present(), f.lima.present())
	}
	planPath, err := f.provider.Plan(f.boxID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("retained recovery Plan is unavailable: %v", err)
	}
	assertFileContents(t, f.sourceMarkerPath, f.sourceMarkerContents)
}

func (f *fullRemovalFixture) assertSiblingUnchanged(t *testing.T) {
	t.Helper()
	assertFileContents(t, f.siblingPath, f.siblingContents)
}

func (f *fullRemovalFixture) write(path, contents string) { f.writeBytes(path, []byte(contents)) }

func (f *fullRemovalFixture) writeBytes(path string, contents []byte) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fullRemovalFixture) git(directory string, arguments ...string) string {
	f.t.Helper()
	return f.gitPathCommand(f.gitClient.GitPath, directory, arguments...)
}

func (f *fullRemovalFixture) gitPathCommand(gitPath, directory string, arguments ...string) string {
	f.t.Helper()
	command := exec.Command(gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(f.root, "git-home"), "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("contents of %q = %q, %v, want %q", path, got, err, want)
	}
}

type fixtureComposeRuntime struct {
	observation compose.RuntimeObservation
	removes     int
}

func (r *fixtureComposeRuntime) Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	return cloneComposeObservation(r.observation), nil
}

func (r *fixtureComposeRuntime) Remove(_ context.Context, _ compose.RuntimeSpec, records compose.RuntimeRecords) (compose.RuntimeObservation, error) {
	if err := compose.VerifyRemovalRecords(records, r.observation); err != nil {
		return cloneComposeObservation(r.observation), err
	}
	r.removes++
	r.observation.Containers = nil
	r.observation.Networks = nil
	r.observation.Volumes = nil
	return cloneComposeObservation(r.observation), nil
}

func (r *fixtureComposeRuntime) present() bool {
	return len(r.observation.Containers)+len(r.observation.Networks)+len(r.observation.Volumes) != 0
}

func cloneComposeObservation(source compose.RuntimeObservation) compose.RuntimeObservation {
	cloned := source
	cloned.Containers = slices.Clone(source.Containers)
	for index := range cloned.Containers {
		cloned.Containers[index].Labels = maps.Clone(source.Containers[index].Labels)
	}
	cloned.Networks = slices.Clone(source.Networks)
	for index := range cloned.Networks {
		cloned.Networks[index].Labels = maps.Clone(source.Networks[index].Labels)
	}
	cloned.Volumes = slices.Clone(source.Volumes)
	for index := range cloned.Volumes {
		cloned.Volumes[index].Labels = maps.Clone(source.Volumes[index].Labels)
	}
	return cloned
}

type fixtureLimaRuntime struct {
	observation    lima.Observation
	deletes        int
	deleteThenFail bool
}

func (r *fixtureLimaRuntime) ObserveRemoval(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error) {
	return cloneLimaObservation(r.observation), nil
}

func (r *fixtureLimaRuntime) Delete(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error) {
	if r.observation.State == lima.Absent {
		return cloneLimaObservation(r.observation), nil
	}
	r.deletes++
	r.observation.State = lima.Absent
	r.observation.LimaStatus = ""
	r.observation.Metadata = nil
	if r.deleteThenFail {
		return cloneLimaObservation(r.observation), errors.New("injected interruption after Lima delete")
	}
	return cloneLimaObservation(r.observation), nil
}

func (r *fixtureLimaRuntime) present() bool { return r.observation.State != lima.Absent }

func cloneLimaObservation(source lima.Observation) lima.Observation {
	source.Metadata = maps.Clone(source.Metadata)
	return source
}

type fixtureQuiescer struct{}

func (fixtureQuiescer) Quiesce(context.Context, Lease) error { return nil }
func (fixtureQuiescer) Resume(context.Context, Lease) error  { return nil }
