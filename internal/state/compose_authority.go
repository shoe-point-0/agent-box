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

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

const composeAuthoritySchemaVersion = "abox.reconciliation-compose-authority/v1alpha1"

// ReconciliationComposeAuthority is monotonic crash-recovery authority for
// Compose intent and the resource identities observed after an apply. Intent is
// retained before Compose may mutate; resource proof is added only afterward.
type ReconciliationComposeAuthority struct {
	BoxID       string
	Generation  int64
	PlanDigest  contract.Digest
	OperationID string
	Spec        compose.RuntimeSpec
	Records     *compose.RuntimeRecords
}

type composeAuthorityDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	ReconciliationComposeAuthority
}

// SaveReconciliationComposeAuthority writes immutable apply intent and permits
// only monotonic addition of the exact observed runtime records.
func (s *Store) SaveReconciliationComposeAuthority(ctx context.Context, authority ReconciliationComposeAuthority, updatedAt time.Time) error {
	encoded, err := encodeComposeAuthority(authority)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		return errors.New("save reconciliation Compose authority: update time is required")
	}
	return s.Update(ctx, func(tx *sql.Tx) error {
		if err := requireAuthorityWrite(ctx, tx, "Compose", authority.BoxID, authority.OperationID, authority.Generation); err != nil {
			return err
		}

		var existingJSON []byte
		err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_compose_authority
			WHERE box_id = ? AND generation = ?`, authority.BoxID, authority.Generation).Scan(&existingJSON)
		if err == nil {
			existing, decodeErr := decodeComposeAuthority(existingJSON)
			if decodeErr != nil {
				return decodeErr
			}
			if err := validateComposeAuthorityTransition(existing, authority); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save reconciliation Compose authority: query existing authority: %w", err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO reconciliation_compose_authority
			(box_id, generation, operation_id, plan_digest, authority_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(box_id, generation) DO UPDATE SET
			operation_id = excluded.operation_id, authority_json = excluded.authority_json, updated_at = excluded.updated_at`,
			authority.BoxID, authority.Generation, authority.OperationID, authority.PlanDigest, string(encoded), updatedAt.UnixNano())
		if err != nil {
			return fmt.Errorf("save reconciliation Compose authority: persist authority: %w", err)
		}
		return nil
	})
}

// ReconciliationComposeAuthorityFor loads recovery authority for one exact
// generation. The returned records are not a live runtime observation.
func (s *Store) ReconciliationComposeAuthorityFor(ctx context.Context, boxID string, generation int64) (authority ReconciliationComposeAuthority, found bool, err error) {
	err = s.View(ctx, func(tx *sql.Tx) error {
		var encoded []byte
		var operationID, planDigest string
		err := tx.QueryRowContext(ctx, `SELECT operation_id, plan_digest, authority_json
			FROM reconciliation_compose_authority WHERE box_id = ? AND generation = ?`, boxID, generation).
			Scan(&operationID, &planDigest, &encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query reconciliation Compose authority: %w", err)
		}
		decoded, err := decodeComposeAuthority(encoded)
		if err != nil {
			return err
		}
		if decoded.BoxID != boxID || decoded.Generation != generation || decoded.OperationID != operationID || string(decoded.PlanDigest) != planDigest {
			return errors.New("read reconciliation Compose authority: row identity differs from payload")
		}
		authority, found = decoded, true
		return nil
	})
	if err != nil {
		return ReconciliationComposeAuthority{}, false, err
	}
	return authority, found, nil
}

func retireReconciliationComposeAuthority(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) error {
	var encoded []byte
	err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_compose_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("publish READY generation: query partial Compose authority: %w", err)
	}
	authority, err := decodeComposeAuthority(encoded)
	if err != nil {
		return err
	}
	if authority.BoxID != publication.Plan.BoxID || authority.Generation != publication.Generation ||
		authority.PlanDigest != publication.Plan.PlanDigest || !reflect.DeepEqual(authority.Spec, publication.RemovalSnapshot.ComposeSpec) ||
		authority.Records == nil || !reflect.DeepEqual(*authority.Records, publication.RemovalSnapshot.Compose) {
		return errors.New("publish READY generation: partial Compose authority differs from removal authority")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reconciliation_compose_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation); err != nil {
		return fmt.Errorf("publish READY generation: retire partial Compose authority: %w", err)
	}
	return nil
}

func encodeComposeAuthority(authority ReconciliationComposeAuthority) ([]byte, error) {
	if err := validateComposeAuthority(authority); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(composeAuthorityDocument{SchemaVersion: composeAuthoritySchemaVersion, ReconciliationComposeAuthority: authority})
	if err != nil {
		return nil, fmt.Errorf("encode reconciliation Compose authority: %w", err)
	}
	return encoded, nil
}

func decodeComposeAuthority(encoded []byte) (ReconciliationComposeAuthority, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document composeAuthorityDocument
	if err := decoder.Decode(&document); err != nil {
		return ReconciliationComposeAuthority{}, fmt.Errorf("decode reconciliation Compose authority: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReconciliationComposeAuthority{}, errors.New("decode reconciliation Compose authority: trailing content")
	}
	if document.SchemaVersion != composeAuthoritySchemaVersion {
		return ReconciliationComposeAuthority{}, errors.New("decode reconciliation Compose authority: unsupported schema version")
	}
	if err := validateComposeAuthority(document.ReconciliationComposeAuthority); err != nil {
		return ReconciliationComposeAuthority{}, fmt.Errorf("decode reconciliation Compose authority: %w", err)
	}
	return document.ReconciliationComposeAuthority, nil
}

func validateComposeAuthority(authority ReconciliationComposeAuthority) error {
	spec := authority.Spec
	if invalidFact(authority.BoxID) || authority.Generation < 1 || !validSHA256(authority.PlanDigest) || invalidFact(authority.OperationID) ||
		spec.Plan.BoxID != authority.BoxID || spec.Plan.PlanDigest != authority.PlanDigest || spec.Generation != authority.Generation {
		return errors.New("save reconciliation Compose authority: identity is incomplete")
	}
	probe := RemovalSnapshot{
		BoxID: authority.BoxID, Generation: authority.Generation, PlanDigest: authority.PlanDigest,
		LimaSpec: spec.RuntimeHost, ComposeSpec: spec, ComposePathEvidence: spec.PathEvidence,
	}
	if err := validateComposeRemovalSpec(probe); err != nil {
		return errors.New("save reconciliation Compose authority: runtime specification is incomplete")
	}
	if authority.Records == nil {
		return nil
	}
	if len(authority.Records.Resources) == 0 {
		return errors.New("save reconciliation Compose authority: resource proof is absent")
	}
	generation := fmt.Sprint(authority.Generation)
	for _, record := range authority.Records.Resources {
		if invalidFact(record.Kind) || invalidFact(record.ID) ||
			!validComposeRemovalLabels(record.Labels, authority.BoxID, spec.Plan.Compose.ProjectName, string(authority.PlanDigest), generation) {
			return errors.New("save reconciliation Compose authority: resource proof differs from intent")
		}
	}
	for _, record := range authority.Records.Volumes {
		if invalidFact(record.ID) ||
			!validComposeRemovalLabels(record.Labels, authority.BoxID, spec.Plan.Compose.ProjectName, string(authority.PlanDigest), generation) {
			return errors.New("save reconciliation Compose authority: volume proof differs from intent")
		}
	}
	return nil
}

func validateComposeAuthorityTransition(existing, next ReconciliationComposeAuthority) error {
	existingBase, nextBase := existing, next
	existingBase.OperationID, nextBase.OperationID = "", ""
	existingBase.Records, nextBase.Records = nil, nil
	if !reflect.DeepEqual(existingBase, nextBase) {
		return errors.New("save reconciliation Compose authority: claimed identity is immutable")
	}
	if existing.Records != nil && next.Records == nil {
		return errors.New("save reconciliation Compose authority: proof cannot regress")
	}
	if existing.Records != nil && !reflect.DeepEqual(existing.Records, next.Records) {
		return errors.New("save reconciliation Compose authority: resource proof is immutable")
	}
	return nil
}
