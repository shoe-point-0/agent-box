package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestResolverReturnsTrustedFactsWithoutMaterialization(t *testing.T) {
	gitPath := requireGit(t)
	root := t.TempDir()
	source := filepath.Join(root, "source with spaces 界")
	mustGit(t, gitPath, root, "init", "--initial-branch=main", source)
	mustMkdir(t, filepath.Join(source, "nested"))
	mustWrite(t, filepath.Join(source, "tracked.txt"), "content\n", 0o600)
	mustGit(t, gitPath, source, "add", "tracked.txt")
	mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	wantCommit := strings.TrimSpace(mustGit(t, gitPath, source, "rev-parse", "HEAD"))

	marker := filepath.Join(root, "hostile-command-ran")
	script := filepath.Join(root, "hostile")
	mustWrite(t, script, "#!/bin/sh\n: > "+shellQuote(marker)+"\nexit 91\n", 0o700)
	for key, value := range map[string]string{
		"core.hooksPath":          root,
		"core.fsmonitor":          script,
		"core.sshCommand":         script,
		"credential.helper":       "!" + script,
		"remote.origin.url":       "ext::" + script,
		"url.ext::evil.insteadOf": source,
	} {
		mustGit(t, gitPath, source, "config", "--local", key, value)
	}

	provider, boxID := testPathsAndBox(t, root)
	home := filepath.Join(root, "controlled-home")
	mustMkdir(t, home)
	resolver := Resolver{
		Git:   preparedMachineGit(t, gitPath, home),
		Paths: provider,
	}
	workspace := testWorkspace(filepath.Join(source, "nested"), "abox/{{ box.task }}/repo")

	before := snapshotTree(t, root)
	facts, err := resolver.Resolve(context.Background(), workspace, boxID, "DEMO-42")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("resolution mutated filesystem\nbefore: %v\nafter:  %v", before, after)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("hostile Git configuration executed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("fact count = %d, want 1", len(facts))
	}
	wantWorktree, _ := provider.WorkspacePath(boxID, "services/repo")
	wantStore, _ := provider.GitStore(boxID, "repo")
	want := Facts{
		Name: "repo", SourceRoot: source, Clean: true, BaseCommit: wantCommit,
		Branch: "abox/DEMO-42/repo", WorktreePath: wantWorktree, GitStorePath: wantStore,
		PublishDestination: Destination{Kind: DestinationLocal, URL: source},
	}
	if facts[0] != want {
		t.Fatalf("facts = %#v, want %#v", facts[0], want)
	}
	if _, err := os.Stat(provider.ApplicationRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("application state root was materialized: %v", err)
	}
}

func TestResolverRefusesDirtySourceWithExactPathsAndNoFacts(t *testing.T) {
	gitPath := requireGit(t)
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustGit(t, gitPath, root, "init", "--initial-branch=main", source)
	mustWrite(t, filepath.Join(source, "old name.txt"), "content\n", 0o600)
	mustGit(t, gitPath, source, "add", "old name.txt")
	mustGit(t, gitPath, source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	mustGit(t, gitPath, source, "mv", "old name.txt", "new name.txt")
	mustWrite(t, filepath.Join(source, "untracked.txt"), "secret\n", 0o600)

	provider, boxID := testPathsAndBox(t, root)
	home := filepath.Join(root, "home")
	mustMkdir(t, home)
	resolver := Resolver{
		Git:   preparedMachineGit(t, gitPath, home),
		Paths: provider,
	}
	before := snapshotTree(t, root)
	facts, err := resolver.Resolve(context.Background(), testWorkspace(source, "abox/task/repo"), boxID, "task")
	if facts != nil {
		t.Fatalf("dirty resolution returned partial facts: %#v", facts)
	}
	var dirty *DirtySourceError
	if !errors.As(err, &dirty) {
		t.Fatalf("Resolve() error = %v, want DirtySourceError", err)
	}
	wantPaths := []string{"new name.txt", "old name.txt", "untracked.txt"}
	if got := dirty.ChangedPaths(); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("changed paths = %q, want %q", got, wantPaths)
	}
	changed := dirty.ChangedPaths()
	changed[0] = "mutated"
	if reflect.DeepEqual(changed, dirty.ChangedPaths()) {
		t.Fatal("ChangedPaths exposed mutable internal storage")
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("dirty-source refusal mutated filesystem")
	}
	if _, statErr := os.Stat(provider.ApplicationRoot()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dirty-source refusal materialized state: %v", statErr)
	}
}

func TestResolverRejectsBranchExpansionAndMalformedObservations(t *testing.T) {
	root := t.TempDir()
	untrustedTop := t.TempDir()
	provider, boxID := testPathsAndBox(t, root)
	sha := strings.Repeat("a", 40)
	tests := []struct {
		name   string
		git    *fakeGit
		branch string
	}{
		{name: "branch expansion", git: &fakeGit{top: root + "\n", commit: sha + "\n", branch: "main\n"}, branch: "@{-1}"},
		{name: "abbreviated commit", git: &fakeGit{top: root + "\n", commit: "deadbeef\n", branch: "abox/task\n"}, branch: "abox/task"},
		{name: "non terminated status", git: &fakeGit{top: root + "\n", status: "? file", commit: sha + "\n", branch: "abox/task\n"}, branch: "abox/task"},
		{name: "untrusted top level", git: &fakeGit{top: untrustedTop + "\n", commit: sha + "\n", branch: "abox/task\n"}, branch: "abox/task"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := testWorkspace(root, test.branch)
			facts, err := (Resolver{Git: test.git, Paths: provider}).Resolve(context.Background(), workspace, boxID, "task")
			if err == nil || facts != nil {
				t.Fatalf("Resolve() = %#v, %v, want refusal with no facts", facts, err)
			}
		})
	}
}

func TestResolverPropagatesContextCancellation(t *testing.T) {
	root := t.TempDir()
	provider, boxID := testPathsAndBox(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observer := &fakeGit{err: ctx.Err()}
	facts, err := (Resolver{Git: observer, Paths: provider}).Resolve(ctx, testWorkspace(root, "abox/task"), boxID, "task")
	if facts != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve() = %#v, %v, want context cancellation", facts, err)
	}
}

type fakeGit struct {
	top, status, commit, branch string
	err                         error
}

func (f *fakeGit) TopLevel(context.Context, string) (process.Result, error) {
	return process.Result{Stdout: []byte(f.top)}, f.err
}
func (f *fakeGit) Status(context.Context, string) (process.Result, error) {
	return process.Result{Stdout: []byte(f.status)}, f.err
}
func (f *fakeGit) ResolveCommit(context.Context, string, string) (process.Result, error) {
	return process.Result{Stdout: []byte(f.commit)}, f.err
}
func (f *fakeGit) CheckBranch(context.Context, string, string) (process.Result, error) {
	return process.Result{Stdout: []byte(f.branch)}, f.err
}

func testWorkspace(source, branch string) *manifest.Workspace {
	return &manifest.Workspace{Spec: manifest.WorkspaceSpec{Repositories: []manifest.Repository{{
		Name: "repo", Source: manifest.Source{Local: source}, CanonicalLocal: source,
		Checkout: manifest.Checkout{Ref: "HEAD", Branch: branch}, WorkspacePath: "services/repo",
	}}}}
}

func preparedMachineGit(t *testing.T, gitPath, home string) machinegit.Client {
	t.Helper()
	client := machinegit.Client{
		GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(home, "git-excludes"),
		Executor: process.Runner{},
	}
	if err := client.Prepare(); err != nil {
		t.Fatal(err)
	}
	return client
}

func testPathsAndBox(t *testing.T, root string) (*paths.Provider, identity.BoxID) {
	t.Helper()
	provider, err := paths.New(filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.Workspace(filepath.Join(root, "box.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "task")
	if err != nil {
		t.Fatal(err)
	}
	return provider, boxID
}

type treeEntry struct {
	Path    string
	Mode    fs.FileMode
	Size    int64
	ModTime int64
	Digest  [sha256.Size]byte
}

func snapshotTree(t *testing.T, root string) []treeEntry {
	t.Helper()
	var entries []treeEntry
	err := filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := treeEntry{Path: relative, Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Digest = sha256.Sum256(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.Digest = sha256.Sum256([]byte(target))
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Path < entries[right].Path })
	return entries
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
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(path)
}

func mustGit(t *testing.T, gitPath, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestDirtyErrorDoesNotReflectPaths(t *testing.T) {
	errorValue := &DirtySourceError{Repository: "repo", paths: []string{"secret\npath"}}
	if strings.Contains(errorValue.Error(), "secret") {
		t.Fatalf("error reflected changed path: %s", errorValue)
	}
	if got := fmt.Sprint(errorValue); got != "repository source is dirty: repo" {
		t.Fatalf("error = %q", got)
	}
}
