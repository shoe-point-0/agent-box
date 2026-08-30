// Package machinegit is the only Machine-side Git process boundary.
package machinegit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/shoe-point-0/agent-box/internal/process"
)

const (
	maxRefLength           = 1024
	falsePath              = "/bin/false"
	nullPath               = "/dev/null"
	managedExcludesContent = ".abox-probe/\n"
)

var (
	ErrPolicy         = errors.New("machine git policy refusal")
	ErrBranchNotFound = errors.New("machine git branch not found")
)

// Result is the observation returned by the hardened Git boundary. The alias
// lets callers define narrow Git interfaces without importing the subprocess
// package and bypassing the production boundary guard.
type Result = process.Result

// Executor is implemented by process.Runner. Tests may substitute a recorder.
type Executor interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Credentials are opt-in Machine credential channels. Executable paths and
// the SSH agent socket must be absolute; inherited Git credential variables
// are never used.
type Credentials struct {
	AskPass     string
	SSH         string
	SSHAuthSock string
}

// Client owns the complete execution profile for every Machine-side Git call.
type Client struct {
	GitPath      string
	Home         string
	Path         string
	ExcludesFile string
	Credentials  Credentials
	Executor     Executor
}

// Prepare atomically materializes the only global ignore policy trusted by
// Machine Git. It never reads or writes the user's global Git configuration.
func (c Client) Prepare() error {
	if err := validateCanonicalPath("controlled home", c.Home); err != nil {
		return err
	}
	if err := validateCanonicalPath("managed excludes file", c.ExcludesFile); err != nil {
		return err
	}
	if filepath.Dir(c.ExcludesFile) != c.Home {
		return refuse("managed excludes file", "must be an immediate child of the controlled home")
	}
	return ensureManagedExcludes(c.Home, c.ExcludesFile)
}

// PolicyError reports a rejected input class without reflecting potentially
// credential-bearing URLs or attacker-controlled values.
type PolicyError struct {
	Field  string
	Reason string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrPolicy, e.Field, e.Reason)
}

func (e *PolicyError) Unwrap() error { return ErrPolicy }

// TopLevel resolves a source checkout through script-safe porcelain.
func (c Client) TopLevel(ctx context.Context, repository string) (process.Result, error) {
	return c.run(ctx, repository, noTransport, "rev-parse", "--path-format=absolute", "--show-toplevel")
}

// Status observes all source changes without executing repository hooks,
// fsmonitor commands, pagers, editors, or text conversion/filter drivers.
func (c Client) Status(ctx context.Context, repository string) (process.Result, error) {
	if err := c.rejectConfiguredFilters(ctx, repository); err != nil {
		return process.Result{}, err
	}
	return c.run(ctx, repository, noTransport, "status", "--porcelain=v2", "-z", "--untracked-files=all")
}

// IgnoredFiles enumerates every ignored file that a raw worktree directory
// removal would discard. Unlike status, ls-files does not collapse ignored
// directories, and -z preserves arbitrary path bytes other than NUL.
func (c Client) IgnoredFiles(ctx context.Context, repository string) (process.Result, error) {
	return c.run(ctx, repository, noTransport, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
}

// ResolveCommit resolves a validated ref to exactly one commit object.
func (c Client) ResolveCommit(ctx context.Context, repository, ref string) (process.Result, error) {
	if !validObjectName(ref, true) {
		return process.Result{}, refuse("ref", "must be HEAD, a full ref, or a full object ID")
	}
	return c.run(ctx, repository, noTransport, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
}

// CheckBranch validates one intended local branch name through Git's own
// ref-format rules. Callers must require stdout to equal branch exactly because
// --branch can otherwise expand previous-checkout syntax.
func (c Client) CheckBranch(ctx context.Context, repository, branch string) (process.Result, error) {
	if !validBranch(branch) {
		return process.Result{}, refuse("branch", "must be a bounded branch name without options or control characters")
	}
	return c.run(ctx, repository, noTransport, "check-ref-format", "--branch", branch)
}

// LookupBranch observes an exact local branch without consulting remote or
// abbreviated ref namespaces. A missing branch returns a process ExitError
// with code 1, matching git rev-parse --quiet's script-facing contract.
func (c Client) LookupBranch(ctx context.Context, repository, branch string) (process.Result, error) {
	if !validBranch(branch) {
		return process.Result{}, refuse("branch", "must be a bounded branch name without options or control characters")
	}
	result, err := c.run(ctx, repository, noTransport, "rev-parse", "--verify", "--quiet", "--end-of-options", "refs/heads/"+branch+"^{commit}")
	var exitErr *process.ExitError
	if errors.As(err, &exitErr) && exitErr.Code == 1 {
		return result, ErrBranchNotFound
	}
	return result, err
}

// InitBare creates an empty private store. Object transfer is deliberately a
// separate fetch or bundle-import operation so checkout filters stay inert.
func (c Client) InitBare(ctx context.Context, parent, destination string) (process.Result, error) {
	if err := validateCanonicalPath("destination", destination); err != nil {
		return process.Result{}, err
	}
	if filepath.Dir(destination) != parent {
		return process.Result{}, refuse("destination", "must be an immediate child of the operation directory")
	}
	return c.runRaw(ctx, parent, noTransport, "init", "--bare", "--initial-branch=abox-seed", "--", destination)
}

// CloneBare copies a local repository into an already-owned or absent bare
// destination. --local plus --no-hardlinks makes object storage independent
// without invoking a transport helper or retaining alternates.
func (c Client) CloneBare(ctx context.Context, parent, source, destination string) (process.Result, error) {
	if err := validateCanonicalPath("source", source); err != nil {
		return process.Result{}, err
	}
	if err := rejectAlternates(source); err != nil {
		return process.Result{}, err
	}
	if err := validateCanonicalPath("destination", destination); err != nil {
		return process.Result{}, err
	}
	if filepath.Dir(destination) != parent {
		return process.Result{}, refuse("destination", "must be an immediate child of the operation directory")
	}
	return c.runRaw(
		ctx,
		parent,
		fileTransport,
		"clone", "--bare", "--local", "--no-hardlinks", "--no-tags", "--no-recurse-submodules", "--", source, destination,
	)
}

// SetPrivateBase pins the immutable planned commit in a private namespace so
// detached commits remain reachable without creating the task branch early.
func (c Client) SetPrivateBase(ctx context.Context, repository, commit string) (process.Result, error) {
	if !validObjectID(commit) {
		return process.Result{}, refuse("commit", "must be a full object ID")
	}
	return c.run(ctx, repository, noTransport, "update-ref", "--no-deref", "refs/abox/base", commit)
}

// ConfigurePrivateStore enables reflogs and removes clone-created publication
// configuration. The trusted publish destination remains outside this store.
func (c Client) ConfigurePrivateStore(ctx context.Context, repository string) error {
	if _, err := c.run(ctx, repository, noTransport, "config", "--local", "core.logAllRefUpdates", "true"); err != nil {
		return err
	}
	_, err := c.run(ctx, repository, noTransport, "config", "--local", "--remove-section", "remote.origin")
	return err
}

// UsesRelativeWorktrees reports whether a private store enables Git's relative
// worktree metadata. Agent Box currently requires invariant absolute paths, so
// callers reject a true result before invoking repository-aware commands.
func (c Client) UsesRelativeWorktrees(ctx context.Context, repository string) (bool, error) {
	if err := validateCanonicalPath("repository", repository); err != nil {
		return false, err
	}
	if err := rejectAlternates(repository); err != nil {
		return false, err
	}
	config := filepath.Join(repository, "config")
	for _, key := range []string{"extensions.relativeWorktrees", "worktree.useRelativePaths"} {
		enabled, found, err := c.configBool(ctx, repository, config, key)
		if err != nil {
			return false, err
		}
		if found && enabled {
			return true, nil
		}
	}
	return false, nil
}

// WorktreeList returns Git's NUL-delimited script-facing registration records.
func (c Client) WorktreeList(ctx context.Context, repository string) (process.Result, error) {
	return c.run(ctx, repository, noTransport, "worktree", "list", "--porcelain", "-z")
}

// Gitlinks lists a planned tree recursively so callers can reject active
// submodules before creating a linked worktree.
func (c Client) Gitlinks(ctx context.Context, repository, commit string) (process.Result, error) {
	if !validObjectID(commit) {
		return process.Result{}, refuse("commit", "must be a full object ID")
	}
	return c.run(ctx, repository, noTransport, "ls-tree", "-r", "-z", "--full-tree", commit)
}

// AddWorktree creates one generated branch and linked worktree at an exact
// commit. Branch and commit inputs are validated before Git starts.
func (c Client) AddWorktree(ctx context.Context, repository, branch, path, commit string) (process.Result, error) {
	if !validBranch(branch) {
		return process.Result{}, refuse("branch", "must be a bounded branch name without options or control characters")
	}
	if err := validateCanonicalPath("worktree", path); err != nil {
		return process.Result{}, err
	}
	if !validObjectID(commit) {
		return process.Result{}, refuse("commit", "must be a full object ID")
	}
	if err := c.rejectConfiguredFilters(ctx, repository); err != nil {
		return process.Result{}, err
	}
	return c.run(ctx, repository, noTransport, "worktree", "add", "-b", branch, "--", path, commit)
}

// LockWorktree protects one linked worktree with an explicit ownership reason.
func (c Client) LockWorktree(ctx context.Context, repository, path, reason string) (process.Result, error) {
	if err := validateCanonicalPath("worktree", path); err != nil {
		return process.Result{}, err
	}
	if reason == "" || len(reason) > 1024 || hasControl(reason) {
		return process.Result{}, refuse("lock reason", "must be bounded text without control characters")
	}
	return c.run(ctx, repository, noTransport, "worktree", "lock", "--reason", reason, "--", path)
}

// UnlockWorktree unlocks a caller-proved worktree immediately before cleanup.
func (c Client) UnlockWorktree(ctx context.Context, repository, path string) (process.Result, error) {
	if err := validateCanonicalPath("worktree", path); err != nil {
		return process.Result{}, err
	}
	return c.run(ctx, repository, noTransport, "worktree", "unlock", "--", path)
}

// WorktreeCommonDir resolves the absolute common Git directory for a linked
// worktree without relying on its writable configuration as ownership state.
func (c Client) WorktreeCommonDir(ctx context.Context, worktree string) (process.Result, error) {
	return c.run(ctx, worktree, noTransport, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

// RemoveWorktree removes a caller-proved clean linked worktree. This method
// performs no ownership decision; callers must prove the exact resource first.
func (c Client) RemoveWorktree(ctx context.Context, repository, path string) (process.Result, error) {
	if err := validateCanonicalPath("worktree", path); err != nil {
		return process.Result{}, err
	}
	return c.run(ctx, repository, noTransport, "worktree", "remove", "--", path)
}

// DeleteBranch removes a generated ref only if it still names expectedCommit.
func (c Client) DeleteBranch(ctx context.Context, repository, branch, expectedCommit string) (process.Result, error) {
	if !validBranch(branch) {
		return process.Result{}, refuse("branch", "must be a bounded branch name without options or control characters")
	}
	if !validObjectID(expectedCommit) {
		return process.Result{}, refuse("commit", "must be a full object ID")
	}
	return c.run(ctx, repository, noTransport, "update-ref", "-d", "refs/heads/"+branch, expectedCommit)
}

// Fetch imports explicit refs from an explicit supported URL. It never reads a
// remote name or destination from repository-writable configuration.
func (c Client) Fetch(ctx context.Context, repository, remoteURL string, refspecs ...string) (process.Result, error) {
	transport, err := validateRemoteURL(remoteURL, c.Credentials)
	if err != nil {
		return process.Result{}, err
	}
	if err := validateRefspecs(refspecs, true); err != nil {
		return process.Result{}, err
	}
	arguments := []string{"fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--", remoteURL}
	arguments = append(arguments, refspecs...)
	return c.run(ctx, repository, transport, arguments...)
}

// VerifyBundle validates a local bundle before import.
func (c Client) VerifyBundle(ctx context.Context, repository, bundle string) (process.Result, error) {
	if err := validateCanonicalPath("bundle", bundle); err != nil {
		return process.Result{}, err
	}
	return c.run(ctx, repository, noTransport, "bundle", "verify", bundle)
}

// ImportBundle imports explicit refs from a previously verified bundle.
func (c Client) ImportBundle(ctx context.Context, repository, bundle string, refspecs ...string) (process.Result, error) {
	if err := validateCanonicalPath("bundle", bundle); err != nil {
		return process.Result{}, err
	}
	if err := validateRefspecs(refspecs, true); err != nil {
		return process.Result{}, err
	}
	arguments := []string{"fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--", bundle}
	arguments = append(arguments, refspecs...)
	return c.run(ctx, repository, fileTransport, arguments...)
}

// IsAncestor performs a reachability check without replacement refs.
func (c Client) IsAncestor(ctx context.Context, repository, base, tip string) (process.Result, error) {
	if !validObjectName(base, false) || !validObjectName(tip, false) {
		return process.Result{}, refuse("object", "must be a full ref or full object ID")
	}
	return c.run(ctx, repository, noTransport, "merge-base", "--is-ancestor", base, tip)
}

// ReachableCommits enumerates commit IDs reachable from explicit trusted tips.
// With no tips it enumerates all refs in repository. It never consults a
// configured remote or enables a transport, and the terminating -- prevents
// paths from being interpreted as revision arguments.
func (c Client) ReachableCommits(ctx context.Context, repository string, tips ...string) (process.Result, error) {
	arguments := []string{"rev-list"}
	if len(tips) == 0 {
		arguments = append(arguments, "--all")
	} else {
		for _, tip := range tips {
			if !validObjectName(tip, false) {
				return process.Result{}, refuse("tip", "must be a full ref or full object ID")
			}
		}
		arguments = append(arguments, tips...)
	}
	arguments = append(arguments, "--")
	return c.run(ctx, repository, noTransport, arguments...)
}

// Push publishes explicit non-forced refspecs to an explicit destination.
func (c Client) Push(ctx context.Context, repository, remoteURL string, refspecs ...string) (process.Result, error) {
	transport, err := validateRemoteURL(remoteURL, c.Credentials)
	if err != nil {
		return process.Result{}, err
	}
	if err := validateRefspecs(refspecs, false); err != nil {
		return process.Result{}, err
	}
	arguments := []string{"push", "--porcelain", "--no-verify", "--no-signed", "--", remoteURL}
	arguments = append(arguments, refspecs...)
	return c.run(ctx, repository, transport, arguments...)
}

type transport string

const (
	noTransport    transport = ""
	fileTransport  transport = "file"
	httpsTransport transport = "https"
	sshTransport   transport = "ssh"
)

func (c Client) run(ctx context.Context, repository string, transport transport, arguments ...string) (process.Result, error) {
	if err := validateCanonicalPath("repository", repository); err != nil {
		return process.Result{}, err
	}
	if err := rejectAlternates(repository); err != nil {
		return process.Result{}, err
	}
	arguments = append([]string{"-C", repository}, arguments...)
	return c.runRaw(ctx, repository, transport, arguments...)
}

func (c Client) rejectConfiguredFilters(ctx context.Context, repository string) error {
	if err := validateCanonicalPath("repository", repository); err != nil {
		return err
	}
	if err := rejectAlternates(repository); err != nil {
		return err
	}
	result, err := c.runRaw(
		ctx,
		repository,
		noTransport,
		"-C", repository,
		"config", "--local", "--null", "--get-regexp", `^filter\..*\.(clean|smudge|process)$`,
	)
	if err == nil {
		return refuse("repository", "configured clean, smudge, or process filters are forbidden")
	}
	var exitErr *process.ExitError
	if errors.As(err, &exitErr) && exitErr.Code == 1 && len(result.Stdout) == 0 {
		return nil
	}
	return err
}

func (c Client) configBool(ctx context.Context, directory, config, key string) (bool, bool, error) {
	result, err := c.runRaw(ctx, directory, noTransport, "config", "--file", config, "--type=bool", "--get", key)
	if err == nil {
		switch strings.TrimSpace(string(result.Stdout)) {
		case "true":
			return true, true, nil
		case "false":
			return false, true, nil
		default:
			return false, false, errors.New("machine git returned a non-boolean configuration value")
		}
	}
	var exitErr *process.ExitError
	if errors.As(err, &exitErr) && exitErr.Code == 1 && len(result.Stdout) == 0 {
		return false, false, nil
	}
	return false, false, err
}

func (c Client) runRaw(ctx context.Context, directory string, transport transport, arguments ...string) (process.Result, error) {
	command, err := c.command(directory, transport, arguments...)
	if err != nil {
		return process.Result{}, err
	}
	return c.Executor.Run(ctx, command)
}

func (c Client) command(directory string, transport transport, arguments ...string) (process.Command, error) {
	if c.Executor == nil {
		return process.Command{}, refuse("executor", "is required")
	}
	for field, value := range map[string]string{
		"git executable": c.GitPath, "controlled home": c.Home, "managed excludes file": c.ExcludesFile,
	} {
		if err := validateCanonicalPath(field, value); err != nil {
			return process.Command{}, err
		}
	}
	if filepath.Dir(c.ExcludesFile) != c.Home {
		return process.Command{}, refuse("managed excludes file", "must be an immediate child of the controlled home")
	}
	if err := validateCanonicalPath("working directory", directory); err != nil {
		return process.Command{}, err
	}
	if err := validatePathList(c.Path); err != nil {
		return process.Command{}, err
	}
	if err := validateCredentials(c.Credentials); err != nil {
		return process.Command{}, err
	}
	if err := validateManagedExcludes(c.ExcludesFile); err != nil {
		return process.Command{}, err
	}

	argv := []string{c.GitPath, "--no-pager", "--no-replace-objects"}
	for _, setting := range fixedConfig(c.Credentials, c.ExcludesFile) {
		argv = append(argv, "-c", setting)
	}
	argv = append(argv, arguments...)

	environment := map[string]string{
		"PATH":                   c.Path,
		"HOME":                   c.Home,
		"XDG_CONFIG_HOME":        c.Home,
		"LANG":                   "C",
		"LC_ALL":                 "C",
		"TZ":                     "UTC",
		"GIT_CONFIG_GLOBAL":      nullPath,
		"GIT_CONFIG_SYSTEM":      nullPath,
		"GIT_CONFIG_NOSYSTEM":    "1",
		"GIT_CONFIG_COUNT":       "0",
		"GIT_ATTR_NOSYSTEM":      "1",
		"GIT_TERMINAL_PROMPT":    "0",
		"GIT_PROTOCOL_FROM_USER": "0",
		"GIT_NO_REPLACE_OBJECTS": "1",
		"GIT_OPTIONAL_LOCKS":     "0",
		"GIT_PAGER":              "cat",
		"PAGER":                  "cat",
		"GIT_EDITOR":             falsePath,
		"GIT_SEQUENCE_EDITOR":    falsePath,
	}
	if transport != noTransport {
		environment["GIT_ALLOW_PROTOCOL"] = string(transport)
	}
	if c.Credentials.AskPass != "" {
		environment["GIT_ASKPASS"] = c.Credentials.AskPass
		environment["SSH_ASKPASS"] = c.Credentials.AskPass
	}
	if c.Credentials.SSH != "" {
		environment["GIT_SSH"] = c.Credentials.SSH
		environment["GIT_SSH_VARIANT"] = "ssh"
	}
	if c.Credentials.SSHAuthSock != "" {
		environment["SSH_AUTH_SOCK"] = c.Credentials.SSHAuthSock
	}
	return process.Command{Argv: argv, Dir: directory, Env: environment}, nil
}

func fixedConfig(credentials Credentials, excludesFile string) []string {
	askPass := falsePath
	ssh := falsePath
	if credentials.AskPass != "" {
		askPass = credentials.AskPass
	}
	if credentials.SSH != "" {
		ssh = credentials.SSH
	}
	return []string{
		"core.excludesFile=" + excludesFile,
		"core.hooksPath=" + nullPath,
		"core.fsmonitor=false",
		"core.useReplaceRefs=false",
		"core.pager=cat",
		"core.editor=" + falsePath,
		"core.askPass=" + askPass,
		"core.sshCommand=" + ssh,
		"sequence.editor=" + falsePath,
		"credential.helper=",
		"protocol.allow=never",
		"fetch.recurseSubmodules=false",
		"submodule.recurse=false",
		"fetch.bundleURI=false",
		"transfer.fsckObjects=true",
		"fetch.fsckObjects=true",
		"receive.fsckObjects=true",
		"diff.external=",
		"interactive.diffFilter=",
		"commit.gpgSign=false",
		"tag.gpgSign=false",
	}
}

func ensureManagedExcludes(home, path string) error {
	homeInfo, err := os.Lstat(home)
	homeOwner, ownerOK := statOwner(homeInfo)
	if err != nil || !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 || homeInfo.Mode().Perm()&0o077 != 0 ||
		!ownerOK || homeOwner != os.Geteuid() {
		return refuse("controlled home", "must be an owner-only real directory")
	}
	if err := validateManagedExcludes(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	temporary, err := os.CreateTemp(home, ".git-excludes-")
	if err != nil {
		return refuse("managed excludes file", "cannot create owner-only content")
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := writeManagedExcludes(temporary); err != nil {
		_ = temporary.Close()
		return refuse("managed excludes file", "cannot persist owner-only content")
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return refuse("managed excludes file", "cannot install owner-only content")
		}
	} else if err := syncDirectory(home); err != nil {
		return refuse("managed excludes file", "cannot persist installation")
	}
	return validateManagedExcludes(path)
}

func writeManagedExcludes(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.WriteString(managedExcludesContent); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}

func validateManagedExcludes(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	owner, ownerOK := statOwner(info)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownerOK || owner != os.Geteuid() {
		return refuse("managed excludes file", "must be an owner-only regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return refuse("managed excludes file", "cannot read managed content")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, int64(len(managedExcludesContent)+1)))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(content) != managedExcludesContent {
		return refuse("managed excludes file", "content differs from Agent Box policy")
	}
	return nil
}

func statOwner(info os.FileInfo) (int, bool) {
	if info == nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	return errors.Join(syncErr, directory.Close())
}

func validateCredentials(credentials Credentials) error {
	for field, value := range map[string]string{
		"askpass executable": credentials.AskPass,
		"ssh executable":     credentials.SSH,
		"ssh agent socket":   credentials.SSHAuthSock,
	} {
		if value != "" {
			if err := validateCanonicalPath(field, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCanonicalPath(field, path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || hasControl(path) {
		return refuse(field, "must be a clean absolute path without control characters")
	}
	return nil
}

func validatePathList(value string) error {
	if value == "" {
		return refuse("PATH", "must contain only clean absolute directories")
	}
	for _, directory := range filepath.SplitList(value) {
		if err := validateCanonicalPath("PATH", directory); err != nil {
			return err
		}
	}
	return nil
}

func rejectAlternates(repository string) error {
	candidates := []string{
		filepath.Join(repository, "objects", "info", "alternates"),
		filepath.Join(repository, ".git", "objects", "info", "alternates"),
	}
	if common, err := linkedCommonDir(repository); err == nil && common != "" {
		candidates = append(candidates, filepath.Join(common, "objects", "info", "alternates"))
	} else if err != nil {
		return refuse("repository", "cannot verify linked-worktree object storage")
	}
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err == nil {
			return refuse("repository", "object alternates are forbidden")
		} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return refuse("repository", "cannot verify object alternates")
		}
	}
	return nil
}

func linkedCommonDir(repository string) (string, error) {
	pointer := filepath.Join(repository, ".git")
	info, err := os.Lstat(pointer)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", nil
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4096 {
		return "", errors.New("invalid linked-worktree pointer")
	}
	content, err := os.ReadFile(pointer)
	if err != nil {
		return "", err
	}
	line := strings.TrimSuffix(string(content), "\n")
	gitDir, found := strings.CutPrefix(line, "gitdir: ")
	if !found || strings.Contains(gitDir, "\n") || validateCanonicalPath("git directory", gitDir) != nil {
		return "", errors.New("invalid linked-worktree pointer")
	}
	commonFile := filepath.Join(gitDir, "commondir")
	commonInfo, err := os.Lstat(commonFile)
	if err != nil || !commonInfo.Mode().IsRegular() || commonInfo.Size() <= 0 || commonInfo.Size() > 4096 {
		return "", errors.New("invalid linked-worktree common directory")
	}
	commonContent, err := os.ReadFile(commonFile)
	if err != nil {
		return "", err
	}
	relative := strings.TrimSuffix(string(commonContent), "\n")
	if relative == "" || strings.Contains(relative, "\n") || strings.IndexByte(relative, 0) >= 0 {
		return "", errors.New("invalid linked-worktree common directory")
	}
	common := filepath.Clean(filepath.Join(gitDir, relative))
	if !filepath.IsAbs(common) {
		return "", errors.New("invalid linked-worktree common directory")
	}
	return common, nil
}

func validateRemoteURL(value string, credentials Credentials) (transport, error) {
	if value == "" || strings.HasPrefix(value, "-") || hasControl(value) || strings.Contains(value, "::") {
		return noTransport, refuse("remote URL", "unsupported or ambiguous syntax")
	}
	parsed, err := url.Parse(value)
	if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "ssh") {
		if parsed.Host == "" || parsed.Hostname() == "" || strings.HasPrefix(parsed.Hostname(), "-") || parsed.Fragment != "" {
			return noTransport, refuse("remote URL", "destination is incomplete or ambiguous")
		}
		if parsed.User != nil {
			if _, hasPassword := parsed.User.Password(); hasPassword {
				return noTransport, refuse("remote URL", "credentials in URLs are forbidden")
			}
		}
		decoded, decodeErr := url.PathUnescape(parsed.EscapedPath())
		if decodeErr != nil || hasControl(decoded) {
			return noTransport, refuse("remote URL", "encoded control characters are forbidden")
		}
		if parsed.Scheme == "https" {
			return httpsTransport, nil
		}
		if credentials.SSH == "" {
			return noTransport, refuse("credentials", "an explicit SSH executable is required")
		}
		return sshTransport, nil
	}
	if validSCPURL(value) {
		if credentials.SSH == "" {
			return noTransport, refuse("credentials", "an explicit SSH executable is required")
		}
		return sshTransport, nil
	}
	return noTransport, refuse("remote URL", "only HTTPS and SSH are supported")
}

func validSCPURL(value string) bool {
	if strings.Contains(value, "://") || strings.Count(value, ":") != 1 {
		return false
	}
	host, path, found := strings.Cut(value, ":")
	if !found || host == "" || path == "" || strings.HasPrefix(path, "-") {
		return false
	}
	for _, r := range host {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("@._-", r) {
			return false
		}
	}
	return !strings.HasPrefix(host, "-") && !strings.Contains(host, "@-")
}

func validateRefspecs(refspecs []string, fetch bool) error {
	if len(refspecs) == 0 {
		return refuse("refspec", "at least one explicit refspec is required")
	}
	for _, refspec := range refspecs {
		if strings.HasPrefix(refspec, "+") || strings.Count(refspec, ":") != 1 || hasControl(refspec) {
			return refuse("refspec", "force, deletion, wildcard, and ambiguous refspecs are forbidden")
		}
		source, destination, _ := strings.Cut(refspec, ":")
		if strings.ContainsAny(refspec, "*") || !validObjectName(source, false) || !validFullRef(destination) {
			return refuse("refspec", "source and destination must be explicit full refs or object IDs")
		}
		if !fetch && !validFullRef(source) {
			return refuse("refspec", "push source must be an explicit full ref")
		}
	}
	return nil
}

func validObjectName(value string, allowHEAD bool) bool {
	if allowHEAD && value == "HEAD" {
		return true
	}
	if len(value) == 40 {
		for _, r := range value {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
		return true
	}
	return validFullRef(value)
}

func validObjectID(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func validBranch(branch string) bool {
	return branch != "" && !strings.HasPrefix(branch, "-") && !hasControl(branch) && len(branch) <= maxRefLength
}

func validFullRef(value string) bool {
	if len(value) < len("refs/x") || len(value) > maxRefLength || !strings.HasPrefix(value, "refs/") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.HasSuffix(value, ".lock") ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.Contains(value, "//") ||
		strings.ContainsAny(value, " ~^:?*[\\") || hasControl(value) {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") {
			return false
		}
	}
	return true
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func refuse(field, reason string) error {
	return &PolicyError{Field: field, Reason: reason}
}
