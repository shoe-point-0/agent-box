// Package repository resolves repository intent without materializing it.
package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
)

const taskTemplate = "{{ box.task }}"

// Git is the read-only subset of the hardened Machine Git profile used during
// repository resolution.
type Git interface {
	TopLevel(context.Context, string) (machinegit.Result, error)
	Status(context.Context, string) (machinegit.Result, error)
	ResolveCommit(context.Context, string, string) (machinegit.Result, error)
	CheckBranch(context.Context, string, string) (machinegit.Result, error)
}

// DestinationKind describes where publication reachability will later be
// checked. Milestone 1 supports only independently canonicalized local roots.
type DestinationKind string

const DestinationLocal DestinationKind = "local"

// Destination is trusted Plan input and is never read from Box-writable Git
// configuration.
type Destination struct {
	Kind DestinationKind
	URL  string
}

// Facts is the immutable, read-only repository input consumed by Plan
// compilation and later materialization operations.
type Facts struct {
	Name               string
	SourceRoot         string
	Clean              bool
	BaseCommit         string
	Branch             string
	WorktreePath       string
	GitStorePath       string
	PublishDestination Destination
}

// DirtySourceError refuses a source with tracked or untracked changes. Paths
// are copied from Git's NUL-delimited porcelain and are not interpolated into
// Error, so diagnostics can render them safely for their output format.
type DirtySourceError struct {
	Repository string
	paths      []string
}

func (e *DirtySourceError) Error() string { return "repository source is dirty: " + e.Repository }

func (e *DirtySourceError) ChangedPaths() []string { return append([]string(nil), e.paths...) }

func (e *DirtySourceError) ErrorCode() contract.ErrorCode { return contract.CodeGitDirtySource }

func (e *DirtySourceError) ExitCode() int { return contract.ExitCodeFor(e.ErrorCode()) }

// CheckClean parses script-facing porcelain and returns a typed safety refusal
// when any tracked or untracked source path changed.
func CheckClean(name string, status []byte) error {
	changed, err := parseStatusPaths(status)
	if err != nil {
		return err
	}
	if len(changed) != 0 {
		return &DirtySourceError{Repository: name, paths: changed}
	}
	return nil
}

// Resolver combines read-only Git observations with centralized path
// construction. It has no state, filesystem, Lima, Compose, or port writer.
type Resolver struct {
	Git   Git
	Paths *paths.Provider
}

// Resolve returns no partial facts on failure.
func (r Resolver) Resolve(ctx context.Context, workspace *manifest.Workspace, boxID identity.BoxID, taskKey string) ([]Facts, error) {
	if workspace == nil {
		return nil, errors.New("resolve repositories: workspace is required")
	}
	if r.Git == nil {
		return nil, errors.New("resolve repositories: Git observer is required")
	}
	if r.Paths == nil {
		return nil, errors.New("resolve repositories: path provider is required")
	}
	if err := identity.ValidateBoxID(boxID); err != nil {
		return nil, fmt.Errorf("resolve repositories: %w", err)
	}
	if err := identity.ValidateTaskKey(taskKey); err != nil {
		return nil, fmt.Errorf("resolve repositories: %w", err)
	}
	if err := manifest.CheckMilestone1Executable(workspace); err != nil {
		return nil, fmt.Errorf("resolve repositories: %w", err)
	}

	resolved := make([]Facts, 0, len(workspace.Spec.Repositories))
	for index := range workspace.Spec.Repositories {
		facts, err := r.resolveOne(ctx, workspace.Spec.Repositories[index], boxID, taskKey)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, facts)
	}
	return resolved, nil
}

func (r Resolver) resolveOne(ctx context.Context, repository manifest.Repository, boxID identity.BoxID, taskKey string) (Facts, error) {
	if repository.CanonicalLocal == "" {
		return Facts{}, fmt.Errorf("resolve repository %s: canonical local source is required", repository.Name)
	}
	topLevel, err := r.Git.TopLevel(ctx, repository.CanonicalLocal)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s top level: %w", repository.Name, err)
	}
	sourceRoot, err := canonicalTopLevel(topLevel.Stdout, repository.CanonicalLocal)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s top level: %w", repository.Name, err)
	}

	status, err := r.Git.Status(ctx, sourceRoot)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s status: %w", repository.Name, err)
	}
	if err := CheckClean(repository.Name, status.Stdout); err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s status: %w", repository.Name, err)
	}

	commitResult, err := r.Git.ResolveCommit(ctx, sourceRoot, repository.Checkout.Ref)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s commit: %w", repository.Name, err)
	}
	commit, err := fullCommit(commitResult.Stdout)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s commit: %w", repository.Name, err)
	}
	branch := strings.ReplaceAll(repository.Checkout.Branch, taskTemplate, taskKey)
	branchResult, err := r.Git.CheckBranch(ctx, sourceRoot, branch)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s branch: %w", repository.Name, err)
	}
	if checked := trimSingleLine(branchResult.Stdout); checked != branch {
		return Facts{}, fmt.Errorf("resolve repository %s branch: Git normalized or expanded the intended branch", repository.Name)
	}

	worktreePath, err := r.Paths.WorkspacePath(boxID, repository.WorkspacePath)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s worktree path: %w", repository.Name, err)
	}
	gitStorePath, err := r.Paths.GitStore(boxID, repository.Name)
	if err != nil {
		return Facts{}, fmt.Errorf("resolve repository %s Git-store path: %w", repository.Name, err)
	}
	return Facts{
		Name: repository.Name, SourceRoot: sourceRoot, Clean: true,
		BaseCommit: commit, Branch: branch, WorktreePath: worktreePath, GitStorePath: gitStorePath,
		PublishDestination: Destination{Kind: DestinationLocal, URL: sourceRoot},
	}, nil
}

func canonicalTopLevel(output []byte, canonicalInput string) (string, error) {
	value := trimSingleLine(output)
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", errors.New("Git returned a non-canonical top-level path")
	}
	canonical, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", errors.New("cannot canonicalize Git top-level path")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", errors.New("cannot make Git top-level path absolute")
	}
	canonical = filepath.Clean(canonical)
	if !within(canonical, canonicalInput) {
		return "", errors.New("Git top-level path does not contain the trusted source")
	}
	return canonical, nil
}

func fullCommit(output []byte) (string, error) {
	value := trimSingleLine(output)
	if len(value) != 40 {
		return "", errors.New("Git did not return a full 40-character commit ID")
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", errors.New("Git returned an invalid commit ID")
		}
	}
	return value, nil
}

func trimSingleLine(output []byte) string {
	trimmed := bytes.TrimSuffix(output, []byte("\n"))
	return string(trimmed)
}

func parseStatusPaths(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	records := bytes.Split(output, []byte{0})
	if len(records[len(records)-1]) != 0 {
		return nil, errors.New("porcelain status is not NUL terminated")
	}
	var changed []string
	for index := 0; index < len(records)-1; index++ {
		record := string(records[index])
		if record == "" {
			return nil, errors.New("porcelain status contains an empty record")
		}
		switch record[0] {
		case '1':
			path, ok := statusField(record, 9)
			if !ok {
				return nil, errors.New("malformed ordinary porcelain record")
			}
			changed = append(changed, path)
		case '2':
			path, ok := statusField(record, 10)
			if !ok || index+1 >= len(records)-1 || len(records[index+1]) == 0 {
				return nil, errors.New("malformed rename porcelain record")
			}
			changed = append(changed, path, string(records[index+1]))
			index++
		case 'u':
			path, ok := statusField(record, 11)
			if !ok {
				return nil, errors.New("malformed unmerged porcelain record")
			}
			changed = append(changed, path)
		case '?':
			if !strings.HasPrefix(record, "? ") || len(record) == 2 {
				return nil, errors.New("malformed untracked porcelain record")
			}
			changed = append(changed, record[2:])
		default:
			return nil, errors.New("unsupported porcelain status record")
		}
	}
	sort.Strings(changed)
	return compact(changed), nil
}

func statusField(record string, fields int) (string, bool) {
	parts := strings.SplitN(record, " ", fields)
	return parts[len(parts)-1], len(parts) == fields && parts[len(parts)-1] != ""
}

func compact(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
