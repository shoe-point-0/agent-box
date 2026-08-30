// Package generatedstate owns removal of transient Box-generated files while
// retaining the immutable Plan and diagnostics needed for recovery.
package generatedstate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
)

// Record is the persisted ownership identity captured when the Box root is
// created. It is authority only when it also matches the immutable Plan and
// fresh filesystem observation.
type Record struct {
	BoxID string
	Root  ownership.PathRecord
}

// Artifact identifies one exact transient generated path present at preview.
type Artifact struct {
	Path     string
	Identity ownership.FileIdentity
	Kind     string
}

// Observation is a read-only snapshot used to bind confirmation and a final
// ownership proof. Present is false when no transient generated artifact
// remains; plan.json and diagnostics are deliberately retained.
type Observation struct {
	BoxID        string
	Root         string
	RootIdentity ownership.FileIdentity
	Artifacts    []Artifact
	Present      bool
}

// Operations is the injectable destructive boundary used for fault tests.
type Operations struct {
	Remove        func(ownership.PathProof) error
	SyncDirectory func(string) error
}

// SystemOperations returns the operating-system implementation.
func SystemOperations() Operations {
	return Operations{
		Remove:        func(proof ownership.PathProof) error { return os.Remove(proof.Path()) },
		SyncDirectory: syncDirectory,
	}
}

// Manager observes and removes only path-provider-defined transient state.
type Manager struct {
	paths      *paths.Provider
	operations Operations
}

// NewManager constructs a generated-state manager with explicit filesystem
// operations. It never removes plan.json or diagnostics.
func NewManager(provider *paths.Provider, operations Operations) (*Manager, error) {
	if provider == nil || operations.Remove == nil || operations.SyncDirectory == nil {
		return nil, errors.New("configure generated-state removal: paths and operations are required")
	}
	return &Manager{paths: provider, operations: operations}, nil
}

// ObserveRemoval validates the immutable Plan, persisted Box-root identity,
// retained plan.json, and every transient target without mutating the filesystem.
func (m *Manager) ObserveRemoval(_ context.Context, immutable contract.Plan, record Record) (Observation, error) {
	boxID, root, encoded, err := m.validate(immutable, record)
	if err != nil {
		return Observation{}, err
	}
	observation := Observation{BoxID: record.BoxID, Root: root}
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return observation, nil
	}
	if err != nil {
		return Observation{}, fmt.Errorf("observe generated-state root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return Observation{}, errors.New("observe generated-state root: Box root is not a real directory")
	}
	rootProof, err := ownership.ProvePath(m.paths.BoxesRoot(), record.Root)
	if err != nil || rootProof.Path() != root {
		return Observation{}, fmt.Errorf("observe generated-state root ownership: %w", errors.Join(err, ownership.ErrNotOwned))
	}
	observation.RootIdentity = rootProof.Identity()
	if err := verifyRetainedPlan(m.paths, boxID, encoded); err != nil {
		return Observation{}, err
	}

	targets, err := transientTargets(m.paths, boxID)
	if err != nil {
		return Observation{}, err
	}
	known := make(map[string]struct{}, len(targets)+2)
	for _, target := range targets {
		known[filepath.Base(target.path)] = struct{}{}
	}
	known["plan.json"] = struct{}{}
	known["diagnostics"] = struct{}{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return Observation{}, fmt.Errorf("observe generated-state root entries: %w", err)
	}
	for _, entry := range entries {
		if _, ok := known[entry.Name()]; ok {
			continue
		}
		if !strings.HasPrefix(entry.Name(), atomicfile.TemporaryPrefix) {
			return Observation{}, errors.New("observe generated-state root: unrecognized entry prevents cleanup")
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return Observation{}, fmt.Errorf("observe generated-state atomic temporary: %w", statErr)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Observation{}, errors.New("observe generated-state atomic temporary: type is unsafe")
		}
		artifactIdentity, identityErr := ownership.InspectPath(path)
		if identityErr != nil {
			return Observation{}, fmt.Errorf("observe generated-state atomic temporary: %w", identityErr)
		}
		observation.Artifacts = append(observation.Artifacts, Artifact{Path: path, Identity: artifactIdentity, Kind: "atomic-temporary"})
	}
	for _, target := range targets {
		info, statErr := os.Lstat(target.path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return Observation{}, fmt.Errorf("observe generated-state artifact %s: %w", target.kind, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || target.directory != info.IsDir() || !target.directory && !info.Mode().IsRegular() {
			return Observation{}, fmt.Errorf("observe generated-state artifact %s: type is unsafe", target.kind)
		}
		if target.directory {
			if _, readErr := os.ReadDir(target.path); readErr != nil {
				return Observation{}, fmt.Errorf("observe generated-state artifact %s: %w", target.kind, readErr)
			}
			// Worktree and private-store removal own the contents. Recording the
			// directory itself here lets the complete preflight run before those
			// dependency stages; os.Remove still refuses it unless they converge
			// the directory to empty before generated-state teardown.
		}
		identity, identityErr := ownership.InspectPath(target.path)
		if identityErr != nil {
			return Observation{}, fmt.Errorf("observe generated-state artifact %s: %w", target.kind, identityErr)
		}
		observation.Artifacts = append(observation.Artifacts, Artifact{Path: target.path, Identity: identity, Kind: target.kind})
	}
	slices.SortFunc(observation.Artifacts, func(left, right Artifact) int {
		return strings.Compare(left.Path, right.Path)
	})
	observation.Present = len(observation.Artifacts) != 0
	return observation, nil
}

// Remove repeats the complete observation, proves every exact inode below the
// unchanged Box root, and removes only the transient paths in that observation.
func (m *Manager) Remove(ctx context.Context, immutable contract.Plan, record Record, expected Observation) error {
	actual, err := m.ObserveRemoval(ctx, immutable, record)
	if err != nil {
		return err
	}
	if !equalObservation(actual, expected) {
		return errors.New("remove generated state: observation changed")
	}
	if !actual.Present {
		return nil
	}
	if _, err := ownership.ProvePath(m.paths.BoxesRoot(), record.Root); err != nil {
		return fmt.Errorf("remove generated state: prove Box root: %w", err)
	}
	for _, artifact := range actual.Artifacts {
		proof, err := ownership.ProvePath(actual.Root, ownership.PathRecord{CanonicalPath: artifact.Path, Identity: artifact.Identity})
		if err != nil {
			return fmt.Errorf("remove generated state artifact %s: %w", artifact.Kind, err)
		}
		if err := m.operations.Remove(proof); err != nil {
			return fmt.Errorf("remove generated state artifact %s: %w", artifact.Kind, err)
		}
	}
	if err := m.operations.SyncDirectory(actual.Root); err != nil {
		return fmt.Errorf("sync generated-state root: %w", err)
	}
	return nil
}

func (m *Manager) validate(immutable contract.Plan, record Record) (identity.BoxID, string, []byte, error) {
	encoded, err := plan.CanonicalJSON(immutable)
	if err != nil {
		return "", "", nil, fmt.Errorf("generated-state removal immutable Plan: %w", err)
	}
	boxID := identity.BoxID(immutable.BoxID)
	if err := identity.ValidateBoxID(boxID); err != nil || record.BoxID != immutable.BoxID {
		return "", "", nil, errors.New("generated-state removal identity is invalid")
	}
	root, err := m.paths.BoxRoot(boxID)
	if err != nil {
		return "", "", nil, err
	}
	if record.Root.CanonicalPath != root || record.Root.Identity == (ownership.FileIdentity{}) {
		return "", "", nil, errors.New("generated-state removal root record differs from the Plan")
	}
	return boxID, root, encoded, nil
}

func verifyRetainedPlan(provider *paths.Provider, boxID identity.BoxID, encoded []byte) error {
	path, err := provider.Plan(boxID)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("observe retained Plan: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("observe retained Plan: path is not a real regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("observe retained Plan: %w", err)
	}
	if !bytes.Equal(contents, encoded) {
		return errors.New("observe retained Plan: content differs from immutable Plan")
	}
	return nil
}

type target struct {
	path      string
	kind      string
	directory bool
}

func transientTargets(provider *paths.Provider, boxID identity.BoxID) ([]target, error) {
	constructors := []struct {
		kind      string
		directory bool
		path      func(identity.BoxID) (string, error)
	}{
		{kind: "context", path: provider.Context},
		{kind: "lima-config", path: provider.LimaConfig},
		{kind: "compose-root", path: provider.ComposeRoot},
		{kind: "compose-override", path: provider.ComposeOverride},
		{kind: "compose-resolved", path: provider.ComposeResolved},
		{kind: "git-root", directory: true, path: provider.GitRoot},
		{kind: "workspace-root", directory: true, path: provider.WorkspaceRoot},
	}
	targets := make([]target, 0, len(constructors))
	for _, constructor := range constructors {
		path, err := constructor.path(boxID)
		if err != nil {
			return nil, fmt.Errorf("construct generated-state artifact %s: %w", constructor.kind, err)
		}
		targets = append(targets, target{path: path, kind: constructor.kind, directory: constructor.directory})
	}
	return targets, nil
}

func equalObservation(left, right Observation) bool {
	return left.BoxID == right.BoxID && left.Root == right.Root && left.RootIdentity == right.RootIdentity &&
		left.Present == right.Present && slices.Equal(left.Artifacts, right.Artifacts)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}
