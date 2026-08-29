// Package gitstore materializes self-contained Box-private Git object stores.
package gitstore

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
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

// Git is the mutation subset of the hardened Machine Git profile needed to
// seed one private store.
type Git interface {
	Status(context.Context, string) (machinegit.Result, error)
	ResolveCommit(context.Context, string, string) (machinegit.Result, error)
	CheckBranch(context.Context, string, string) (machinegit.Result, error)
	LookupBranch(context.Context, string, string) (machinegit.Result, error)
	CloneBare(context.Context, string, string, string) (machinegit.Result, error)
	SetPrivateBase(context.Context, string, string) (machinegit.Result, error)
	ConfigurePrivateStore(context.Context, string) error
}

// Materializer owns creation of one store. Worktree creation is intentionally
// a later operation so dirty and collision refusals happen first.
type Materializer struct {
	Git Git
}

// Record is Machine-owned state. PublishDestination must be persisted by the
// control plane; the Box-writable Git config is never its source of truth.
type Record struct {
	Repository         string
	GitCommonDir       string
	Identity           ownership.FileIdentity
	BaseCommit         string
	PublishDestination repository.Destination
}

// BranchConflictError reports that the generated task branch already exists in
// the source and therefore cannot be created unambiguously in the Box.
type BranchConflictError struct {
	Repository string
	Branch     string
}

func (e *BranchConflictError) Error() string {
	return "generated branch already exists for repository: " + e.Repository
}

func (e *BranchConflictError) ErrorCode() contract.ErrorCode { return contract.CodeBranchConflict }

func (e *BranchConflictError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// StoreConflictError refuses to adopt or overwrite an unproven destination.
type StoreConflictError struct {
	Repository string
}

func (e *StoreConflictError) Error() string {
	return "private Git store destination already exists for repository: " + e.Repository
}

func (e *StoreConflictError) ErrorCode() contract.ErrorCode { return contract.CodeOwnershipConflict }

func (e *StoreConflictError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// Materialize creates a complete private store or removes every resource this
// call created. Resolved facts and the digest-protected Plan must agree exactly.
func (m Materializer) Materialize(ctx context.Context, facts repository.Facts, immutable contract.Plan) (Record, error) {
	if m.Git == nil {
		return Record{}, errors.New("materialize private Git store: Git adapter is required")
	}
	if err := validateInput(facts, immutable); err != nil {
		return Record{}, err
	}

	status, err := m.Git.Status(ctx, facts.SourceRoot)
	if err != nil {
		return Record{}, fmt.Errorf("materialize private Git store %s: re-observe source status: %w", facts.Name, err)
	}
	if err := repository.CheckClean(facts.Name, status.Stdout); err != nil {
		return Record{}, fmt.Errorf("materialize private Git store %s: %w", facts.Name, err)
	}

	commitResult, err := m.Git.ResolveCommit(ctx, facts.SourceRoot, facts.BaseCommit)
	if err != nil {
		return Record{}, fmt.Errorf("materialize private Git store %s: re-observe base commit: %w", facts.Name, err)
	}
	commit, err := fullCommit(commitResult.Stdout)
	if err != nil || commit != facts.BaseCommit {
		return Record{}, fmt.Errorf("materialize private Git store %s: planned base commit no longer resolves exactly", facts.Name)
	}
	checkedBranch, err := m.Git.CheckBranch(ctx, facts.SourceRoot, facts.Branch)
	if err != nil {
		return Record{}, fmt.Errorf("materialize private Git store %s: revalidate branch: %w", facts.Name, err)
	}
	if trimSingleLine(checkedBranch.Stdout) != facts.Branch {
		return Record{}, fmt.Errorf("materialize private Git store %s: Git normalized the planned branch", facts.Name)
	}
	if _, err := m.Git.LookupBranch(ctx, facts.SourceRoot, facts.Branch); err == nil {
		return Record{}, &BranchConflictError{Repository: facts.Name, Branch: facts.Branch}
	} else if !missingBranch(err) {
		return Record{}, fmt.Errorf("materialize private Git store %s: inspect branch collision: %w", facts.Name, err)
	}

	parent := filepath.Dir(facts.GitStorePath)
	createdParents, err := makePrivateParents(parent)
	if err != nil {
		return Record{}, fmt.Errorf("materialize private Git store %s: create parent: %w", facts.Name, err)
	}
	cleanupParents := func() error { return removeEmptyParents(createdParents) }

	if err := os.Mkdir(facts.GitStorePath, 0o700); err != nil {
		cleanupErr := cleanupParents()
		if errors.Is(err, os.ErrExist) {
			return Record{}, errors.Join(&StoreConflictError{Repository: facts.Name}, cleanupErr)
		}
		return Record{}, errors.Join(fmt.Errorf("materialize private Git store %s: claim destination: %w", facts.Name, err), cleanupErr)
	}
	owned, err := os.Lstat(facts.GitStorePath)
	if err != nil {
		return Record{}, errors.Join(fmt.Errorf("materialize private Git store %s: inspect claimed destination: %w", facts.Name, err), cleanupOwned(facts.GitStorePath, nil), cleanupParents())
	}
	fail := func(applyErr error) (Record, error) {
		return Record{}, errors.Join(applyErr, cleanupOwned(facts.GitStorePath, owned), cleanupParents())
	}

	if _, err := m.Git.CloneBare(ctx, parent, facts.SourceRoot, facts.GitStorePath); err != nil {
		return fail(fmt.Errorf("materialize private Git store %s: seed objects: %w", facts.Name, err))
	}
	if _, err := m.Git.SetPrivateBase(ctx, facts.GitStorePath, facts.BaseCommit); err != nil {
		return fail(fmt.Errorf("materialize private Git store %s: pin base commit: %w", facts.Name, err))
	}
	if err := m.Git.ConfigurePrivateStore(ctx, facts.GitStorePath); err != nil {
		return fail(fmt.Errorf("materialize private Git store %s: configure store: %w", facts.Name, err))
	}
	base, err := m.Git.ResolveCommit(ctx, facts.GitStorePath, "refs/abox/base")
	if err != nil || trimSingleLine(base.Stdout) != facts.BaseCommit {
		if err == nil {
			err = errors.New("private base ref differs from Plan")
		}
		return fail(fmt.Errorf("materialize private Git store %s: verify base commit: %w", facts.Name, err))
	}

	identity, err := ownership.InspectPath(facts.GitStorePath)
	if err != nil {
		return fail(fmt.Errorf("materialize private Git store %s: record identity: %w", facts.Name, err))
	}

	return Record{
		Repository: facts.Name, GitCommonDir: facts.GitStorePath, BaseCommit: facts.BaseCommit,
		Identity: identity, PublishDestination: facts.PublishDestination,
	}, nil
}

func validateInput(facts repository.Facts, immutable contract.Plan) error {
	if _, err := plan.CanonicalJSON(immutable); err != nil {
		return fmt.Errorf("materialize private Git store: immutable Plan: %w", err)
	}
	if len(immutable.Repositories) != 1 {
		return errors.New("materialize private Git store: Plan must contain exactly one repository")
	}
	planned := immutable.Repositories[0]
	if !facts.Clean || facts.Name == "" || planned.Name != facts.Name ||
		planned.SourcePath != facts.SourceRoot || planned.BaseCommit != facts.BaseCommit ||
		planned.Branch != facts.Branch || planned.WorktreePath != facts.WorktreePath ||
		planned.GitCommonDir != facts.GitStorePath {
		return errors.New("materialize private Git store: resolved facts do not match immutable Plan")
	}
	if facts.PublishDestination.Kind != repository.DestinationLocal || facts.PublishDestination.URL != facts.SourceRoot {
		return errors.New("materialize private Git store: trusted local publish destination does not match source")
	}
	return nil
}

func fullCommit(output []byte) (string, error) {
	value := trimSingleLine(output)
	if len(value) != 40 || strings.Trim(value, "0123456789abcdef") != "" {
		return "", errors.New("Git did not return a full lowercase commit ID")
	}
	return value, nil
}

func trimSingleLine(output []byte) string {
	return string(bytes.TrimSuffix(output, []byte("\n")))
}

func missingBranch(err error) bool {
	return errors.Is(err, machinegit.ErrBranchNotFound)
}

func makePrivateParents(path string) ([]string, error) {
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() {
				return nil, errors.New("parent component is not a directory")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		missing = append(missing, current)
		next := filepath.Dir(current)
		if next == current {
			return nil, errors.New("no existing parent directory")
		}
	}
	slices.Reverse(missing)
	created := make([]string, 0, len(missing))
	for _, directory := range missing {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return nil, errors.Join(err, removeEmptyParents(created))
		}
		created = append(created, directory)
	}
	return created, nil
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

func cleanupOwned(path string, owned os.FileInfo) error {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private Git store for cleanup: %w", err)
	}
	if owned == nil || !os.SameFile(owned, current) {
		return errors.New("refuse private Git store cleanup: destination identity changed")
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove failed private Git store: %w", err)
	}
	return nil
}
