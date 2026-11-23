package worktree

import (
	"context"
	"errors"
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
	"github.com/shoe-point-0/agent-box/internal/testutil"
)

func TestEnsureRealGitLifecycleObservationAndNoOp(t *testing.T) {
	fixture := newFixture(t, false)
	adapter := &countingGit{Client: fixture.client}
	manager := Manager{Git: adapter, WorkspaceRoot: fixture.workspaceRoot}

	first, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	if !first.Registered || first.Dirty || first.Branch != fixture.facts.Branch ||
		first.Commit != fixture.facts.BaseCommit || first.CanonicalPath != fixture.facts.WorktreePath ||
		first.CanonicalCommon != fixture.facts.GitStorePath || first.LockReason != "owned by Agent Box "+fixture.plan.BoxID ||
		first.WorktreeIdentity == (first.StoreIdentity) || first.WorktreeIdentity == (fixture.store.Identity) {
		t.Fatalf("first observation = %#v", first)
	}
	if adapter.adds != 1 || adapter.locks != 1 {
		t.Fatalf("mutation calls after create = add:%d lock:%d", adapter.adds, adapter.locks)
	}

	pointer, err := os.ReadFile(filepath.Join(fixture.facts.WorktreePath, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(pointer), "gitdir: "+fixture.facts.GitStorePath+string(filepath.Separator)) {
		t.Fatalf("worktree pointer is not invariant and absolute: %q", pointer)
	}
	machineStatus, err := fixture.client.Status(context.Background(), fixture.facts.WorktreePath)
	if err != nil || len(machineStatus.Stdout) != 0 {
		t.Fatalf("Machine status = %q, %v", machineStatus.Stdout, err)
	}
	if got := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "rev-parse", "--path-format=absolute", "--git-common-dir")); got != fixture.facts.GitStorePath {
		t.Fatalf("stock Git common dir = %q, want %q", got, fixture.facts.GitStorePath)
	}

	second, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}
	if !reflect.DeepEqual(second, first) || adapter.adds != 1 || adapter.locks != 1 {
		t.Fatalf("second ensure mutated or drifted: observation=%#v add:%d lock:%d", second, adapter.adds, adapter.locks)
	}

	mustWrite(t, filepath.Join(fixture.facts.WorktreePath, "box-commit.txt"), "box only\n")
	mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "add", "box-commit.txt")
	mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "box commit")
	boxCommit := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "rev-parse", "HEAD"))
	advanced, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err != nil || advanced.Commit != boxCommit || advanced.Commit == fixture.facts.BaseCommit {
		t.Fatalf("Ensure(advanced) = %#v, %v", advanced, err)
	}
	if sourceHead := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.source, "rev-parse", "HEAD")); sourceHead != fixture.facts.BaseCommit {
		t.Fatalf("source HEAD changed to %s", sourceHead)
	}

	dirtyName := "untracked\nfile.txt"
	mustWrite(t, filepath.Join(fixture.facts.WorktreePath, dirtyName), "dirty\n")
	mustWrite(t, filepath.Join(fixture.facts.GitStorePath, "info", "exclude"), "ignored-cache/\n")
	ignoredName := filepath.Join("ignored-cache", "artifact\n.bin")
	mustMkdir(t, filepath.Dir(filepath.Join(fixture.facts.WorktreePath, ignoredName)))
	mustWrite(t, filepath.Join(fixture.facts.WorktreePath, ignoredName), "ignored\n")
	dirty, err := manager.Observe(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err != nil || !dirty.Dirty || !reflect.DeepEqual(dirty.ChangedPaths, []string{dirtyName}) ||
		!reflect.DeepEqual(dirty.IgnoredPaths, []string{ignoredName}) {
		t.Fatalf("Observe(dirty and ignored) = %#v, %v", dirty, err)
	}
	if adapter.adds != 1 || adapter.locks != 1 {
		t.Fatal("read-only observation recreated or relocked the worktree")
	}
}

func TestRemoveRealGitDiscardsAuthorizedLockedWorktreeAndBranch(t *testing.T) {
	fixture := newFixture(t, false)
	manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}
	if _, err := manager.Ensure(t.Context(), fixture.facts, fixture.plan, fixture.store); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(fixture.facts.WorktreePath, "untracked.txt"), "discard\n")
	mustWrite(t, filepath.Join(fixture.facts.GitStorePath, "info", "exclude"), "ignored/\n")
	mustMkdir(t, filepath.Join(fixture.facts.WorktreePath, "ignored"))
	mustWrite(t, filepath.Join(fixture.facts.WorktreePath, "ignored", "cache.bin"), "discard\n")

	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Registered || !observed.BranchPresent || !observed.Dirty {
		t.Fatalf("removal observation = %#v", observed)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, fixture.store, observed); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, fixture.facts.WorktreePath)
	if _, err := fixture.client.LookupBranch(t.Context(), fixture.facts.GitStorePath, fixture.facts.Branch); !errors.Is(err, machinegit.ErrBranchNotFound) {
		t.Fatalf("generated branch remains: %v", err)
	}
	if _, err := os.Stat(fixture.facts.GitStorePath); err != nil {
		t.Fatalf("private store was removed: %v", err)
	}
	terminal, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, fixture.store)
	if err != nil || terminal.Registered || terminal.BranchPresent {
		t.Fatalf("terminal removal observation = %#v, %v", terminal, err)
	}
}

func TestRemoveRealGitRetryDeletesBranchLeftByInterruption(t *testing.T) {
	fixture := newFixture(t, false)
	adapter := &failingDeleteBranchGit{Client: fixture.client}
	manager := Manager{Git: adapter, WorkspaceRoot: fixture.workspaceRoot}
	if _, err := manager.Ensure(t.Context(), fixture.facts, fixture.plan, fixture.store); err != nil {
		t.Fatal(err)
	}
	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, fixture.store, observed); err == nil || !strings.Contains(err.Error(), "injected branch deletion failure") {
		t.Fatalf("first Remove() error = %v", err)
	}
	assertAbsent(t, fixture.facts.WorktreePath)

	remainder, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	if remainder.Registered || !remainder.BranchPresent || remainder.Commit != observed.Commit || remainder.WorktreeIdentity != (ownership.FileIdentity{}) {
		t.Fatalf("interrupted removal observation = %#v", remainder)
	}
	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, fixture.store, remainder); err != nil {
		t.Fatal(err)
	}
	if adapter.deletes != 2 {
		t.Fatalf("branch delete calls = %d, want 2", adapter.deletes)
	}
	if _, err := adapter.LookupBranch(t.Context(), fixture.facts.GitStorePath, fixture.facts.Branch); !errors.Is(err, machinegit.ErrBranchNotFound) {
		t.Fatalf("generated branch remains after retry: %v", err)
	}
}

func TestRemoveRealGitRefusesReplacedRegisteredPath(t *testing.T) {
	fixture := newFixture(t, false)
	manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}
	if _, err := manager.Ensure(t.Context(), fixture.facts, fixture.plan, fixture.store); err != nil {
		t.Fatal(err)
	}
	observed, err := manager.ObserveRemoval(t.Context(), fixture.facts, fixture.plan, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	moved := fixture.facts.WorktreePath + "-moved"
	if err := os.Rename(fixture.facts.WorktreePath, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, fixture.facts.WorktreePath)
	marker := filepath.Join(fixture.facts.WorktreePath, "foreign.txt")
	mustWrite(t, marker, "preserve\n")

	if err := manager.Remove(t.Context(), fixture.facts, fixture.plan, fixture.store, observed); err == nil {
		t.Fatal("Remove() accepted a replaced registered path")
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "preserve\n" {
		t.Fatalf("replacement path changed: %q, %v", contents, err)
	}
	if _, err := fixture.client.LookupBranch(t.Context(), fixture.facts.GitStorePath, fixture.facts.Branch); err != nil {
		t.Fatalf("generated branch was removed: %v", err)
	}
}

func TestEnsureRealGitRefusesConflictsWithoutReset(t *testing.T) {
	t.Run("wrong lock", func(t *testing.T) {
		fixture := newFixture(t, false)
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}
		if _, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store); err != nil {
			t.Fatal(err)
		}
		mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "worktree", "unlock", "--", fixture.facts.WorktreePath)
		mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "worktree", "lock", "--reason", "owned by somebody else", "--", fixture.facts.WorktreePath)
		before := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "rev-parse", "HEAD"))

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *ConflictError
		if !errors.As(err, &conflict) || conflict.ExitCode() != contract.ExitConflict {
			t.Fatalf("Ensure(wrong lock) error = %T %v", err, err)
		}
		if after := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "rev-parse", "HEAD")); after != before {
			t.Fatalf("HEAD reset from %s to %s", before, after)
		}
	})

	t.Run("wrong branch", func(t *testing.T) {
		fixture := newFixture(t, false)
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}
		if _, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store); err != nil {
			t.Fatal(err)
		}
		mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "switch", "-c", "foreign/branch")

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Ensure(wrong branch) error = %T %v", err, err)
		}
		if got := strings.TrimSpace(mustGit(t, fixture.gitPath, fixture.facts.WorktreePath, "branch", "--show-current")); got != "foreign/branch" {
			t.Fatalf("branch changed to %q", got)
		}
	})

	t.Run("common directory pointer", func(t *testing.T) {
		fixture := newFixture(t, false)
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}
		if _, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store); err != nil {
			t.Fatal(err)
		}
		pointer := filepath.Join(fixture.facts.WorktreePath, ".git")
		mustWrite(t, pointer, "gitdir: /foreign/worktrees/repo\n")

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Ensure(common pointer) error = %T %v", err, err)
		}
		if got, readErr := os.ReadFile(pointer); readErr != nil || string(got) != "gitdir: /foreign/worktrees/repo\n" {
			t.Fatalf("drifted pointer changed: %q, %v", got, readErr)
		}
	})

	t.Run("unregistered path", func(t *testing.T) {
		fixture := newFixture(t, false)
		mustMkdir(t, fixture.facts.WorktreePath)
		marker := filepath.Join(fixture.facts.WorktreePath, "foreign")
		mustWrite(t, marker, "preserve\n")
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Ensure(unregistered) error = %T %v", err, err)
		}
		if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "preserve\n" {
			t.Fatalf("foreign path changed: %q, %v", got, readErr)
		}
	})

	t.Run("existing branch", func(t *testing.T) {
		fixture := newFixture(t, false)
		mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "branch", fixture.facts.Branch, "refs/abox/base")
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *BranchConflictError
		if !errors.As(err, &conflict) || conflict.ErrorCode() != contract.CodeBranchConflict {
			t.Fatalf("Ensure(existing branch) error = %T %v", err, err)
		}
		assertAbsent(t, fixture.facts.WorktreePath)
	})

	t.Run("store identity", func(t *testing.T) {
		fixture := newFixture(t, false)
		fixture.store.Identity.Inode++
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Ensure(store identity) error = %T %v", err, err)
		}
		assertAbsent(t, fixture.facts.WorktreePath)
	})
}

func TestEnsureRealGitRejectsUnsupportedRepositoryModes(t *testing.T) {
	t.Run("relative worktrees", func(t *testing.T) {
		fixture := newFixture(t, false)
		mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "config", "worktree.useRelativePaths", "true")
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var unsupported *RelativeWorktreesError
		if !errors.As(err, &unsupported) || unsupported.ErrorCode() != contract.CodeFeatureNotSupported {
			t.Fatalf("Ensure(relative) error = %T %v", err, err)
		}
		assertAbsent(t, fixture.facts.WorktreePath)
	})

	t.Run("active submodule", func(t *testing.T) {
		fixture := newFixture(t, true)
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
		var unsupported *ActiveSubmodulesError
		if !errors.As(err, &unsupported) || !reflect.DeepEqual(unsupported.Paths(), []string{"modules/sub"}) {
			t.Fatalf("Ensure(submodule) error = %T %v paths=%q", err, err, unsupported.Paths())
		}
		assertAbsent(t, fixture.facts.WorktreePath)
	})

	t.Run("checkout filter", func(t *testing.T) {
		fixture := newFixture(t, false)
		marker := filepath.Join(fixture.root, "filter-executed")
		script := filepath.Join(fixture.root, "filter")
		mustWrite(t, script, "#!/bin/sh\n: > '"+marker+"'\nexit 97\n")
		if err := os.Chmod(script, 0o700); err != nil {
			t.Fatal(err)
		}
		mustGit(t, fixture.gitPath, fixture.facts.GitStorePath, "config", "filter.evil.smudge", script)
		manager := Manager{Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot}

		if _, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store); !errors.Is(err, machinegit.ErrPolicy) {
			t.Fatalf("Ensure(filter) error = %T %v", err, err)
		}
		assertAbsent(t, marker)
		assertAbsent(t, fixture.facts.WorktreePath)
	})
}

func TestEnsureCleansExactWorktreeAfterLockFailure(t *testing.T) {
	fixture := newFixture(t, false)
	adapter := &failingLockGit{Client: fixture.client}
	manager := Manager{Git: adapter, WorkspaceRoot: fixture.workspaceRoot}

	_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err == nil || !strings.Contains(err.Error(), "injected lock failure") {
		t.Fatalf("Ensure() error = %v", err)
	}
	assertAbsent(t, fixture.facts.WorktreePath)
	if _, lookupErr := fixture.client.LookupBranch(context.Background(), fixture.facts.GitStorePath, fixture.facts.Branch); !errors.Is(lookupErr, machinegit.ErrBranchNotFound) {
		t.Fatalf("generated branch remains after cleanup: %v", lookupErr)
	}
	if _, statErr := os.Lstat(fixture.workspaceRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("empty workspace parents remain: %v", statErr)
	}
}

func TestEnsureCleansLockedWorktreeAfterVerificationFailure(t *testing.T) {
	fixture := newFixture(t, false)
	adapter := &failingObservationGit{Client: fixture.client}
	manager := Manager{Git: adapter, WorkspaceRoot: fixture.workspaceRoot}

	_, err := manager.Ensure(context.Background(), fixture.facts, fixture.plan, fixture.store)
	if err == nil || !strings.Contains(err.Error(), "verify creation") {
		t.Fatalf("Ensure() error = %v", err)
	}
	assertAbsent(t, fixture.facts.WorktreePath)
	if _, lookupErr := fixture.client.LookupBranch(context.Background(), fixture.facts.GitStorePath, fixture.facts.Branch); !errors.Is(lookupErr, machinegit.ErrBranchNotFound) {
		t.Fatalf("generated branch remains after cleanup: %v", lookupErr)
	}
	if _, statErr := os.Lstat(fixture.workspaceRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("empty workspace parents remain: %v", statErr)
	}
}

func TestParseRegistrationsUsesNULRecords(t *testing.T) {
	sha := strings.Repeat("a", 40)
	output := []byte("worktree /store.git\x00bare\x00\x00worktree /workspace/line\nname\x00HEAD " + sha + "\x00branch refs/heads/abox/test\x00locked owned by Agent Box box-id\x00future value\x00\x00")
	registrations, err := parseRegistrations(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(registrations) != 2 || registrations[1].Path != "/workspace/line\nname" ||
		registrations[1].Head != sha || registrations[1].Branch != "refs/heads/abox/test" ||
		registrations[1].LockReason != "owned by Agent Box box-id" {
		t.Fatalf("registrations = %#v", registrations)
	}
	for name, malformed := range map[string][]byte{
		"unterminated": []byte("worktree /store.git"),
		"relative":     []byte("worktree relative\x00bare\x00\x00"),
		"duplicate":    []byte("worktree /one\x00worktree /two\x00bare\x00\x00"),
		"bad head":     []byte("worktree /one\x00HEAD bad\x00\x00"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRegistrations(malformed); err == nil {
				t.Fatal("parseRegistrations() succeeded")
			}
		})
	}
}

type testFixture struct {
	root, source, workspaceRoot, gitPath string
	client                               machinegit.Client
	facts                                repository.Facts
	plan                                 contract.Plan
	store                                gitstore.Record
}

func newFixture(t *testing.T, gitlink bool) testFixture {
	t.Helper()
	gitPath := testutil.RequireGit(t)
	root := testutil.TempDir(t)
	source := filepath.Join(root, "source with spaces 界")
	mustGit(t, gitPath, root, "init", "--initial-branch=main", source)
	mustWrite(t, filepath.Join(source, "compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(source, "tracked.txt"), "initial\n")
	mustWrite(t, filepath.Join(source, ".gitattributes"), "*.filtered filter=evil\n")
	mustGit(t, gitPath, source, "add", "compose.yaml", "tracked.txt", ".gitattributes")
	mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	if gitlink {
		submodule := filepath.Join(source, "modules", "sub")
		mustMkdir(t, filepath.Dir(submodule))
		mustGit(t, gitPath, source, "init", "--initial-branch=main", submodule)
		mustWrite(t, filepath.Join(submodule, "sub.txt"), "submodule\n")
		mustGit(t, gitPath, submodule, "add", "sub.txt")
		mustGit(t, gitPath, submodule, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "submodule")
		commit := strings.TrimSpace(mustGit(t, gitPath, submodule, "rev-parse", "HEAD"))
		mustGit(t, gitPath, source, "update-index", "--add", "--cacheinfo", "160000,"+commit+",modules/sub")
		mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "gitlink")
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
	boxID, err := identity.Box(workspaceID, "TASK-20")
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
	workspaceRoot, err := provider.WorkspaceRoot(boxID)
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repo", SourceRoot: source, Clean: true, BaseCommit: baseCommit,
		Branch: "abox/TASK-20/repo", WorktreePath: worktreePath, GitStorePath: storePath,
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
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-20",
		ManifestPath: manifestPath, Repositories: []repository.Facts{facts}, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "test", Lima: "test", Docker: "test", Compose: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "controlled-home")
	mustMkdir(t, home)
	client := machinegit.Client{
		GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(home, "git-excludes"),
		Executor: process.Runner{},
	}
	if err := client.Prepare(); err != nil {
		t.Fatal(err)
	}
	store, err := (gitstore.Materializer{Git: client}).Materialize(context.Background(), facts, compiled)
	if err != nil {
		t.Fatal(err)
	}
	return testFixture{
		root: root, source: source, workspaceRoot: workspaceRoot, gitPath: gitPath,
		client: client, facts: facts, plan: compiled, store: store,
	}
}

type countingGit struct {
	machinegit.Client
	adds, locks int
}

func (g *countingGit) AddWorktree(ctx context.Context, repository, branch, path, commit string) (machinegit.Result, error) {
	g.adds++
	return g.Client.AddWorktree(ctx, repository, branch, path, commit)
}

func (g *countingGit) LockWorktree(ctx context.Context, repository, path, reason string) (machinegit.Result, error) {
	g.locks++
	return g.Client.LockWorktree(ctx, repository, path, reason)
}

type failingLockGit struct{ machinegit.Client }

func (g *failingLockGit) LockWorktree(context.Context, string, string, string) (machinegit.Result, error) {
	return machinegit.Result{}, errors.New("injected lock failure")
}

type failingObservationGit struct {
	machinegit.Client
	commonCalls int
}

func (g *failingObservationGit) WorktreeCommonDir(ctx context.Context, path string) (machinegit.Result, error) {
	g.commonCalls++
	if g.commonCalls == 1 {
		return machinegit.Result{}, errors.New("injected common-directory observation failure")
	}
	return g.Client.WorktreeCommonDir(ctx, path)
}

type failingDeleteBranchGit struct {
	machinegit.Client
	deletes int
}

func (g *failingDeleteBranchGit) DeleteBranch(ctx context.Context, repository, branch, expectedCommit string) (machinegit.Result, error) {
	g.deletes++
	if g.deletes == 1 {
		return machinegit.Result{}, errors.New("injected branch deletion failure")
	}
	return g.Client.DeleteBranch(ctx, repository, branch, expectedCommit)
}

func mustGit(t *testing.T, gitPath, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=" + testutil.TempDir(t), "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
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
