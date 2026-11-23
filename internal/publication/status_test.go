package publication_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/publication"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/testutil"
)

func TestRealGitPublicationStatus(t *testing.T) {
	t.Run("local Box-only commit", func(t *testing.T) {
		fixture := newFixture(t)
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination)
		assertCommits(t, status[0].UnpublishedCommits, fixture.tip)
		if status[0].Published || !status[0].Destinations[0].Available {
			t.Fatalf("status = %#v, want available destination and unpublished Box commit", status[0])
		}
		if got := strings.TrimSpace(fixture.git(t, fixture.worktree, "status", "--porcelain")); got != "" {
			t.Fatalf("Box worktree is dirty: %q", got)
		}
	})

	t.Run("local destination contains every commit", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.git(t, fixture.source, "fetch", fixture.store, "refs/heads/abox/task:refs/heads/published")
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination)
		if !status[0].Published || len(status[0].UnpublishedCommits) != 0 {
			t.Fatalf("status = %#v, want published", status[0])
		}
	})

	t.Run("missing destination is conservative", func(t *testing.T) {
		fixture := newFixture(t)
		missing := filepath.Join(filepath.Dir(fixture.source), "missing.git")
		fixture.record.PublishDestination = localDestination(missing)
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination)
		assertCommits(t, status[0].UnpublishedCommits, fixture.base, fixture.tip)
		if status[0].Destinations[0].Reason != publication.DestinationMissing {
			t.Fatalf("destination evidence = %#v, want missing", status[0].Destinations[0])
		}
	})

	t.Run("rewritten destination no longer proves old history", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.git(t, fixture.source, "checkout", "--orphan", "replacement")
		fixture.git(t, fixture.source, "rm", "-rf", ".")
		writeFile(t, filepath.Join(fixture.source, "replacement.txt"), "replacement\n", 0o600)
		fixture.git(t, fixture.source, "add", "replacement.txt")
		fixture.git(t, fixture.source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "replacement")
		fixture.git(t, fixture.source, "branch", "-f", "main", "replacement")
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination)
		assertCommits(t, status[0].UnpublishedCommits, fixture.base, fixture.tip)
	})

	t.Run("detached commit", func(t *testing.T) {
		fixture := newFixture(t)
		status := fixture.check(t, []publication.Reference{{Commit: fixture.tip}}, fixture.record.PublishDestination)
		assertCommits(t, status[0].UnpublishedCommits, fixture.tip)
		if status[0].Reference.Name != "" {
			t.Fatalf("reference = %#v, want detached", status[0].Reference)
		}
	})

	t.Run("multiple destinations are unioned with per-destination evidence", func(t *testing.T) {
		fixture := newFixture(t)
		second := filepath.Join(filepath.Dir(fixture.source), "second.git")
		fixture.git(t, filepath.Dir(fixture.source), "clone", "--bare", "--no-hardlinks", fixture.source, second)
		fixture.git(t, second, "fetch", fixture.store, "refs/heads/abox/task:refs/heads/published")
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination, localDestination(second))
		if !status[0].Published || len(status[0].UnpublishedCommits) != 0 {
			t.Fatalf("status = %#v, want published by second destination", status[0])
		}
		assertCommits(t, status[0].Destinations[0].UnpublishedCommits, fixture.tip)
		assertCommits(t, status[0].Destinations[1].UnpublishedCommits)
	})

	t.Run("hostile Box config cannot redirect or execute", func(t *testing.T) {
		fixture := newFixture(t)
		marker := filepath.Join(filepath.Dir(fixture.source), "executed")
		hooks := filepath.Join(filepath.Dir(fixture.source), "hooks")
		if err := os.Mkdir(hooks, 0o700); err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(hooks, "attacker")
		writeFile(t, script, "#!/bin/sh\n: > "+shellQuote(marker)+"\nexit 97\n", 0o700)
		if err := os.Symlink(script, filepath.Join(hooks, "reference-transaction")); err != nil {
			t.Fatal(err)
		}
		for key, value := range map[string]string{
			"core.fsmonitor": script, "core.hooksPath": hooks, "core.pager": script,
			"core.sshCommand": script, "credential.helper": "!" + script,
			"remote.origin.url":                 filepath.Join(filepath.Dir(fixture.source), "attacker.git"),
			"url.ext::" + script + ".insteadOf": fixture.source,
		} {
			fixture.git(t, fixture.store, "config", key, value)
		}
		status := fixture.check(t, []publication.Reference{fixture.branchReference()}, fixture.record.PublishDestination)
		assertCommits(t, status[0].UnpublishedCommits, fixture.tip)
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("hostile Box configuration executed: %v", err)
		}
	})

	t.Run("named ref drift is rejected", func(t *testing.T) {
		fixture := newFixture(t)
		_, err := fixture.checkError([]publication.Reference{{Name: "refs/heads/abox/task", Commit: fixture.base}}, fixture.record.PublishDestination)
		var drift *publication.ReferenceDriftError
		if !errors.As(err, &drift) {
			t.Fatalf("Check() error = %v, want ReferenceDriftError", err)
		}
	})
}

func TestMalformedBoxGraphIsRejected(t *testing.T) {
	root := testutil.TempDir(t)
	identity, err := ownership.InspectPath(root)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	destination := localDestination(root)
	checker := publication.Checker{Git: fakeGit{reachable: process.Result{Stdout: []byte(commit)}}}
	_, err = checker.Check(context.Background(), gitstore.Record{
		Repository: "repo", GitCommonDir: root, Identity: identity,
		PublishDestination: destination,
	}, []publication.Reference{{Commit: commit}}, []repository.Destination{destination})
	if err == nil || !strings.Contains(err.Error(), "not newline terminated") {
		t.Fatalf("Check() error = %v, want malformed rev-list refusal", err)
	}
}

type fixture struct {
	gitPath  string
	home     string
	source   string
	store    string
	worktree string
	base     string
	tip      string
	record   gitstore.Record
	checker  publication.Checker
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gitPath := testutil.RequireGit(t)
	root := testutil.TempDir(t)
	f := &fixture{
		gitPath: gitPath, home: filepath.Join(root, "controlled-home"),
		source: filepath.Join(root, "source with spaces 界"),
		store:  filepath.Join(root, "private.git"), worktree: filepath.Join(root, "worktree"),
	}
	if err := os.Mkdir(f.home, 0o700); err != nil {
		t.Fatal(err)
	}
	f.git(t, root, "init", "--initial-branch=main", f.source)
	writeFile(t, filepath.Join(f.source, "tracked.txt"), "base\n", 0o600)
	f.git(t, f.source, "add", "tracked.txt")
	f.git(t, f.source, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	f.base = strings.TrimSpace(f.git(t, f.source, "rev-parse", "HEAD"))
	f.git(t, root, "clone", "--bare", "--no-hardlinks", f.source, f.store)
	f.git(t, f.store, "remote", "remove", "origin")
	f.git(t, root, "--git-dir", f.store, "worktree", "add", "-b", "abox/task", f.worktree, f.base)
	writeFile(t, filepath.Join(f.worktree, "box.txt"), "Box only\n", 0o600)
	f.git(t, f.worktree, "add", "box.txt")
	f.git(t, f.worktree, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "Box only")
	f.tip = strings.TrimSpace(f.git(t, f.worktree, "rev-parse", "HEAD"))
	identity, err := ownership.InspectPath(f.store)
	if err != nil {
		t.Fatal(err)
	}
	f.record = gitstore.Record{
		Repository: "repo", GitCommonDir: f.store, Identity: identity, BaseCommit: f.base,
		PublishDestination: localDestination(f.source),
	}
	client := machinegit.Client{
		GitPath: f.gitPath, Home: f.home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(f.home, "git-excludes"),
		Executor: process.Runner{},
	}
	if err := client.Prepare(); err != nil {
		t.Fatal(err)
	}
	f.checker = publication.Checker{Git: client}
	return f
}

func (f *fixture) branchReference() publication.Reference {
	return publication.Reference{Name: "refs/heads/abox/task", Commit: f.tip}
}

func (f *fixture) check(t *testing.T, references []publication.Reference, destinations ...repository.Destination) []publication.ReferenceStatus {
	t.Helper()
	status, err := f.checkError(references, destinations...)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	return status
}

func (f *fixture) checkError(references []publication.Reference, destinations ...repository.Destination) ([]publication.ReferenceStatus, error) {
	return f.checker.Check(context.Background(), f.record, references, destinations)
}

func (f *fixture) git(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(f.gitPath, arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=" + f.home, "LANG=C", "LC_ALL=C", "TZ=UTC",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func localDestination(path string) repository.Destination {
	return repository.Destination{Kind: repository.DestinationLocal, URL: path}
}

func assertCommits(t *testing.T, got []string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("commits = %v, want %v", got, want)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

type fakeGit struct {
	reachable process.Result
}

func (fakeGit) ResolveCommit(context.Context, string, string) (machinegit.Result, error) {
	return process.Result{}, errors.New("unexpected ResolveCommit call")
}

func (f fakeGit) ReachableCommits(context.Context, string, ...string) (machinegit.Result, error) {
	return f.reachable, nil
}
