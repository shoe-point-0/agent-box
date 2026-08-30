package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

// RemovalObservation is fresh Machine-filesystem evidence for one private Git
// store. An absent store is the terminal state after an interrupted removal.
type RemovalObservation struct {
	Repository    string
	CanonicalPath string
	Identity      ownership.FileIdentity
	Present       bool
}

// PathRemover is the narrow destructive filesystem boundary. Requiring a
// PathProof prevents implementations from accepting an unverified raw path.
type PathRemover interface {
	Remove(context.Context, ownership.PathProof) error
}

// OSPathRemover recursively removes one freshly proved owned path.
type OSPathRemover struct{}

func (OSPathRemover) Remove(ctx context.Context, proof ownership.PathProof) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.RemoveAll(proof.Path())
}

// RemovalManager observes and removes one Box-private Git store below an
// explicit canonical Box root.
type RemovalManager struct {
	BoxRoot string
	Paths   PathRemover
}

// ObserveRemoval verifies immutable Plan, persisted record, containment, and
// live inode identity without mutating the filesystem.
func (m RemovalManager) ObserveRemoval(_ context.Context, facts repository.Facts, immutable contract.Plan, record Record) (RemovalObservation, error) {
	if err := m.validateInput(facts, immutable, record); err != nil {
		return RemovalObservation{}, err
	}
	observation := RemovalObservation{Repository: facts.Name, CanonicalPath: facts.GitStorePath}
	info, err := os.Lstat(facts.GitStorePath)
	if errors.Is(err, os.ErrNotExist) {
		return observation, nil
	}
	if err != nil {
		return RemovalObservation{}, fmt.Errorf("observe private Git store %s: inspect path: %w", facts.Name, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return RemovalObservation{}, &StoreConflictError{Repository: facts.Name}
	}
	proof, err := ownership.ProvePath(m.BoxRoot, ownership.PathRecord{
		CanonicalPath: record.GitCommonDir,
		Identity:      record.Identity,
	})
	if err != nil {
		return RemovalObservation{}, &StoreConflictError{Repository: facts.Name}
	}
	observation.Identity = proof.Identity()
	observation.Present = true
	return observation, nil
}

// Remove reobserves the exact expected inode, consumes a typed path proof, and
// verifies terminal absence. A fresh absent observation makes retries converge
// without replaying recursive deletion.
func (m RemovalManager) Remove(ctx context.Context, facts repository.Facts, immutable contract.Plan, record Record, expected RemovalObservation) error {
	fresh, err := m.ObserveRemoval(ctx, facts, immutable, record)
	if err != nil {
		return err
	}
	if fresh != expected {
		return &StoreConflictError{Repository: facts.Name}
	}
	if !fresh.Present {
		return nil
	}
	proof, err := ownership.ProvePath(m.BoxRoot, ownership.PathRecord{
		CanonicalPath: fresh.CanonicalPath,
		Identity:      fresh.Identity,
	})
	if err != nil {
		return &StoreConflictError{Repository: facts.Name}
	}
	if err := m.Paths.Remove(ctx, proof); err != nil {
		return fmt.Errorf("remove private Git store %s: %w", facts.Name, err)
	}
	terminal, err := m.ObserveRemoval(ctx, facts, immutable, record)
	if err != nil {
		return fmt.Errorf("verify private Git store removal %s: %w", facts.Name, err)
	}
	if terminal.Present {
		return &StoreConflictError{Repository: facts.Name}
	}
	return nil
}

func (m RemovalManager) validateInput(facts repository.Facts, immutable contract.Plan, record Record) error {
	if m.Paths == nil {
		return errors.New("remove private Git store: path remover is required")
	}
	if err := validateInput(facts, immutable); err != nil {
		return err
	}
	if record.Repository != facts.Name || record.GitCommonDir != facts.GitStorePath ||
		record.BaseCommit != facts.BaseCommit || record.PublishDestination != facts.PublishDestination ||
		record.Identity == (ownership.FileIdentity{}) {
		return errors.New("remove private Git store: persisted record does not match resolved facts")
	}
	if !cleanAbsolute(m.BoxRoot) || !strictlyWithin(m.BoxRoot, facts.GitStorePath) {
		return errors.New("remove private Git store: path is outside the canonical Box root")
	}
	info, err := os.Lstat(m.BoxRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("remove private Git store: Box root is not a real directory")
	}
	resolved, err := filepath.EvalSymlinks(m.BoxRoot)
	if err != nil || resolved != m.BoxRoot {
		return errors.New("remove private Git store: Box root is not canonical")
	}
	return nil
}

func cleanAbsolute(path string) bool {
	return path != "" && strings.IndexByte(path, 0) < 0 && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func strictlyWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
