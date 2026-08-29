package gitstore_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestMaterializeRealGitDetachedPrivateStore(t *testing.T) {
	fixture := newFixture(t, true)
	marker := filepath.Join(fixture.root, "hostile-source-config-executed")
	hook := filepath.Join(fixture.root, "hostile-hook")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n: > '"+marker+"'\nexit 93\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustGit(t, fixture.gitPath, fixture.source, "config", "core.hooksPath", fixture.root)
	mustGit(t, fixture.gitPath, fixture.source, "config", "core.fsmonitor", hook)
	mustGit(t, fixture.gitPath, fixture.source, "config", "core.sshCommand", hook)
	mustGit(t, fixture.gitPath, fixture.source, "config", "remote.origin.url", "ext::"+hook)
	beforeRefs := mustGit(t, fixture.gitPath, fixture.source, "for-each-ref", "--format=%(refname)%00%(objectname)")
	beforeObjects := objectFiles(t, filepath.Join(fixture.source, ".git", "objects"))

	record, err := (gitstore.Materializer{Git: fixture.client}).Materialize(context.Background(), fixture.facts, fixture.plan)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	wantRecord := gitstore.Record{
		Repository: fixture.facts.Name, GitCommonDir: fixture.facts.GitStorePath,
		Identity: record.Identity, BaseCommit: fixture.facts.BaseCommit, PublishDestination: fixture.facts.PublishDestination,
	}
	if record != wantRecord {
		t.Fatalf("record = %#v, want %#v", record, wantRecord)
	}
	if record.Identity == (ownership.FileIdentity{}) {
		t.Fatal("private store identity is empty")
	}
	assertAbsent(t, marker)
	if got := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "rev-parse", "refs/abox/base")); got != fixture.facts.BaseCommit {
		t.Fatalf("private base = %s, want %s", got, fixture.facts.BaseCommit)
	}
	if got := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "config", "--bool", "core.logAllRefUpdates")); got != "true" {
		t.Fatalf("core.logAllRefUpdates = %q, want true", got)
	}
	if output, err := rawGit(t, fixture.gitPath, fixture.facts.GitStorePath, "config", "--get", "remote.origin.url"); err == nil {
		t.Fatalf("private store retained origin URL %q", output)
	}
	if _, err := os.Lstat(filepath.Join(fixture.facts.GitStorePath, "objects", "info", "alternates")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private store alternates = %v, want absent", err)
	}
	if afterRefs := mustGit(t, fixture.gitPath, fixture.source, "for-each-ref", "--format=%(refname)%00%(objectname)"); afterRefs != beforeRefs {
		t.Fatalf("source refs changed\nbefore: %q\n after: %q", beforeRefs, afterRefs)
	}
	assertNoSharedObjectInodes(t, beforeObjects, objectFiles(t, filepath.Join(fixture.facts.GitStorePath, "objects")))

	unique := filepath.Join(fixture.root, "box-only-object")
	mustWrite(t, unique, "box-only-object-content\n")
	objectID := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "hash-object", "-w", unique))
	sourceObject := filepath.Join(fixture.source, ".git", "objects", objectID[:2], objectID[2:])
	if _, err := os.Lstat(sourceObject); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Box object appeared in source object database: %v", err)
	}
	mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "config", "remote.origin.url", "/attacker/changed/destination")
	if record.PublishDestination != fixture.facts.PublishDestination {
		t.Fatal("trusted publish destination changed with Box Git config")
	}
}

func TestMaterializeRefusesDirtySourceBeforeStateCreation(t *testing.T) {
	fixture := newFixture(t, false)
	mustWrite(t, filepath.Join(fixture.source, "untracked secret.txt"), "secret\n")

	record, err := (gitstore.Materializer{Git: fixture.client}).Materialize(context.Background(), fixture.facts, fixture.plan)
	if record != (gitstore.Record{}) {
		t.Fatalf("record = %#v, want zero", record)
	}
	var dirty *repository.DirtySourceError
	if !errors.As(err, &dirty) || dirty.ErrorCode() != contract.CodeGitDirtySource || dirty.ExitCode() != contract.ExitSafety {
		t.Fatalf("Materialize() error = %T %v, want ABOX_GIT_DIRTY_SOURCE/7", err, err)
	}
	if !reflect.DeepEqual(dirty.ChangedPaths(), []string{"untracked secret.txt"}) {
		t.Fatalf("changed paths = %q", dirty.ChangedPaths())
	}
	assertAbsent(t, fixture.applicationRoot)
}

func TestMaterializeRefusesGeneratedBranchCollisionBeforeStateCreation(t *testing.T) {
	fixture := newFixture(t, false)
	mustGit(t, fixture.gitPath, fixture.source, "branch", fixture.facts.Branch)

	_, err := (gitstore.Materializer{Git: fixture.client}).Materialize(context.Background(), fixture.facts, fixture.plan)
	var conflict *gitstore.BranchConflictError
	if !errors.As(err, &conflict) || conflict.ErrorCode() != contract.CodeBranchConflict || conflict.ExitCode() != contract.ExitConflict {
		t.Fatalf("Materialize() error = %T %v, want branch conflict/4", err, err)
	}
	assertAbsent(t, fixture.applicationRoot)
}

func TestMaterializeRefusesExistingStoreWithoutChangingIt(t *testing.T) {
	fixture := newFixture(t, false)
	mustMkdir(t, fixture.facts.GitStorePath)
	marker := filepath.Join(fixture.facts.GitStorePath, "foreign-marker")
	mustWrite(t, marker, "do not remove\n")

	_, err := (gitstore.Materializer{Git: fixture.client}).Materialize(context.Background(), fixture.facts, fixture.plan)
	var conflict *gitstore.StoreConflictError
	if !errors.As(err, &conflict) || conflict.ExitCode() != contract.ExitConflict {
		t.Fatalf("Materialize() error = %T %v, want store conflict", err, err)
	}
	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "do not remove\n" {
		t.Fatalf("pre-existing store changed: content=%q error=%v", got, readErr)
	}
}

func TestMaterializeCleansOwnedStoreAfterApplyFailure(t *testing.T) {
	fixture := newFixture(t, false)
	adapter := failingConfigureGit{Client: fixture.client}

	_, err := (gitstore.Materializer{Git: adapter}).Materialize(context.Background(), fixture.facts, fixture.plan)
	if err == nil || !strings.Contains(err.Error(), "injected configure failure") {
		t.Fatalf("Materialize() error = %v, want injected failure", err)
	}
	assertAbsent(t, fixture.applicationRoot)
}

func TestMaterializeRejectsMutatedPlanBeforeGitOrFilesystem(t *testing.T) {
	fixture := newFixture(t, false)
	fixture.plan.Repositories[0].BaseCommit = strings.Repeat("0", 40)
	recorder := &countingGit{}

	_, err := (gitstore.Materializer{Git: recorder}).Materialize(context.Background(), fixture.facts, fixture.plan)
	if err == nil || recorder.calls != 0 {
		t.Fatalf("Materialize(mutated Plan) error = %v, Git calls = %d", err, recorder.calls)
	}
	assertAbsent(t, fixture.applicationRoot)
}

type testFixture struct {
	root, source, applicationRoot, gitPath string
	client                                 machinegit.Client
	facts                                  repository.Facts
	plan                                   contract.Plan
}

func newFixture(t *testing.T, detached bool) testFixture {
	t.Helper()
	gitPath := requireGit(t)
	root := t.TempDir()
	source := filepath.Join(root, "source with spaces 界")
	mustGit(t, gitPath, root, "init", "--initial-branch=main", source)
	mustWrite(t, filepath.Join(source, "compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(source, "tracked.txt"), "initial\n")
	mustGit(t, gitPath, source, "add", "compose.yaml", "tracked.txt")
	mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	if detached {
		mustGit(t, gitPath, source, "checkout", "--detach")
		mustWrite(t, filepath.Join(source, "tracked.txt"), "detached commit\n")
		mustGit(t, gitPath, source, "add", "tracked.txt")
		mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "detached")
	}
	baseCommit := strings.TrimSpace(mustGit(t, gitPath, source, "rev-parse", "HEAD"))

	manifestDir := filepath.Join(root, "workspace", ".abox")
	mustMkdir(t, manifestDir)
	manifestPath := filepath.Join(manifestDir, "box.yaml")
	mustWrite(t, manifestPath, "fixture manifest\n")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-19")
	if err != nil {
		t.Fatal(err)
	}
	applicationRoot := filepath.Join(root, "state with spaces 界")
	provider, err := paths.New(applicationRoot, filepath.Join(root, "cache"), filepath.Join(root, "logs"))
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
		Name: "repo", SourceRoot: source, Clean: true, BaseCommit: baseCommit,
		Branch: "abox/TASK-19/repo", WorktreePath: worktreePath, GitStorePath: storePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: source},
	}
	workspace := &manifest.Workspace{Spec: manifest.WorkspaceSpec{
		Repositories: []manifest.Repository{{
			Name: "repo", Source: manifest.Source{Local: source}, CanonicalLocal: source,
			Checkout: manifest.Checkout{Ref: baseCommit, Branch: facts.Branch}, WorkspacePath: "services/repo",
		}},
		Host:             manifest.Host{CPUMillicores: 1000, MemoryBytes: 1 << 30, DiskBytes: 8 << 30},
		Compose:          manifest.Compose{Includes: []manifest.ComposeInclude{{Repository: "repo", Path: "compose.yaml"}}},
		WorkspaceService: manifest.WorkspaceService{Name: "workspace", Repository: "repo", WorkingDirectory: "services/repo"},
		Mounts:           []manifest.Mount{{Repository: "repo", Services: []string{"workspace"}, Mode: "read-write"}},
		Policy:           manifest.Policy{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
	}}
	compiled, _, err := plan.Compile(plan.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-19",
		ManifestPath: manifestPath, Repositories: []repository.Facts{facts}, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: "test", Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatalf("compile fixture Plan: %v", err)
	}
	home := filepath.Join(root, "controlled-home")
	mustMkdir(t, home)
	client := machinegit.Client{GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", Executor: process.Runner{}}
	return testFixture{root: root, source: source, applicationRoot: applicationRoot, gitPath: gitPath, client: client, facts: facts, plan: compiled}
}

type failingConfigureGit struct {
	machinegit.Client
}

func (f failingConfigureGit) ConfigurePrivateStore(context.Context, string) error {
	return errors.New("injected configure failure")
}

type countingGit struct {
	calls int
}

func (g *countingGit) called() (machinegit.Result, error) {
	g.calls++
	return machinegit.Result{}, errors.New("unexpected Git call")
}
func (g *countingGit) Status(context.Context, string) (machinegit.Result, error) { return g.called() }
func (g *countingGit) ResolveCommit(context.Context, string, string) (machinegit.Result, error) {
	return g.called()
}
func (g *countingGit) CheckBranch(context.Context, string, string) (machinegit.Result, error) {
	return g.called()
}
func (g *countingGit) LookupBranch(context.Context, string, string) (machinegit.Result, error) {
	return g.called()
}
func (g *countingGit) CloneBare(context.Context, string, string, string) (machinegit.Result, error) {
	return g.called()
}
func (g *countingGit) SetPrivateBase(context.Context, string, string) (machinegit.Result, error) {
	return g.called()
}
func (g *countingGit) ConfigurePrivateStore(context.Context, string) error {
	_, err := g.called()
	return err
}

func requireGit(t *testing.T) string {
	t.Helper()
	path := os.Getenv("HERMES_CRUSHER_REAL_GIT")
	if path == "" {
		var err error
		path, err = exec.LookPath("git")
		if err != nil {
			t.Skip("real Git is unavailable")
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(absolute)
}

func mustGit(t *testing.T, gitPath, directory string, arguments ...string) string {
	t.Helper()
	output, err := rawGit(t, gitPath, directory, arguments...)
	if err != nil {
		t.Fatalf("git %q failed: %v\n%s", arguments, err, output)
	}
	return output
}

func rawGit(t *testing.T, gitPath, directory string, arguments ...string) (string, error) {
	t.Helper()
	command := exec.Command(gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1",
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func objectFiles(t *testing.T, root string) map[string]fs.FileInfo {
	t.Helper()
	objects := make(map[string]fs.FileInfo)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		objects[relative] = info
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return objects
}

func assertNoSharedObjectInodes(t *testing.T, source, store map[string]fs.FileInfo) {
	t.Helper()
	common := 0
	for relative, sourceInfo := range source {
		storeInfo, exists := store[relative]
		if !exists {
			continue
		}
		common++
		if os.SameFile(sourceInfo, storeInfo) {
			t.Fatalf("source and private store share object inode %s", relative)
		}
	}
	if common == 0 {
		t.Fatal("no common source/store object files were available for inode proof")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q exists or cannot be inspected: %v", path, err)
	}
}
