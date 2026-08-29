package machinegit

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestRealGitAdversarialExecutionProfile(t *testing.T) {
	gitPath := realGitPath(t)
	root := t.TempDir()
	repository := filepath.Join(root, "source with spaces 界")
	mustRawGit(t, gitPath, root, "init", "--initial-branch=main", repository)
	mustWrite(t, filepath.Join(repository, "tracked.txt"), "first\n", 0o600)
	mustRawGit(t, gitPath, repository, "add", "tracked.txt")
	mustRawGit(t, gitPath, repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "first")
	base := strings.TrimSpace(mustRawGit(t, gitPath, repository, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(repository, "tracked.txt"), "second\n", 0o600)
	mustRawGit(t, gitPath, repository, "add", "tracked.txt")
	mustRawGit(t, gitPath, repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "second")
	tip := strings.TrimSpace(mustRawGit(t, gitPath, repository, "rev-parse", "HEAD"))

	marker := filepath.Join(root, "machine-code-executed")
	attacker := markerScript(t, root, "attacker", marker, 97)
	hostileHome := filepath.Join(root, "hostile-home")
	mustMkdir(t, hostileHome)
	hostileSystem := filepath.Join(root, "hostile-system.config")
	hostileConfig := hostileGitConfig(attacker)
	mustWrite(t, hostileSystem, hostileConfig, 0o600)
	mustWrite(t, filepath.Join(hostileHome, ".gitconfig"), hostileConfig, 0o600)
	for _, setting := range strings.Split(strings.TrimSpace(hostileRepositoryConfig(attacker)), "\n") {
		parts := strings.SplitN(setting, "=", 2)
		mustRawGit(t, gitPath, repository, "config", "--local", parts[0], parts[1])
	}
	mustRawGit(t, gitPath, repository, "replace", tip, base)
	for _, hook := range []string{"post-index-change", "reference-transaction", "pre-push"} {
		if err := os.Symlink(attacker, filepath.Join(root, hook)); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(repository, ".gitattributes"), "* filter=evil diff=evil\n", 0o600)

	t.Setenv("HOME", hostileHome)
	t.Setenv("GIT_CONFIG_SYSTEM", hostileSystem)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(hostileHome, ".gitconfig"))
	t.Setenv("GIT_SSH_COMMAND", attacker)
	t.Setenv("GIT_ASKPASS", attacker)
	t.Setenv("GIT_PAGER", attacker)
	t.Setenv("GIT_EDITOR", attacker)
	t.Setenv("GIT_PROTOCOL_FROM_USER", "1")
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext:file:ssh:https")
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(root, "redirected-objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(root, "redirected-alternates"))

	client := realClient(t, gitPath, filepath.Join(root, "controlled-home"), Credentials{})
	for name, invoke := range map[string]func() (process.Result, error){
		"top level":      func() (process.Result, error) { return client.TopLevel(context.Background(), repository) },
		"status":         func() (process.Result, error) { return client.Status(context.Background(), repository) },
		"resolve commit": func() (process.Result, error) { return client.ResolveCommit(context.Background(), repository, "HEAD") },
		"check branch": func() (process.Result, error) {
			return client.CheckBranch(context.Background(), repository, "abox/demo")
		},
		"reachability": func() (process.Result, error) { return client.IsAncestor(context.Background(), repository, base, tip) },
	} {
		t.Run(name, func(t *testing.T) {
			result, err := invoke()
			if err != nil {
				t.Fatalf("operation error = %v; stderr = %s", err, result.Stderr)
			}
			assertAbsent(t, marker)
		})
	}

	bundle := filepath.Join(root, "source.bundle")
	mustRawGit(t, gitPath, repository, "bundle", "create", bundle, "refs/heads/main")
	stores := filepath.Join(root, "stores")
	mustMkdir(t, stores)
	store := filepath.Join(stores, "private.git")
	if _, err := client.InitBare(context.Background(), stores, store); err != nil {
		t.Fatalf("InitBare() error = %v", err)
	}
	if _, err := client.VerifyBundle(context.Background(), store, bundle); err != nil {
		t.Fatalf("VerifyBundle() error = %v", err)
	}
	if _, err := client.ImportBundle(context.Background(), store, bundle, "refs/heads/main:refs/heads/imported"); err != nil {
		t.Fatalf("ImportBundle() error = %v", err)
	}
	result, err := client.ResolveCommit(context.Background(), store, "refs/heads/imported")
	if err != nil || strings.TrimSpace(string(result.Stdout)) != tip {
		t.Fatalf("ResolveCommit(imported) = %q, %v, want %s", result.Stdout, err, tip)
	}
	assertAbsent(t, marker)
}

func TestProfileRejectsAlternatesBeforeGitStarts(t *testing.T) {
	repository := t.TempDir()
	mustMkdir(t, filepath.Join(repository, ".git", "objects", "info"))
	mustWrite(t, filepath.Join(repository, ".git", "objects", "info", "alternates"), "/tmp/foreign\n", 0o600)
	recorder := &recordingExecutor{}
	client := testClient(repository, recorder)
	_, err := client.Status(context.Background(), repository)
	if !errors.Is(err, ErrPolicy) || recorder.calls != 0 {
		t.Fatalf("Status() error = %v, calls = %d, want policy refusal before execution", err, recorder.calls)
	}
}

func TestProfileRejectsRepositoryFiltersBeforeStatus(t *testing.T) {
	gitPath := realGitPath(t)
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	mustRawGit(t, gitPath, root, "init", repository)
	marker := filepath.Join(root, "filter-executed")
	filter := markerScript(t, root, "filter", marker, 97)
	mustRawGit(t, gitPath, repository, "config", "filter.evil.clean", filter)
	mustRawGit(t, gitPath, repository, "config", "filter.evil.required", "true")
	mustWrite(t, filepath.Join(repository, ".gitattributes"), "* filter=evil\n", 0o600)
	mustWrite(t, filepath.Join(repository, "source.txt"), "content\n", 0o600)

	client := realClient(t, gitPath, filepath.Join(root, "home"), Credentials{})
	_, err := client.Status(context.Background(), repository)
	if !errors.Is(err, ErrPolicy) {
		t.Fatalf("Status() error = %v, want policy refusal", err)
	}
	assertAbsent(t, marker)
}

func TestProfileRejectsCraftedRefsURLsHelpersAndOptions(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingExecutor{}
	client := testClient(root, recorder)
	client.Credentials.SSH = "/usr/bin/ssh"
	sha := strings.Repeat("a", 40)

	tests := map[string]func() error{
		"option ref": func() error { _, err := client.ResolveCommit(context.Background(), root, "--help"); return err },
		"ref traversal": func() error {
			_, err := client.ResolveCommit(context.Background(), root, "refs/heads/a..b")
			return err
		},
		"ref newline": func() error {
			_, err := client.ResolveCommit(context.Background(), root, "refs/heads/a\nb")
			return err
		},
		"remote helper": func() error {
			_, err := client.Fetch(context.Background(), root, "evil::payload", sha+":refs/heads/base")
			return err
		},
		"ext transport": func() error {
			_, err := client.Fetch(context.Background(), root, "ext::sh -c marker", sha+":refs/heads/base")
			return err
		},
		"file transport": func() error {
			_, err := client.Fetch(context.Background(), root, "file:///tmp/source", sha+":refs/heads/base")
			return err
		},
		"URL password": func() error {
			_, err := client.Fetch(context.Background(), root, "https://user:secret@example.invalid/repo", sha+":refs/heads/base")
			return err
		},
		"encoded newline": func() error {
			_, err := client.Fetch(context.Background(), root, "https://example.invalid/repo%0aoption", sha+":refs/heads/base")
			return err
		},
		"option URL": func() error {
			_, err := client.Push(context.Background(), root, "--receive-pack=evil", "refs/heads/main:refs/heads/main")
			return err
		},
		"forced push": func() error {
			_, err := client.Push(context.Background(), root, "https://example.invalid/repo", "+refs/heads/main:refs/heads/main")
			return err
		},
		"delete push": func() error {
			_, err := client.Push(context.Background(), root, "https://example.invalid/repo", ":refs/heads/main")
			return err
		},
		"wildcard fetch": func() error {
			_, err := client.Fetch(context.Background(), root, "https://example.invalid/repo", "refs/heads/*:refs/heads/*")
			return err
		},
		"ambiguous refspec": func() error {
			_, err := client.Fetch(context.Background(), root, "https://example.invalid/repo", "refs/heads/main")
			return err
		},
	}
	for name, invoke := range tests {
		t.Run(name, func(t *testing.T) {
			if err := invoke(); !errors.Is(err, ErrPolicy) {
				t.Fatalf("error = %v, want ErrPolicy", err)
			}
		})
	}
	if recorder.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", recorder.calls)
	}
}

func TestCommandUsesOnlyControlledEnvironmentAndExplicitCredentials(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingExecutor{}
	credentials := Credentials{
		AskPass:     filepath.Join(root, "askpass"),
		SSH:         filepath.Join(root, "ssh"),
		SSHAuthSock: filepath.Join(root, "agent.sock"),
	}
	client := testClient(root, recorder)
	client.Credentials = credentials
	_, err := client.Push(context.Background(), root, "git@example.invalid:repo.git", "refs/heads/main:refs/heads/main")
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	command := recorder.last
	for _, forbidden := range []string{"GIT_SSH_COMMAND", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CEILING_DIRECTORIES"} {
		if _, exists := command.Env[forbidden]; exists {
			t.Fatalf("environment contains inherited %s", forbidden)
		}
	}
	for key, want := range map[string]string{
		"GIT_CONFIG_GLOBAL": nullPath, "GIT_CONFIG_SYSTEM": nullPath,
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_PROTOCOL_FROM_USER": "0",
		"GIT_OPTIONAL_LOCKS": "0",
		"GIT_ALLOW_PROTOCOL": "ssh", "GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS": credentials.AskPass, "GIT_SSH": credentials.SSH,
		"SSH_AUTH_SOCK": credentials.SSHAuthSock,
	} {
		if got := command.Env[key]; got != want {
			t.Errorf("Env[%s] = %q, want %q", key, got, want)
		}
	}
	joined := strings.Join(command.Argv, "\x00")
	for _, want := range []string{"--no-pager", "--no-replace-objects", "core.hooksPath=/dev/null", "core.fsmonitor=false", "credential.helper=", "protocol.allow=never", "--\x00git@example.invalid:repo.git"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv does not contain %q: %q", want, command.Argv)
		}
	}
}

func TestExplicitSSHCredentialPathWinsOverAmbientCommand(t *testing.T) {
	gitPath := realGitPath(t)
	root := t.TempDir()
	ambientMarker := filepath.Join(root, "ambient")
	explicitMarker := filepath.Join(root, "explicit")
	ambient := markerScript(t, root, "ambient-ssh", ambientMarker, 98)
	explicit := markerScript(t, root, "explicit-ssh", explicitMarker, 99)
	t.Setenv("GIT_SSH_COMMAND", ambient)

	repository := filepath.Join(root, "repo")
	mustRawGit(t, gitPath, root, "init", "--bare", repository)
	client := realClient(t, gitPath, filepath.Join(root, "home"), Credentials{SSH: explicit})
	_, err := client.Fetch(context.Background(), repository, "ssh://example.invalid/repo", strings.Repeat("a", 40)+":refs/heads/base")
	if err == nil {
		t.Fatal("Fetch() succeeded through failing fixture SSH executable")
	}
	assertAbsent(t, ambientMarker)
	if _, statErr := os.Stat(explicitMarker); statErr != nil {
		t.Fatalf("explicit SSH executable was not invoked: %v", statErr)
	}
}

func TestProductionMachineGitCallsCannotBypassProfile(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	internalRoot := filepath.Dir(filepath.Dir(current))
	allowed := map[string]bool{"machinegit": true, "process": true, "spike": true}
	var violations []string
	entries, err := os.ReadDir(internalRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || allowed[entry.Name()] {
			continue
		}
		packageRoot := filepath.Join(internalRoot, entry.Name())
		_ = filepath.WalkDir(packageRoot, func(path string, item os.DirEntry, walkErr error) error {
			if walkErr != nil || item.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return walkErr
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imported := range file.Imports {
				name := strings.Trim(imported.Path.Value, "\"")
				if name == "os/exec" || strings.HasSuffix(name, "/internal/process") {
					violations = append(violations, path+": "+name)
				}
			}
			return nil
		})
	}
	slices.Sort(violations)
	if len(violations) != 0 {
		t.Fatalf("Machine command boundary bypasses: %v", violations)
	}
}

type recordingExecutor struct {
	calls int
	last  process.Command
}

func (r *recordingExecutor) Run(_ context.Context, command process.Command) (process.Result, error) {
	r.calls++
	r.last = command
	return process.Result{ExitCode: 0}, nil
}

func testClient(root string, executor Executor) Client {
	return Client{
		GitPath:  "/usr/bin/git",
		Home:     filepath.Join(root, "controlled-home"),
		Path:     "/usr/bin:/bin",
		Executor: executor,
	}
}

func realClient(t *testing.T, gitPath, home string, credentials Credentials) Client {
	t.Helper()
	mustMkdir(t, home)
	return Client{GitPath: gitPath, Home: home, Path: "/usr/bin:/bin", Credentials: credentials, Executor: process.Runner{}}
}

func realGitPath(t *testing.T) string {
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

func mustRawGit(t *testing.T, gitPath, directory string, arguments ...string) string {
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
		t.Fatalf("fixture git %q failed: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func hostileGitConfig(script string) string {
	settings := [][2]string{
		{"core.fsmonitor", script}, {"core.hooksPath", filepath.Dir(script)},
		{"core.pager", script}, {"core.editor", script}, {"core.askPass", script},
		{"core.sshCommand", script}, {"credential.helper", "!" + script},
		{"diff.external", script}, {"filter.evil.clean", script}, {"filter.evil.smudge", script},
		{"filter.evil.required", "true"}, {"protocol.ext.allow", "always"},
		{"url.ext::" + script + ".insteadOf", "https://example.invalid/"},
		{"fetch.recurseSubmodules", "true"}, {"submodule.recurse", "true"},
	}
	var output strings.Builder
	for _, setting := range settings {
		fmt.Fprintf(&output, "%s=%s\n", setting[0], setting[1])
	}
	return output.String()
}

func hostileRepositoryConfig(script string) string {
	settings := [][2]string{
		{"core.fsmonitor", script}, {"core.hooksPath", filepath.Dir(script)},
		{"core.pager", script}, {"core.editor", script}, {"core.askPass", script},
		{"core.sshCommand", script}, {"credential.helper", "!" + script},
		{"diff.external", script}, {"protocol.ext.allow", "always"},
		{"url.ext::" + script + ".insteadOf", "https://example.invalid/"},
		{"fetch.recurseSubmodules", "true"}, {"submodule.recurse", "true"},
	}
	var output strings.Builder
	for _, setting := range settings {
		fmt.Fprintf(&output, "%s=%s\n", setting[0], setting[1])
	}
	return output.String()
}

func markerScript(t *testing.T, root, name, marker string, exitCode int) string {
	t.Helper()
	path := filepath.Join(root, name)
	content := fmt.Sprintf("#!/bin/sh\n: > %s\nexit %d\n", shellQuote(marker), exitCode)
	mustWrite(t, path, content, 0o700)
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
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

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected marker %q: %v", path, err)
	}
}
