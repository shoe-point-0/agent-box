package generatedstate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestManagerRemovesOnlySelectedTransientStateAndRetainsRecoveryEvidence(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	unrelated := filepath.Join(fixture.provider.BoxesRoot(), "unrelated-box", "keep.txt")
	mustWrite(t, unrelated, []byte("keep"))
	diagnostics, err := fixture.provider.DiagnosticsRoot(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(diagnostics, "removal.log"), []byte("diagnostic"))
	atomicTemporary := filepath.Join(fixture.record.Root.CanonicalPath, atomicfile.TemporaryPrefix+"interrupted")
	mustWrite(t, atomicTemporary, []byte("incomplete"))

	manager := newManager(t, fixture.provider, generatedstate.SystemOperations())
	observed, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Present || len(observed.Artifacts) != 8 {
		t.Fatalf("observation = %#v", observed)
	}
	if err := manager.Remove(t.Context(), fixture.plan, fixture.record, observed); err != nil {
		t.Fatal(err)
	}
	terminal, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Present || len(terminal.Artifacts) != 0 {
		t.Fatalf("terminal observation = %#v", terminal)
	}
	assertContents(t, fixture.planPath, fixture.planBytes)
	assertContents(t, filepath.Join(diagnostics, "removal.log"), []byte("diagnostic"))
	assertContents(t, unrelated, []byte("keep"))
	if _, err := os.Lstat(atomicTemporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("atomic temporary remains: %v", err)
	}
}

func TestManagerRefusesArtifactReplacementWithoutTouchingReplacement(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	manager := newManager(t, fixture.provider, generatedstate.SystemOperations())
	observed, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil {
		t.Fatal(err)
	}
	contextPath, err := fixture.provider.Context(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	displaced := filepath.Join(fixture.provider.ApplicationRoot(), "displaced-context")
	if err := os.Rename(contextPath, displaced); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("replacement")
	mustWrite(t, contextPath, replacement)

	if err := manager.Remove(t.Context(), fixture.plan, fixture.record, observed); err == nil {
		t.Fatal("Remove() accepted replacement inode")
	}
	assertContents(t, contextPath, replacement)
}

func TestManagerFreshObservationConvergesAfterInterruptedSuccessfulDelete(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	injected := errors.New("injected interruption after delete")
	calls := 0
	operations := generatedstate.SystemOperations()
	systemRemove := operations.Remove
	operations.Remove = func(proof ownership.PathProof) error {
		calls++
		if err := systemRemove(proof); err != nil {
			return err
		}
		if calls == 1 {
			return injected
		}
		return nil
	}
	manager := newManager(t, fixture.provider, operations)
	observed, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(t.Context(), fixture.plan, fixture.record, observed); !errors.Is(err, injected) {
		t.Fatalf("Remove() error = %v, want injected interruption", err)
	}
	fresh, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Artifacts) != len(observed.Artifacts)-1 {
		t.Fatalf("fresh artifacts = %d, want %d", len(fresh.Artifacts), len(observed.Artifacts)-1)
	}
	if err := manager.Remove(t.Context(), fixture.plan, fixture.record, fresh); err != nil {
		t.Fatal(err)
	}
	terminal, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record)
	if err != nil || terminal.Present {
		t.Fatalf("terminal observation = %#v, %v", terminal, err)
	}
}

func TestManagerRefusesNonemptyResourceDirectory(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	workspace, err := fixture.provider.WorkspaceRoot(fixture.boxID)
	if err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(workspace, "unexpected.txt")
	mustWrite(t, canary, []byte("keep"))
	manager := newManager(t, fixture.provider, generatedstate.SystemOperations())
	if _, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record); err == nil {
		t.Fatal("ObserveRemoval() accepted nonempty workspace root")
	}
	assertContents(t, canary, []byte("keep"))
}

func TestManagerRefusesUnrecognizedBoxRootEntry(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	canary := filepath.Join(fixture.record.Root.CanonicalPath, "user-created.txt")
	mustWrite(t, canary, []byte("keep"))
	manager := newManager(t, fixture.provider, generatedstate.SystemOperations())
	if _, err := manager.ObserveRemoval(t.Context(), fixture.plan, fixture.record); err == nil {
		t.Fatal("ObserveRemoval() accepted unrecognized Box-root entry")
	}
	assertContents(t, canary, []byte("keep"))
}

type fixture struct {
	provider  *paths.Provider
	boxID     identity.BoxID
	plan      contract.Plan
	planPath  string
	planBytes []byte
	record    generatedstate.Record
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustWrite(t, filepath.Join(source, "compose.yaml"), []byte("services: {}\n"))
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
	mustWrite(t, manifestPath, []byte("fixture manifest\n"))
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "REMOVE-STATE")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := paths.New(filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := provider.WorkspacePath(boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	storePath, err := provider.GitStore(boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repo", SourceRoot: source, Clean: true,
		BaseCommit: "1111111111111111111111111111111111111111", Branch: "abox/remove-state/repo",
		WorktreePath: worktreePath, GitStorePath: storePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: source},
	}
	workspace := &manifest.Workspace{Spec: manifest.WorkspaceSpec{
		Repositories: []manifest.Repository{{
			Name: "repo", Source: manifest.Source{Local: source}, CanonicalLocal: source,
			Checkout: manifest.Checkout{Ref: facts.BaseCommit, Branch: facts.Branch}, WorkspacePath: "repo",
		}},
		Host:             manifest.Host{CPUMillicores: 1000, MemoryBytes: 1 << 30, DiskBytes: 8 << 30},
		Compose:          manifest.Compose{Includes: []manifest.ComposeInclude{{Repository: "repo", Path: "compose.yaml"}}},
		WorkspaceService: manifest.WorkspaceService{Name: "workspace", Repository: "repo", WorkingDirectory: "repo"},
		Mounts:           []manifest.Mount{{Repository: "repo", Services: []string{"workspace"}, Mode: "read-write"}},
		Policy:           manifest.Policy{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
	}}
	compiled, encoded, err := plan.Compile(plan.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "REMOVE-STATE",
		ManifestPath: manifestPath, Repositories: []repository.Facts{facts}, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: "test", Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	boxRoot, err := provider.BoxRoot(boxID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(boxRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	planPath, err := provider.Plan(boxID)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, planPath, encoded)
	for _, constructor := range []func(identity.BoxID) (string, error){
		provider.Context, provider.LimaConfig, provider.ComposeRoot, provider.ComposeOverride, provider.ComposeResolved,
	} {
		path, pathErr := constructor(boxID)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		mustWrite(t, path, []byte("generated"))
	}
	for _, constructor := range []func(identity.BoxID) (string, error){provider.GitRoot, provider.WorkspaceRoot} {
		path, pathErr := constructor(boxID)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rootIdentity, err := ownership.InspectPath(boxRoot)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		provider: provider, boxID: boxID, plan: compiled, planPath: planPath, planBytes: encoded,
		record: generatedstate.Record{BoxID: boxID.String(), Root: ownership.PathRecord{CanonicalPath: boxRoot, Identity: rootIdentity}},
	}
}

func newManager(t *testing.T, provider *paths.Provider, operations generatedstate.Operations) *generatedstate.Manager {
	t.Helper()
	manager, err := generatedstate.NewManager(provider, operations)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func mustWrite(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("contents of %s = %q, want %q", filepath.Base(path), got, want)
	}
}
