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
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

const generatedAuthoritySchemaVersion = "abox.reconciliation-generated-authority/v1alpha1"

// ReconciliationGeneratedAuthority is monotonic crash-recovery authority for
// the generated Box root created before a complete READY generation exists.
type ReconciliationGeneratedAuthority struct {
	BoxID       string
	Generation  int64
	PlanDigest  contract.Digest
	OperationID string
	BoxRoot     *generatedstate.Record
}

type generatedAuthorityDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	ReconciliationGeneratedAuthority
}

// SaveReconciliationGeneratedAuthority writes intent before root creation and
// then permits only monotonic addition of the exact root identity.
func (s *Store) SaveReconciliationGeneratedAuthority(ctx context.Context, authority ReconciliationGeneratedAuthority, updatedAt time.Time) error {
	encoded, err := encodeGeneratedAuthority(authority)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		return errors.New("save reconciliation generated authority: update time is required")
	}
	return s.Update(ctx, func(tx *sql.Tx) error {
		if err := requireAuthorityWrite(ctx, tx, "generated", authority.BoxID, authority.OperationID, authority.Generation); err != nil {
			return err
		}

		var existingJSON []byte
		err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_generated_authority
			WHERE box_id = ? AND generation = ?`, authority.BoxID, authority.Generation).Scan(&existingJSON)
		if err == nil {
			existing, decodeErr := decodeGeneratedAuthority(existingJSON)
			if decodeErr != nil {
				return decodeErr
			}
			if err := validateGeneratedAuthorityTransition(existing, authority); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save reconciliation generated authority: query existing authority: %w", err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO reconciliation_generated_authority
			(box_id, generation, operation_id, plan_digest, authority_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(box_id, generation) DO UPDATE SET
			operation_id = excluded.operation_id, authority_json = excluded.authority_json, updated_at = excluded.updated_at`,
			authority.BoxID, authority.Generation, authority.OperationID, authority.PlanDigest, string(encoded), updatedAt.UnixNano())
		if err != nil {
			return fmt.Errorf("save reconciliation generated authority: persist authority: %w", err)
		}
		return nil
	})
}

// ReconciliationGeneratedAuthorityFor loads authority for an exact planned
// generation. The returned root must still be freshly observed before use.
func (s *Store) ReconciliationGeneratedAuthorityFor(ctx context.Context, boxID string, generation int64) (authority ReconciliationGeneratedAuthority, found bool, err error) {
	err = s.View(ctx, func(tx *sql.Tx) error {
		var encoded []byte
		var operationID, planDigest string
		err := tx.QueryRowContext(ctx, `SELECT operation_id, plan_digest, authority_json
			FROM reconciliation_generated_authority WHERE box_id = ? AND generation = ?`, boxID, generation).
			Scan(&operationID, &planDigest, &encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query reconciliation generated authority: %w", err)
		}
		decoded, err := decodeGeneratedAuthority(encoded)
		if err != nil {
			return err
		}
		if decoded.BoxID != boxID || decoded.Generation != generation || decoded.OperationID != operationID || string(decoded.PlanDigest) != planDigest {
			return errors.New("read reconciliation generated authority: row identity differs from payload")
		}
		authority, found = decoded, true
		return nil
	})
	if err != nil {
		return ReconciliationGeneratedAuthority{}, false, err
	}
	return authority, found, nil
}

func retireReconciliationGeneratedAuthority(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) error {
	var encoded []byte
	err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_generated_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("publish READY generation: query partial generated authority: %w", err)
	}
	authority, err := decodeGeneratedAuthority(encoded)
	if err != nil {
		return err
	}
	if authority.BoxID != publication.Plan.BoxID || authority.Generation != publication.Generation ||
		authority.PlanDigest != publication.Plan.PlanDigest || authority.BoxRoot == nil ||
		!reflect.DeepEqual(*authority.BoxRoot, publication.RemovalSnapshot.BoxRoot) {
		return errors.New("publish READY generation: partial generated authority differs from removal authority")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reconciliation_generated_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation); err != nil {
		return fmt.Errorf("publish READY generation: retire partial generated authority: %w", err)
	}
	return nil
}

func encodeGeneratedAuthority(authority ReconciliationGeneratedAuthority) ([]byte, error) {
	if err := validateGeneratedAuthority(authority); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(generatedAuthorityDocument{SchemaVersion: generatedAuthoritySchemaVersion, ReconciliationGeneratedAuthority: authority})
	if err != nil {
		return nil, fmt.Errorf("encode reconciliation generated authority: %w", err)
	}
	return encoded, nil
}

func decodeGeneratedAuthority(encoded []byte) (ReconciliationGeneratedAuthority, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document generatedAuthorityDocument
	if err := decoder.Decode(&document); err != nil {
		return ReconciliationGeneratedAuthority{}, fmt.Errorf("decode reconciliation generated authority: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReconciliationGeneratedAuthority{}, errors.New("decode reconciliation generated authority: trailing content")
	}
	if document.SchemaVersion != generatedAuthoritySchemaVersion {
		return ReconciliationGeneratedAuthority{}, errors.New("decode reconciliation generated authority: unsupported schema version")
	}
	if err := validateGeneratedAuthority(document.ReconciliationGeneratedAuthority); err != nil {
		return ReconciliationGeneratedAuthority{}, fmt.Errorf("decode reconciliation generated authority: %w", err)
	}
	return document.ReconciliationGeneratedAuthority, nil
}

func validateGeneratedAuthority(authority ReconciliationGeneratedAuthority) error {
	if invalidFact(authority.BoxID) || authority.Generation < 1 || !validSHA256(authority.PlanDigest) || invalidFact(authority.OperationID) {
		return errors.New("save reconciliation generated authority: identity is incomplete")
	}
	if authority.BoxRoot != nil {
		root := authority.BoxRoot
		if root.BoxID != authority.BoxID || !validPathRecord(root.Root) || root.Root.Identity == (ownership.FileIdentity{}) {
			return errors.New("save reconciliation generated authority: Box-root proof differs from intent")
		}
	}
	return nil
}

func validateGeneratedAuthorityTransition(existing, next ReconciliationGeneratedAuthority) error {
	existingBase, nextBase := existing, next
	existingBase.OperationID, nextBase.OperationID = "", ""
	existingBase.BoxRoot, nextBase.BoxRoot = nil, nil
	if !reflect.DeepEqual(existingBase, nextBase) {
		return errors.New("save reconciliation generated authority: claimed identity is immutable")
	}
	if existing.BoxRoot != nil && next.BoxRoot == nil {
		return errors.New("save reconciliation generated authority: proof cannot regress")
	}
	if existing.BoxRoot != nil && !reflect.DeepEqual(existing.BoxRoot, next.BoxRoot) {
		return errors.New("save reconciliation generated authority: Box-root proof is immutable")
	}
	return nil
}
