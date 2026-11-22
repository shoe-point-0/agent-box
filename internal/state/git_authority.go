package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

const gitAuthoritySchemaVersion = "abox.reconciliation-git-authority/v1alpha1"

// GitWorktreeAuthority is the immutable Machine identity and Git registration
// retained immediately after a worktree is created and verified.
type GitWorktreeAuthority struct {
	CanonicalPath string
	GitCommonDir  string
	Branch        string
	LockReason    string
	PathIdentity  ownership.FileIdentity
}

// ReconciliationGitAuthority is monotonic crash-recovery authority for Git
// resources created before a complete READY generation can be published.
// Database presence never replaces fresh filesystem and Git observation.
type ReconciliationGitAuthority struct {
	BoxID       string
	Generation  int64
	Repository  repository.Facts
	PlanDigest  contract.Digest
	OperationID string
	Store       *gitstore.Record
	Worktree    *GitWorktreeAuthority
}

type gitAuthorityDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	ReconciliationGitAuthority
}

// SaveReconciliationGitAuthority creates an immutable claim before Git apply,
// then permits only monotonic addition of the exact store and worktree proof.
func (s *Store) SaveReconciliationGitAuthority(ctx context.Context, authority ReconciliationGitAuthority, updatedAt time.Time) error {
	encoded, err := encodeGitAuthority(authority)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		return errors.New("save reconciliation Git authority: update time is required")
	}
	return s.Update(ctx, func(tx *sql.Tx) error {
		if err := requireAuthorityWrite(ctx, tx, "Git", authority.BoxID, authority.OperationID, authority.Generation); err != nil {
			return err
		}

		var existingJSON []byte
		err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_git_authority
			WHERE box_id = ? AND generation = ? AND repository_name = ?`,
			authority.BoxID, authority.Generation, authority.Repository.Name).Scan(&existingJSON)
		if err == nil {
			existing, decodeErr := decodeGitAuthority(existingJSON)
			if decodeErr != nil {
				return decodeErr
			}
			if err := validateGitAuthorityTransition(existing, authority); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save reconciliation Git authority: query existing authority: %w", err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO reconciliation_git_authority
			(box_id, generation, repository_name, operation_id, plan_digest, authority_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(box_id, generation, repository_name) DO UPDATE SET
			operation_id = excluded.operation_id,
			authority_json = excluded.authority_json, updated_at = excluded.updated_at`,
			authority.BoxID, authority.Generation, authority.Repository.Name,
			authority.OperationID, authority.PlanDigest, string(encoded), updatedAt.UnixNano())
		if err != nil {
			return fmt.Errorf("save reconciliation Git authority: persist authority: %w", err)
		}
		return nil
	})
}

// ReconciliationGitAuthorityFor loads partial authority for an exact planned
// generation and repository. Callers must re-observe every returned resource.
func (s *Store) ReconciliationGitAuthorityFor(ctx context.Context, boxID string, generation int64, repositoryName string) (authority ReconciliationGitAuthority, found bool, err error) {
	err = s.View(ctx, func(tx *sql.Tx) error {
		var encoded []byte
		var operationID, planDigest string
		err := tx.QueryRowContext(ctx, `SELECT operation_id, plan_digest, authority_json FROM reconciliation_git_authority
			WHERE box_id = ? AND generation = ? AND repository_name = ?`,
			boxID, generation, repositoryName).Scan(&operationID, &planDigest, &encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query reconciliation Git authority: %w", err)
		}
		decoded, err := decodeGitAuthority(encoded)
		if err != nil {
			return err
		}
		if decoded.BoxID != boxID || decoded.Generation != generation || decoded.Repository.Name != repositoryName ||
			decoded.OperationID != operationID || string(decoded.PlanDigest) != planDigest {
			return errors.New("read reconciliation Git authority: row identity differs from payload")
		}
		authority, found = decoded, true
		return nil
	})
	if err != nil {
		return ReconciliationGitAuthority{}, false, err
	}
	return authority, found, nil
}

func encodeGitAuthority(authority ReconciliationGitAuthority) ([]byte, error) {
	if err := validateGitAuthority(authority); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(gitAuthorityDocument{SchemaVersion: gitAuthoritySchemaVersion, ReconciliationGitAuthority: authority})
	if err != nil {
		return nil, fmt.Errorf("encode reconciliation Git authority: %w", err)
	}
	return encoded, nil
}

func decodeGitAuthority(encoded []byte) (ReconciliationGitAuthority, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document gitAuthorityDocument
	if err := decoder.Decode(&document); err != nil {
		return ReconciliationGitAuthority{}, fmt.Errorf("decode reconciliation Git authority: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReconciliationGitAuthority{}, errors.New("decode reconciliation Git authority: trailing content")
	}
	if document.SchemaVersion != gitAuthoritySchemaVersion {
		return ReconciliationGitAuthority{}, errors.New("decode reconciliation Git authority: unsupported schema version")
	}
	if err := validateGitAuthority(document.ReconciliationGitAuthority); err != nil {
		return ReconciliationGitAuthority{}, fmt.Errorf("decode reconciliation Git authority: %w", err)
	}
	return document.ReconciliationGitAuthority, nil
}

func validateGitAuthority(authority ReconciliationGitAuthority) error {
	facts := authority.Repository
	if invalidFact(authority.BoxID) || authority.Generation < 1 || !validSHA256(authority.PlanDigest) ||
		invalidFact(authority.OperationID) || invalidFact(facts.Name) || !facts.Clean ||
		!cleanAbsolutePath(facts.SourceRoot) || !validCommit(facts.BaseCommit) || invalidFact(facts.Branch) ||
		!cleanAbsolutePath(facts.WorktreePath) || !cleanAbsolutePath(facts.GitStorePath) ||
		facts.PublishDestination.Kind != repository.DestinationLocal || facts.PublishDestination.URL != facts.SourceRoot {
		return errors.New("save reconciliation Git authority: identity is incomplete")
	}
	if authority.Store != nil {
		store := authority.Store
		if store.Repository != facts.Name || store.GitCommonDir != facts.GitStorePath || store.BaseCommit != facts.BaseCommit ||
			store.PublishDestination != facts.PublishDestination || store.Identity == (ownership.FileIdentity{}) {
			return errors.New("save reconciliation Git authority: private-store proof differs from intent")
		}
	}
	if authority.Worktree != nil {
		worktree := authority.Worktree
		if authority.Store == nil || worktree.CanonicalPath != facts.WorktreePath || worktree.GitCommonDir != facts.GitStorePath ||
			worktree.Branch != facts.Branch || invalidFact(worktree.LockReason) || worktree.PathIdentity == (ownership.FileIdentity{}) {
			return errors.New("save reconciliation Git authority: worktree proof differs from intent")
		}
	}
	return nil
}

func validateGitAuthorityTransition(existing, next ReconciliationGitAuthority) error {
	existingBase, nextBase := existing, next
	existingBase.Store, existingBase.Worktree = nil, nil
	nextBase.Store, nextBase.Worktree = nil, nil
	existingBase.OperationID, nextBase.OperationID = "", ""
	if !reflect.DeepEqual(existingBase, nextBase) {
		return errors.New("save reconciliation Git authority: claimed identity is immutable")
	}
	if existing.Store != nil && next.Store == nil || existing.Worktree != nil && next.Worktree == nil {
		return errors.New("save reconciliation Git authority: proof cannot regress")
	}
	if existing.Store != nil && !reflect.DeepEqual(existing.Store, next.Store) {
		return errors.New("save reconciliation Git authority: private-store proof is immutable")
	}
	if existing.Worktree != nil && !reflect.DeepEqual(existing.Worktree, next.Worktree) {
		return errors.New("save reconciliation Git authority: worktree proof is immutable")
	}
	return nil
}
