package up

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func TestReconcileGitRealGitResumesOnlyRetainedIdentities(t *testing.T) {
	fixture := newGitReconcileFixture(t)
	authority := &memoryGitAuthority{}
	stores := &checkingMaterializer{
		Materializer: gitstore.Materializer{Git: fixture.client}, authority: authority,
	}
	worktrees := &interruptingWorktrees{Manager: worktree.Manager{
		Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot,
	}, failEnsure: true}
	config := GitConfig{Authority: authority, Stores: stores, Worktrees: worktrees, Now: monotonicNow()}

	_, err := ReconcileGit(t.Context(), config, fixture.attempt("operation-one"))
	if err == nil || !strings.Contains(err.Error(), "injected interruption") {
		t.Fatalf("first reconciliation error = %v", err)
	}
	if stores.calls != 1 || authority.value.Store == nil || authority.value.Worktree != nil {
		t.Fatalf("partial authority after interruption = %#v, materializations=%d", authority.value, stores.calls)
	}
	storeIdentity := authority.value.Store.Identity

	worktrees.failEnsure = false
	second, err := ReconcileGit(t.Context(), config, fixture.attempt("operation-two"))
	if err != nil {
		t.Fatal(err)
	}
	if stores.calls != 1 || worktrees.ensures != 2 || worktrees.observes != 0 {
		t.Fatalf("restart calls materialize=%d ensure=%d observe=%d", stores.calls, worktrees.ensures, worktrees.observes)
	}
	if second.Store.Identity != storeIdentity || second.Worktree.WorktreeIdentity == (second.Store.Identity) || authority.value.OperationID != "operation-two" {
		t.Fatalf("restart identities = result %#v authority %#v", second, authority.value)
	}

	third, err := ReconcileGit(t.Context(), config, fixture.attempt("operation-three"))
	if err != nil {
		t.Fatal(err)
	}
	if stores.calls != 1 || worktrees.ensures != 2 || worktrees.observes != 1 || !reflect.DeepEqual(third, second) {
		t.Fatalf("warm Git reconciliation mutated: result=%#v calls=%d/%d/%d", third, stores.calls, worktrees.ensures, worktrees.observes)
	}
}

func TestReconcileGitRefusesReplacementInsteadOfAdoptingPlannedPath(t *testing.T) {
	fixture := newGitReconcileFixture(t)
	authority := &memoryGitAuthority{}
	stores := &checkingMaterializer{Materializer: gitstore.Materializer{Git: fixture.client}, authority: authority}
	worktrees := &interruptingWorktrees{Manager: worktree.Manager{
		Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot,
	}, failEnsure: true}
	config := GitConfig{Authority: authority, Stores: stores, Worktrees: worktrees, Now: monotonicNow()}
	if _, err := ReconcileGit(t.Context(), config, fixture.attempt("operation-one")); err == nil {
		t.Fatal("first reconciliation unexpectedly succeeded")
	}

	moved := fixture.facts.GitStorePath + "-owned"
	if err := os.Rename(fixture.facts.GitStorePath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.facts.GitStorePath, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(fixture.facts.GitStorePath, "foreign")
	if err := os.WriteFile(marker, []byte("preserve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktrees.failEnsure = false
	if _, err := ReconcileGit(t.Context(), config, fixture.attempt("operation-two")); err == nil {
		t.Fatal("reconciliation adopted a replacement at the deterministic path")
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "preserve\n" {
		t.Fatalf("replacement changed: content=%q error=%v", content, err)
	}
}

func TestReconcileGitRefusesAuthorityDriftBeforeExternalMutation(t *testing.T) {
	fixture := newGitReconcileFixture(t)
	authority := &memoryGitAuthority{found: true, value: state.ReconciliationGitAuthority{
		BoxID: fixture.plan.BoxID, Generation: 1, Repository: fixture.facts,
		PlanDigest: fixture.plan.PlanDigest, OperationID: "old-operation",
	}}
	authority.value.Repository.Branch = "abox/foreign"
	stores := &checkingMaterializer{Materializer: gitstore.Materializer{Git: fixture.client}, authority: authority}
	worktrees := &interruptingWorktrees{Manager: worktree.Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}}

	_, err := ReconcileGit(t.Context(), GitConfig{
		Authority: authority, Stores: stores, Worktrees: worktrees,
	}, fixture.attempt("new-operation"))
	if err == nil || !strings.Contains(err.Error(), "differs from prepared attempt") {
		t.Fatalf("drift error = %v", err)
	}
	if stores.calls != 0 || worktrees.ensures != 0 || worktrees.observes != 0 {
		t.Fatal("authority drift reached an external mutation boundary")
	}
}

type memoryGitAuthority struct {
	value state.ReconciliationGitAuthority
	found bool
	saves int
}

func (s *memoryGitAuthority) ReconciliationGitAuthorityFor(_ context.Context, boxID string, generation int64, repositoryName string) (state.ReconciliationGitAuthority, bool, error) {
	if !s.found {
		return state.ReconciliationGitAuthority{}, false, nil
	}
	if s.value.BoxID != boxID || s.value.Generation != generation || s.value.Repository.Name != repositoryName {
		return state.ReconciliationGitAuthority{}, false, errors.New("unexpected authority lookup")
	}
	return s.value, true, nil
}

func (s *memoryGitAuthority) SaveReconciliationGitAuthority(_ context.Context, authority state.ReconciliationGitAuthority, _ time.Time) error {
	if s.found {
		existing, next := s.value, authority
		existing.OperationID, next.OperationID = "", ""
		existing.Store, existing.Worktree, next.Store, next.Worktree = nil, nil, nil, nil
		if !reflect.DeepEqual(existing, next) {
			return errors.New("authority identity drift")
		}
	}
	s.value, s.found = authority, true
	s.saves++
	return nil
}

type checkingMaterializer struct {
	gitstore.Materializer
	authority *memoryGitAuthority
	calls     int
}

func (m *checkingMaterializer) Materialize(ctx context.Context, facts repository.Facts, plan contract.Plan) (gitstore.Record, error) {
	m.calls++
	if !m.authority.found || m.authority.value.Store != nil {
		return gitstore.Record{}, errors.New("private-store mutation preceded durable intent")
	}
	return m.Materializer.Materialize(ctx, facts, plan)
}

type interruptingWorktrees struct {
	worktree.Manager
	failEnsure        bool
	ensures, observes int
}

func (w *interruptingWorktrees) Ensure(ctx context.Context, facts repository.Facts, plan contract.Plan, store gitstore.Record) (worktree.Observation, error) {
	w.ensures++
	if w.failEnsure {
		return worktree.Observation{}, errors.New("injected interruption after private-store persistence")
	}
	return w.Manager.Ensure(ctx, facts, plan, store)
}

func (w *interruptingWorktrees) Observe(ctx context.Context, facts repository.Facts, plan contract.Plan, store gitstore.Record) (worktree.Observation, error) {
	w.observes++
	return w.Manager.Observe(ctx, facts, plan, store)
}

type gitReconcileFixture struct {
	client        machinegit.Client
	facts         repository.Facts
	plan          contract.Plan
	workspaceRoot string
}

func newGitReconcileFixture(t *testing.T) gitReconcileFixture {
	return newGitReconcileFixtureWithDriver(t, "lima-qemu")
}

func newGitReconcileFixtureWithDriver(t *testing.T, driver string) gitReconcileFixture {
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
	root := t.TempDir()
	source := filepath.Join(root, "source with spaces")
	runFixtureGit(t, gitPath, root, "init", "--initial-branch=main", source)
	writeFixtureFile(t, filepath.Join(source, "compose.yaml"), "services: {}\n")
	writeFixtureFile(t, filepath.Join(source, "tracked.txt"), "content\n")
	runFixtureGit(t, gitPath, source, "add", "compose.yaml", "tracked.txt")
	runFixtureGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	base := strings.TrimSpace(runFixtureGit(t, gitPath, source, "rev-parse", "HEAD"))

	manifestDir := filepath.Join(root, "workspace", ".abox")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(manifestDir, "box.yaml")
	writeFixtureFile(t, manifestPath, "fixture\n")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-69")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := paths.New(filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot, err := provider.WorkspaceRoot(boxID)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := provider.WorkspacePath(boxID, "services/repo")
	if err != nil {
		t.Fatal(err)
	}
	storePath, err := provider.GitStore(boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repo", SourceRoot: source, Clean: true, BaseCommit: base, Branch: "abox/TASK-69/repo",
		WorktreePath: worktreePath, GitStorePath: storePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: source},
	}
	workspace := &manifest.Workspace{Spec: manifest.WorkspaceSpec{
		Repositories: []manifest.Repository{{
			Name: "repo", Source: manifest.Source{Local: source}, CanonicalLocal: source,
			Checkout: manifest.Checkout{Ref: base, Branch: facts.Branch}, WorkspacePath: "services/repo",
		}},
		Host:             manifest.Host{CPUMillicores: 1000, MemoryBytes: 1 << 30, DiskBytes: 8 << 30},
		Compose:          manifest.Compose{Includes: []manifest.ComposeInclude{{Repository: "repo", Path: "compose.yaml"}}},
		WorkspaceService: manifest.WorkspaceService{Name: "workspace", Repository: "repo", WorkingDirectory: "services/repo"},
		Mounts:           []manifest.Mount{{Repository: "repo", Services: []string{"workspace"}, Mode: "read-write"}},
		Policy:           manifest.Policy{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
	}}
	compiled, _, err := plancompiler.Compile(plancompiler.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-69", ManifestPath: manifestPath,
		Repositories: []repository.Facts{facts}, Runtime: plancompiler.RuntimeFacts{Driver: driver},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: "test", Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "controlled-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	client := machinegit.Client{
		GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(home, "git-excludes"), Executor: process.Runner{},
	}
	if err := client.Prepare(); err != nil {
		t.Fatal(err)
	}
	return gitReconcileFixture{client: client, facts: facts, plan: compiled, workspaceRoot: workspaceRoot}
}

func (f gitReconcileFixture) attempt(operationID string) PreparedAttempt {
	return PreparedAttempt{
		Operation: journal.Operation{ID: operationID, BoxID: f.plan.BoxID, Kind: "up", State: journal.StateRunning},
		Plan:      f.plan, Generation: 1, Repository: f.facts, WorkspaceRoot: f.workspaceRoot,
	}
}

func monotonicNow() func() time.Time {
	value := time.Unix(100, 0)
	return func() time.Time {
		value = value.Add(time.Second)
		return value
	}
}

func runFixtureGit(t *testing.T, gitPath, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
