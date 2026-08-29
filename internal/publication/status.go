// Package publication classifies Box commits against trusted publish destinations.
package publication

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

const (
	DestinationMissing     = "missing"
	DestinationUnavailable = "unavailable"
)

// Git is the read-only hardened Machine Git surface needed by Checker.
type Git interface {
	ResolveCommit(context.Context, string, string) (machinegit.Result, error)
	ReachableCommits(context.Context, string, ...string) (machinegit.Result, error)
}

// Checker compares exact Box commits with destinations supplied from trusted
// control-plane state. It never discovers destinations from Git configuration.
type Checker struct {
	Git Git
}

// Reference identifies one Box ref. Name is an explicit full ref, or empty for
// a detached commit. Commit is always the recorded full object ID.
type Reference struct {
	Name   string
	Commit string
}

// DestinationEvidence records whether one trusted destination could be
// inspected and the exact commits from a Reference not reachable there.
type DestinationEvidence struct {
	Destination        repository.Destination
	Available          bool
	Reason             string
	UnpublishedCommits []string
}

// ReferenceStatus is the conservative publication classification for one Box
// ref. Published means every reachable commit exists in at least one available
// trusted destination.
type ReferenceStatus struct {
	Reference          Reference
	Published          bool
	UnpublishedCommits []string
	Destinations       []DestinationEvidence
}

// ReferenceDriftError reports a named Box ref that no longer matches its
// recorded commit. Publication status is unsafe until the worktree is observed
// again.
type ReferenceDriftError struct {
	Reference Reference
}

func (e *ReferenceDriftError) Error() string {
	return "publication status reference differs from recorded commit"
}

// Check returns stable exact evidence for every reference. Unavailable
// destinations contribute no proof of publication; other available
// destinations can still prove individual commits reachable.
func (c Checker) Check(ctx context.Context, store gitstore.Record, references []Reference, destinations []repository.Destination) ([]ReferenceStatus, error) {
	if c.Git == nil {
		return nil, errors.New("check publication status: Git adapter is required")
	}
	if err := validateInput(store, references, destinations); err != nil {
		return nil, err
	}
	liveIdentity, err := ownership.InspectPath(store.GitCommonDir)
	if err != nil || liveIdentity != store.Identity {
		return nil, errors.New("check publication status: private-store identity differs from its record")
	}

	boxCommits := make([][]string, len(references))
	for index, reference := range references {
		object := reference.Commit
		if reference.Name != "" {
			resolved, resolveErr := c.Git.ResolveCommit(ctx, store.GitCommonDir, reference.Name)
			if resolveErr != nil {
				return nil, fmt.Errorf("check publication status ref %d: resolve: %w", index, resolveErr)
			}
			actual, parseErr := parseResolvedCommit(resolved.Stdout)
			if parseErr != nil || actual != reference.Commit {
				return nil, &ReferenceDriftError{Reference: reference}
			}
		}
		result, reachErr := c.Git.ReachableCommits(ctx, store.GitCommonDir, object)
		if reachErr != nil {
			return nil, fmt.Errorf("check publication status ref %d: enumerate Box commits: %w", index, reachErr)
		}
		commits, parseErr := parseCommitList(result.Stdout)
		if parseErr != nil {
			return nil, fmt.Errorf("check publication status ref %d: parse Box commits: %w", index, parseErr)
		}
		if !slices.Contains(commits, reference.Commit) {
			return nil, &ReferenceDriftError{Reference: reference}
		}
		boxCommits[index] = commits
	}

	destinationCommits := make([]map[string]struct{}, len(destinations))
	destinationReasons := make([]string, len(destinations))
	for index, destination := range destinations {
		reason := inspectDestination(destination)
		if reason != "" {
			destinationReasons[index] = reason
			continue
		}
		result, reachErr := c.Git.ReachableCommits(ctx, destination.URL)
		if reachErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			destinationReasons[index] = DestinationUnavailable
			continue
		}
		commits, parseErr := parseCommitList(result.Stdout)
		if parseErr != nil {
			destinationReasons[index] = DestinationUnavailable
			continue
		}
		destinationCommits[index] = commitSet(commits)
	}

	statuses := make([]ReferenceStatus, len(references))
	for referenceIndex, reference := range references {
		publishedAnywhere := make(map[string]struct{})
		evidence := make([]DestinationEvidence, len(destinations))
		for destinationIndex, destination := range destinations {
			evidence[destinationIndex] = DestinationEvidence{
				Destination: destination,
				Available:   destinationReasons[destinationIndex] == "",
				Reason:      destinationReasons[destinationIndex],
			}
			if destinationReasons[destinationIndex] != "" {
				evidence[destinationIndex].UnpublishedCommits = slices.Clone(boxCommits[referenceIndex])
				continue
			}
			for commit := range destinationCommits[destinationIndex] {
				publishedAnywhere[commit] = struct{}{}
			}
			evidence[destinationIndex].UnpublishedCommits = difference(boxCommits[referenceIndex], destinationCommits[destinationIndex])
		}
		unpublished := difference(boxCommits[referenceIndex], publishedAnywhere)
		statuses[referenceIndex] = ReferenceStatus{
			Reference: reference, Published: len(unpublished) == 0,
			UnpublishedCommits: unpublished, Destinations: evidence,
		}
	}
	return statuses, nil
}

func validateInput(store gitstore.Record, references []Reference, destinations []repository.Destination) error {
	if store.Repository == "" || store.GitCommonDir == "" || store.Identity == (ownership.FileIdentity{}) {
		return errors.New("check publication status: complete private-store record is required")
	}
	if len(references) == 0 {
		return errors.New("check publication status: at least one reference is required")
	}
	if len(destinations) == 0 || !slices.Contains(destinations, store.PublishDestination) {
		return errors.New("check publication status: recorded publish destination is required")
	}
	seenReferences := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if !validCommit(reference.Commit) {
			return errors.New("check publication status: reference commit must be a full lowercase object ID")
		}
		key := reference.Name
		if key == "" {
			key = "detached:" + reference.Commit
		}
		if _, exists := seenReferences[key]; exists {
			return errors.New("check publication status: references must be unique")
		}
		seenReferences[key] = struct{}{}
	}
	seenDestinations := make(map[repository.Destination]struct{}, len(destinations))
	for _, destination := range destinations {
		if destination.Kind != repository.DestinationLocal {
			return errors.New("check publication status: unsupported destination kind")
		}
		if _, exists := seenDestinations[destination]; exists {
			return errors.New("check publication status: destinations must be unique")
		}
		seenDestinations[destination] = struct{}{}
	}
	return nil
}

func inspectDestination(destination repository.Destination) string {
	if destination.URL == "" || !filepath.IsAbs(destination.URL) || filepath.Clean(destination.URL) != destination.URL || strings.IndexByte(destination.URL, 0) >= 0 {
		return DestinationUnavailable
	}
	if _, err := os.Lstat(destination.URL); errors.Is(err, os.ErrNotExist) {
		return DestinationMissing
	} else if err != nil {
		return DestinationUnavailable
	}
	canonical, err := filepath.EvalSymlinks(destination.URL)
	if err != nil || canonical != destination.URL {
		return DestinationUnavailable
	}
	return ""
}

func parseResolvedCommit(output []byte) (string, error) {
	value := string(bytes.TrimSuffix(output, []byte("\n")))
	if !validCommit(value) {
		return "", errors.New("Git returned an invalid resolved commit")
	}
	return value, nil
}

func parseCommitList(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != '\n' {
		return nil, errors.New("rev-list output is not newline terminated")
	}
	lines := strings.Split(string(output[:len(output)-1]), "\n")
	seen := make(map[string]struct{}, len(lines))
	for _, commit := range lines {
		if !validCommit(commit) {
			return nil, errors.New("rev-list output contains an invalid commit ID")
		}
		if _, exists := seen[commit]; exists {
			return nil, errors.New("rev-list output contains a duplicate commit ID")
		}
		seen[commit] = struct{}{}
	}
	slices.Sort(lines)
	return lines, nil
}

func validCommit(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}

func commitSet(commits []string) map[string]struct{} {
	result := make(map[string]struct{}, len(commits))
	for _, commit := range commits {
		result[commit] = struct{}{}
	}
	return result
}

func difference(commits []string, reachable map[string]struct{}) []string {
	var result []string
	for _, commit := range commits {
		if _, exists := reachable[commit]; !exists {
			result = append(result, commit)
		}
	}
	return result
}
