// Package worktree manages Box-private linked Git worktrees.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

const branchPrefix = "refs/heads/"

// Git is the narrow hardened Machine Git surface used by Manager.
type Git interface {
	UsesRelativeWorktrees(context.Context, string) (bool, error)
	WorktreeList(context.Context, string) (machinegit.Result, error)
	Gitlinks(context.Context, string, string) (machinegit.Result, error)
	LookupBranch(context.Context, string, string) (machinegit.Result, error)
	AddWorktree(context.Context, string, string, string, string) (machinegit.Result, error)
	LockWorktree(context.Context, string, string, string) (machinegit.Result, error)
	UnlockWorktree(context.Context, string, string) (machinegit.Result, error)
	WorktreeCommonDir(context.Context, string) (machinegit.Result, error)
	Status(context.Context, string) (machinegit.Result, error)
	IgnoredFiles(context.Context, string) (machinegit.Result, error)
	RemoveWorktree(context.Context, string, string) (machinegit.Result, error)
	RemoveOwnedWorktree(context.Context, string, string) (machinegit.Result, error)
	DeleteBranch(context.Context, string, string, string) (machinegit.Result, error)
}

// Manager creates and observes worktrees below one explicit Box workspace root.
type Manager struct {
	Git           Git
	WorkspaceRoot string
}

// Observation is the complete Machine-side identity and mutable status of one
// registered linked worktree.
type Observation struct {
	Repository       string
	CanonicalPath    string
	CanonicalCommon  string
	Branch           string
	Commit           string
	LockReason       string
	Registered       bool
	BranchPresent    bool
	Dirty            bool
	ChangedPaths     []string
	IgnoredPaths     []string
	WorktreeIdentity ownership.FileIdentity
	StoreIdentity    ownership.FileIdentity
}

// ConflictError refuses ambiguous or drifted worktree ownership without reset.
type ConflictError struct {
	Repository string
	Reason     string
}

func (e *ConflictError) Error() string {
	return "linked worktree ownership conflict for repository " + e.Repository + ": " + e.Reason
}

func (e *ConflictError) ErrorCode() contract.ErrorCode { return contract.CodeOwnershipConflict }

func (e *ConflictError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// BranchConflictError reports an existing generated branch that cannot be
// safely adopted or reset.
type BranchConflictError struct {
	Repository string
	Branch     string
}

func (e *BranchConflictError) Error() string {
	return "generated worktree branch already exists for repository: " + e.Repository
}

func (e *BranchConflictError) ErrorCode() contract.ErrorCode { return contract.CodeBranchConflict }

func (e *BranchConflictError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// RelativeWorktreesError reports metadata that violates the invariant-path
// compatibility contract.
type RelativeWorktreesError struct{ Repository string }

func (e *RelativeWorktreesError) Error() string {
	return "relative worktree metadata is unsupported for repository: " + e.Repository
}

func (e *RelativeWorktreesError) ErrorCode() contract.ErrorCode {
	return contract.CodeFeatureNotSupported
}

func (e *RelativeWorktreesError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// ActiveSubmodulesError reports gitlinks in the active commit. Paths are kept
// out of Error so JSON and human renderers can escape them appropriately.
type ActiveSubmodulesError struct {
	Repository string
	paths      []string
}

func (e *ActiveSubmodulesError) Error() string {
	return "active submodules are unsupported for repository: " + e.Repository
}

func (e *ActiveSubmodulesError) Paths() []string { return append([]string(nil), e.paths...) }

func (e *ActiveSubmodulesError) ErrorCode() contract.ErrorCode {
	return contract.CodeFeatureNotSupported
}

func (e *ActiveSubmodulesError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// Observe performs the complete read-only ownership and loss preflight for an
// existing linked worktree. It never creates, locks, unlocks, or removes state.
func (m Manager) Observe(ctx context.Context, facts repository.Facts, immutable contract.Plan, store gitstore.Record) (Observation, error) {
	if m.Git == nil {
		return Observation{}, errors.New("observe linked worktree: Git adapter is required")
	}
	if err := m.validateInput(facts, immutable, store); err != nil {
		return Observation{}, err
	}
	if err := verifyStoreIdentity(store); err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	if relative, err := m.Git.UsesRelativeWorktrees(ctx, store.GitCommonDir); err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s: inspect relative-worktree mode: %w", facts.Name, err)
	} else if relative {
		return Observation{}, &RelativeWorktreesError{Repository: facts.Name}
	}

	registrations, err := m.registrations(ctx, store.GitCommonDir)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s: observe registrations: %w", facts.Name, err)
	}
	registered, exists, err := selectRegistration(registrations, facts.WorktreePath, facts.Branch)
	if err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	if !exists {
		return Observation{}, conflict(facts.Name, "expected Git registration is absent")
	}
	return m.observe(ctx, facts, store, registered, ownerReason(immutable.BoxID))
}

// ObserveRemoval returns the complete live worktree state, including the
// interruption state where Git removed the worktree but its generated branch
// remains. An absent worktree and absent branch is a successful terminal state;
// an unregistered path is never adopted or removed.
func (m Manager) ObserveRemoval(ctx context.Context, facts repository.Facts, immutable contract.Plan, store gitstore.Record) (Observation, error) {
	if m.Git == nil {
		return Observation{}, errors.New("observe linked worktree removal: Git adapter is required")
	}
	if err := m.validateInput(facts, immutable, store); err != nil {
		return Observation{}, err
	}
	if err := verifyStoreIdentity(store); err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	if relative, err := m.Git.UsesRelativeWorktrees(ctx, store.GitCommonDir); err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree removal %s: inspect relative-worktree mode: %w", facts.Name, err)
	} else if relative {
		return Observation{}, &RelativeWorktreesError{Repository: facts.Name}
	}
	registrations, err := m.registrations(ctx, store.GitCommonDir)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree removal %s: observe registrations: %w", facts.Name, err)
	}
	registered, exists, err := selectRegistration(registrations, facts.WorktreePath, facts.Branch)
	if err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	if exists {
		return m.observe(ctx, facts, store, registered, ownerReason(immutable.BoxID))
	}
	if _, err := os.Lstat(facts.WorktreePath); err == nil {
		return Observation{}, conflict(facts.Name, "planned path exists without the expected Git registration")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Observation{}, fmt.Errorf("observe linked worktree removal %s: inspect absent path: %w", facts.Name, err)
	}
	result, err := m.Git.LookupBranch(ctx, store.GitCommonDir, facts.Branch)
	if errors.Is(err, machinegit.ErrBranchNotFound) {
		return Observation{
			Repository: facts.Name, CanonicalPath: facts.WorktreePath, CanonicalCommon: store.GitCommonDir,
			Branch: facts.Branch, StoreIdentity: store.Identity,
		}, nil
	}
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree removal %s: inspect generated branch: %w", facts.Name, err)
	}
	commit := trimLine(result.Stdout)
	if !validCommit(commit) {
		return Observation{}, conflict(facts.Name, "generated branch has an invalid commit")
	}
	return Observation{
		Repository: facts.Name, CanonicalPath: facts.WorktreePath, CanonicalCommon: store.GitCommonDir,
		Branch: facts.Branch, Commit: commit, BranchPresent: true, StoreIdentity: store.Identity,
	}, nil
}

// Remove destroys only the exact freshly observed Agent Box registration and
// generated branch. The branch-only state is deliberately supported so a retry
// converges after interruption between Git's worktree and ref mutations.
func (m Manager) Remove(ctx context.Context, facts repository.Facts, immutable contract.Plan, store gitstore.Record, expected Observation) error {
	fresh, err := m.ObserveRemoval(ctx, facts, immutable, store)
	if err != nil {
		return err
	}
	if !equalObservation(fresh, expected) {
		return conflict(facts.Name, "worktree removal evidence changed")
	}
	if !fresh.Registered && !fresh.BranchPresent {
		return nil
	}
	if fresh.Registered {
		if _, err := m.Git.RemoveOwnedWorktree(ctx, store.GitCommonDir, facts.WorktreePath); err != nil {
			return fmt.Errorf("remove linked worktree %s: %w", facts.Name, err)
		}
		fresh, err = m.ObserveRemoval(ctx, facts, immutable, store)
		if err != nil {
			return fmt.Errorf("verify linked worktree removal %s: %w", facts.Name, err)
		}
		if fresh.Registered || !fresh.BranchPresent || fresh.Commit != expected.Commit {
			return conflict(facts.Name, "worktree removal did not leave the expected generated branch")
		}
	}
	if _, err := m.Git.DeleteBranch(ctx, store.GitCommonDir, facts.Branch, fresh.Commit); err != nil {
		return fmt.Errorf("remove linked worktree branch %s: %w", facts.Name, err)
	}
	terminal, err := m.ObserveRemoval(ctx, facts, immutable, store)
	if err != nil {
		return fmt.Errorf("verify linked worktree branch removal %s: %w", facts.Name, err)
	}
	if terminal.Registered || terminal.BranchPresent {
		return conflict(facts.Name, "generated branch remains after removal")
	}
	return nil
}

// Ensure returns a fresh observation of a matching worktree or creates, locks,
// and verifies it. It never resets or adopts conflicting state.
func (m Manager) Ensure(ctx context.Context, facts repository.Facts, immutable contract.Plan, store gitstore.Record) (Observation, error) {
	if m.Git == nil {
		return Observation{}, errors.New("ensure linked worktree: Git adapter is required")
	}
	if err := m.validateInput(facts, immutable, store); err != nil {
		return Observation{}, err
	}
	if err := verifyStoreIdentity(store); err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	lockReason := ownerReason(immutable.BoxID)
	if relative, err := m.Git.UsesRelativeWorktrees(ctx, store.GitCommonDir); err != nil {
		return Observation{}, fmt.Errorf("ensure linked worktree %s: inspect relative-worktree mode: %w", facts.Name, err)
	} else if relative {
		return Observation{}, &RelativeWorktreesError{Repository: facts.Name}
	}

	registrations, err := m.registrations(ctx, store.GitCommonDir)
	if err != nil {
		return Observation{}, fmt.Errorf("ensure linked worktree %s: observe registrations: %w", facts.Name, err)
	}
	registered, exists, err := selectRegistration(registrations, facts.WorktreePath, facts.Branch)
	if err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	if exists {
		observation, observeErr := m.observe(ctx, facts, store, registered, lockReason)
		if observeErr != nil {
			return Observation{}, observeErr
		}
		if err := m.rejectGitlinks(ctx, facts.Name, store.GitCommonDir, observation.Commit); err != nil {
			return Observation{}, err
		}
		return observation, nil
	}

	if _, err := os.Lstat(facts.WorktreePath); err == nil {
		return Observation{}, conflict(facts.Name, "planned path exists without the expected Git registration")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Observation{}, fmt.Errorf("ensure linked worktree %s: inspect destination: %w", facts.Name, err)
	}
	if _, err := m.Git.LookupBranch(ctx, store.GitCommonDir, facts.Branch); err == nil {
		return Observation{}, &BranchConflictError{Repository: facts.Name, Branch: facts.Branch}
	} else if !errors.Is(err, machinegit.ErrBranchNotFound) {
		return Observation{}, fmt.Errorf("ensure linked worktree %s: inspect generated branch: %w", facts.Name, err)
	}
	if err := m.rejectGitlinks(ctx, facts.Name, store.GitCommonDir, facts.BaseCommit); err != nil {
		return Observation{}, err
	}

	createdParents, err := makeWorkspaceParents(m.WorkspaceRoot, filepath.Dir(facts.WorktreePath))
	if err != nil {
		return Observation{}, fmt.Errorf("ensure linked worktree %s: create parent: %w", facts.Name, err)
	}
	cleanupParents := func() error { return removeEmptyParents(createdParents) }
	if _, err := m.Git.AddWorktree(ctx, store.GitCommonDir, facts.Branch, facts.WorktreePath, facts.BaseCommit); err != nil {
		return Observation{}, errors.Join(fmt.Errorf("ensure linked worktree %s: create: %w", facts.Name, err), cleanupParents())
	}
	createdIdentity, err := ownership.InspectPath(facts.WorktreePath)
	if err != nil {
		return Observation{}, errors.Join(
			fmt.Errorf("ensure linked worktree %s: record created identity: %w", facts.Name, err),
			m.cleanupCreated(ctx, facts, store, ownership.FileIdentity{}, lockReason, false), cleanupParents(),
		)
	}
	if _, err := m.Git.LockWorktree(ctx, store.GitCommonDir, facts.WorktreePath, lockReason); err != nil {
		return Observation{}, errors.Join(
			fmt.Errorf("ensure linked worktree %s: lock: %w", facts.Name, err),
			m.cleanupCreated(ctx, facts, store, createdIdentity, lockReason, false), cleanupParents(),
		)
	}

	registrations, err = m.registrations(ctx, store.GitCommonDir)
	if err == nil {
		registered, exists, err = selectRegistration(registrations, facts.WorktreePath, facts.Branch)
		if err == nil && !exists {
			err = errors.New("created worktree is not registered")
		}
	}
	if err == nil {
		var observation Observation
		observation, err = m.observe(ctx, facts, store, registered, lockReason)
		if err == nil && observation.Commit != facts.BaseCommit {
			err = errors.New("created worktree commit differs from immutable Plan")
		}
		if err == nil {
			return observation, nil
		}
	}
	return Observation{}, errors.Join(
		fmt.Errorf("ensure linked worktree %s: verify creation: %w", facts.Name, err),
		m.cleanupCreated(ctx, facts, store, createdIdentity, lockReason, true), cleanupParents(),
	)
}

func (m Manager) validateInput(facts repository.Facts, immutable contract.Plan, store gitstore.Record) error {
	if _, err := plan.CanonicalJSON(immutable); err != nil {
		return fmt.Errorf("ensure linked worktree: immutable Plan: %w", err)
	}
	if len(immutable.Repositories) != 1 {
		return errors.New("ensure linked worktree: Plan must contain exactly one repository")
	}
	planned := immutable.Repositories[0]
	if facts.Name == "" || planned.Name != facts.Name || planned.SourcePath != facts.SourceRoot ||
		planned.BaseCommit != facts.BaseCommit || planned.Branch != facts.Branch ||
		planned.WorktreePath != facts.WorktreePath || planned.GitCommonDir != facts.GitStorePath {
		return errors.New("ensure linked worktree: resolved facts do not match immutable Plan")
	}
	if store.Repository != facts.Name || store.GitCommonDir != facts.GitStorePath ||
		store.BaseCommit != facts.BaseCommit || store.PublishDestination != facts.PublishDestination ||
		store.Identity == (ownership.FileIdentity{}) {
		return errors.New("ensure linked worktree: private-store record does not match resolved facts")
	}
	if !cleanAbsolute(m.WorkspaceRoot) || !strictlyWithin(m.WorkspaceRoot, facts.WorktreePath) {
		return errors.New("ensure linked worktree: worktree path is outside the canonical workspace root")
	}
	if immutable.BoxID == "" || strings.IndexByte(immutable.BoxID, 0) >= 0 {
		return errors.New("ensure linked worktree: Box ID is invalid")
	}
	return nil
}

func (m Manager) rejectGitlinks(ctx context.Context, name, store, commit string) error {
	result, err := m.Git.Gitlinks(ctx, store, commit)
	if err != nil {
		return fmt.Errorf("ensure linked worktree %s: inspect submodules: %w", name, err)
	}
	paths, err := parseGitlinks(result.Stdout)
	if err != nil {
		return fmt.Errorf("ensure linked worktree %s: parse submodules: %w", name, err)
	}
	if len(paths) != 0 {
		return &ActiveSubmodulesError{Repository: name, paths: paths}
	}
	return nil
}

func (m Manager) registrations(ctx context.Context, store string) ([]registration, error) {
	result, err := m.Git.WorktreeList(ctx, store)
	if err != nil {
		return nil, err
	}
	return parseRegistrations(result.Stdout)
}

func (m Manager) observe(ctx context.Context, facts repository.Facts, store gitstore.Record, registered registration, lockReason string) (Observation, error) {
	if registered.Branch != branchPrefix+facts.Branch {
		return Observation{}, conflict(facts.Name, "registered branch differs from immutable Plan")
	}
	if registered.LockReason != lockReason {
		return Observation{}, conflict(facts.Name, "worktree lock reason differs from Box ownership")
	}
	worktreeIdentity, err := ownership.InspectPath(facts.WorktreePath)
	if err != nil {
		return Observation{}, conflict(facts.Name, "registered worktree path cannot be observed")
	}
	if err := verifyStoreIdentity(store); err != nil {
		return Observation{}, conflict(facts.Name, err.Error())
	}
	common, err := m.Git.WorktreeCommonDir(ctx, facts.WorktreePath)
	if err != nil {
		return Observation{}, conflict(facts.Name, "worktree common directory cannot be verified")
	}
	commonDir := trimLine(common.Stdout)
	if !cleanAbsolute(commonDir) || commonDir != store.GitCommonDir {
		return Observation{}, conflict(facts.Name, "worktree common directory differs from private store")
	}
	status, err := m.Git.Status(ctx, facts.WorktreePath)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s status: %w", facts.Name, err)
	}
	changed, err := repository.ChangedPaths(status.Stdout)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s status: %w", facts.Name, err)
	}
	ignoredResult, err := m.Git.IgnoredFiles(ctx, facts.WorktreePath)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s ignored files: %w", facts.Name, err)
	}
	ignored, err := parseNULPaths(ignoredResult.Stdout)
	if err != nil {
		return Observation{}, fmt.Errorf("observe linked worktree %s ignored files: %w", facts.Name, err)
	}
	return Observation{
		Repository: facts.Name, CanonicalPath: facts.WorktreePath, CanonicalCommon: commonDir,
		Branch: facts.Branch, Commit: registered.Head, LockReason: registered.LockReason,
		Registered: true, BranchPresent: true, Dirty: len(changed) != 0 || len(ignored) != 0,
		ChangedPaths: changed, IgnoredPaths: ignored,
		WorktreeIdentity: worktreeIdentity, StoreIdentity: store.Identity,
	}, nil
}

func equalObservation(left, right Observation) bool {
	return left.Repository == right.Repository && left.CanonicalPath == right.CanonicalPath &&
		left.CanonicalCommon == right.CanonicalCommon && left.Branch == right.Branch &&
		left.Commit == right.Commit && left.LockReason == right.LockReason &&
		left.Registered == right.Registered && left.BranchPresent == right.BranchPresent &&
		left.Dirty == right.Dirty && slices.Equal(left.ChangedPaths, right.ChangedPaths) &&
		slices.Equal(left.IgnoredPaths, right.IgnoredPaths) &&
		left.WorktreeIdentity == right.WorktreeIdentity && left.StoreIdentity == right.StoreIdentity
}

func parseNULPaths(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, errors.New("ignored-file list is not NUL terminated")
	}
	fields := bytes.Split(output[:len(output)-1], []byte{0})
	paths := make([]string, len(fields))
	for index, field := range fields {
		if len(field) == 0 {
			return nil, errors.New("ignored-file list contains an empty path")
		}
		paths[index] = string(field)
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

func ownerReason(boxID string) string { return "owned by Agent Box " + boxID }

func (m Manager) cleanupCreated(ctx context.Context, facts repository.Facts, store gitstore.Record, identity ownership.FileIdentity, lockReason string, locked bool) error {
	if identity == (ownership.FileIdentity{}) {
		return errors.New("refuse failed-worktree cleanup: created path identity is unavailable")
	}
	live, err := ownership.InspectPath(facts.WorktreePath)
	if err != nil || live != identity {
		return errors.New("refuse failed-worktree cleanup: worktree path identity changed")
	}
	registrations, err := m.registrations(ctx, store.GitCommonDir)
	if err != nil {
		return fmt.Errorf("refuse failed-worktree cleanup: observe registration: %w", err)
	}
	registered, exists, err := selectRegistration(registrations, facts.WorktreePath, facts.Branch)
	if err != nil || !exists || registered.Head != facts.BaseCommit || registered.Branch != branchPrefix+facts.Branch {
		return errors.New("refuse failed-worktree cleanup: worktree registration changed")
	}
	common, err := m.Git.WorktreeCommonDir(ctx, facts.WorktreePath)
	if err != nil || trimLine(common.Stdout) != store.GitCommonDir {
		return errors.New("refuse failed-worktree cleanup: common directory changed")
	}
	if locked {
		if registered.LockReason != lockReason {
			return errors.New("refuse failed-worktree cleanup: expected worktree lock differs")
		}
		if _, err := m.Git.UnlockWorktree(ctx, store.GitCommonDir, facts.WorktreePath); err != nil {
			return fmt.Errorf("unlock failed worktree: %w", err)
		}
	} else if registered.LockReason != "" {
		return errors.New("refuse failed-worktree cleanup: unexpected worktree lock appeared")
	}
	if _, err := m.Git.RemoveWorktree(ctx, store.GitCommonDir, facts.WorktreePath); err != nil {
		return fmt.Errorf("remove failed worktree: %w", err)
	}
	if _, err := m.Git.DeleteBranch(ctx, store.GitCommonDir, facts.Branch, facts.BaseCommit); err != nil {
		return fmt.Errorf("remove failed worktree branch: %w", err)
	}
	return nil
}

func verifyStoreIdentity(store gitstore.Record) error {
	identity, err := ownership.InspectPath(store.GitCommonDir)
	if err != nil {
		return errors.New("private store cannot be observed")
	}
	if identity != store.Identity {
		return errors.New("private-store filesystem identity differs from its record")
	}
	return nil
}

type registration struct {
	Path       string
	Head       string
	Branch     string
	LockReason string
	Bare       bool
}

func parseRegistrations(output []byte) ([]registration, error) {
	if len(output) == 0 || output[len(output)-1] != 0 {
		return nil, errors.New("worktree porcelain is not NUL terminated")
	}
	fields := bytes.Split(output[:len(output)-1], []byte{0})
	var registrations []registration
	var current *registration
	seen := map[string]bool{}
	finish := func() error {
		if current == nil {
			return nil
		}
		if current.Path == "" {
			return errors.New("worktree porcelain record has no path")
		}
		if !current.Bare && !validCommit(current.Head) {
			return errors.New("worktree porcelain record has an invalid HEAD")
		}
		registrations = append(registrations, *current)
		current = nil
		seen = map[string]bool{}
		return nil
	}
	for _, raw := range fields {
		field := string(raw)
		if field == "" {
			if err := finish(); err != nil {
				return nil, err
			}
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			if current != nil {
				return nil, errors.New("worktree porcelain record has no separator")
			}
			if !cleanAbsolute(value) {
				return nil, errors.New("worktree porcelain path is not canonical")
			}
			current = &registration{Path: value}
			seen = map[string]bool{"worktree": true}
			continue
		}
		if current == nil {
			return nil, errors.New("worktree porcelain field precedes its path")
		}
		if seen[key] {
			return nil, errors.New("worktree porcelain contains a duplicate field")
		}
		seen[key] = true
		switch key {
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = value
		case "bare":
			current.Bare = true
		case "locked":
			current.LockReason = value
		case "detached", "prunable":
			// Known fields not needed by the v0.1 ownership record.
		default:
			// Porcelain permits forward-compatible fields; ignore their values.
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	if len(registrations) == 0 {
		return nil, errors.New("worktree porcelain contains no registrations")
	}
	return registrations, nil
}

func selectRegistration(registrations []registration, path, branch string) (registration, bool, error) {
	fullBranch := branchPrefix + branch
	var selected registration
	pathCount := 0
	for _, current := range registrations {
		if current.Path == path {
			selected = current
			pathCount++
		}
		if current.Branch == fullBranch && current.Path != path {
			return registration{}, false, errors.New("generated branch is registered to a different worktree")
		}
	}
	if pathCount > 1 {
		return registration{}, false, errors.New("planned path appears more than once in Git registration")
	}
	return selected, pathCount == 1, nil
}

func parseGitlinks(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, errors.New("ls-tree output is not NUL terminated")
	}
	var paths []string
	for _, raw := range bytes.Split(output[:len(output)-1], []byte{0}) {
		metadata, path, found := bytes.Cut(raw, []byte{'\t'})
		fields := bytes.Fields(metadata)
		if !found || len(fields) != 3 || len(path) == 0 {
			return nil, errors.New("malformed ls-tree record")
		}
		if string(fields[0]) == "160000" {
			paths = append(paths, string(path))
		}
	}
	return paths, nil
}

func makeWorkspaceParents(root, target string) ([]string, error) {
	if !cleanAbsolute(root) || !cleanAbsolute(target) || target != root && !strictlyWithin(root, target) {
		return nil, errors.New("parent path is outside workspace root")
	}
	base := filepath.Dir(root)
	if err := requireRealDirectory(base); err != nil {
		return nil, err
	}
	components := []string{root}
	if target != root {
		relative, err := filepath.Rel(root, target)
		if err != nil {
			return nil, err
		}
		current := root
		for _, component := range strings.Split(relative, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			components = append(components, current)
		}
	}
	var created []string
	for _, directory := range components {
		if err := requireRealDirectory(directory); err == nil {
			continue
		} else if _, statErr := os.Lstat(directory); !errors.Is(statErr, os.ErrNotExist) {
			return nil, errors.Join(err, removeEmptyParents(created))
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			return nil, errors.Join(err, removeEmptyParents(created))
		}
		created = append(created, directory)
	}
	return created, nil
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path component is not a real directory")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return errors.New("path component is not canonical")
	}
	return nil
}

func removeEmptyParents(created []string) error {
	var cleanupErr error
	for index := len(created) - 1; index >= 0; index-- {
		if err := os.Remove(created[index]); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func conflict(repository, reason string) error {
	return &ConflictError{Repository: repository, Reason: reason}
}

func cleanAbsolute(path string) bool {
	return path != "" && strings.IndexByte(path, 0) < 0 && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func strictlyWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validCommit(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}

func trimLine(output []byte) string { return string(bytes.TrimSuffix(output, []byte("\n"))) }
