package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

const limaAuthoritySchemaVersion = "abox.reconciliation-lima-authority/v1alpha1"

// ReconciliationLimaAuthority is monotonic crash-recovery authority for the
// generated Lima configuration and runtime host created before READY exists.
// Every retained path and instance must still be freshly re-observed before use.
type ReconciliationLimaAuthority struct {
	BoxID          string
	Generation     int64
	PlanDigest     contract.Digest
	OperationID    string
	LimaArtifact   toolchain.Identity
	Spec           lima.Spec
	ConfigPath     string
	ConfigEvidence *ownership.PathSetRecord
	Instance       *ownership.LimaInstanceRecord
}

type limaAuthorityDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	ReconciliationLimaAuthority
}

// SaveReconciliationLimaAuthority writes immutable runtime intent before
// mutation, then permits only monotonic addition of config and instance proof.
func (s *Store) SaveReconciliationLimaAuthority(ctx context.Context, authority ReconciliationLimaAuthority, updatedAt time.Time) error {
	encoded, err := encodeLimaAuthority(authority)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		return errors.New("save reconciliation Lima authority: update time is required")
	}
	return s.Update(ctx, func(tx *sql.Tx) error {
		var currentGeneration int64
		var operationKind, operationState string
		err := tx.QueryRowContext(ctx, `SELECT b.current_generation, o.kind, o.recorded_state
			FROM boxes AS b JOIN operations AS o ON o.box_id = b.id
			WHERE b.id = ? AND o.id = ?`, authority.BoxID, authority.OperationID).
			Scan(&currentGeneration, &operationKind, &operationState)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("save reconciliation Lima authority: Box operation is absent")
		}
		if err != nil {
			return fmt.Errorf("save reconciliation Lima authority: query Box operation: %w", err)
		}
		if operationKind != "up" || operationState != "running" {
			return errors.New("save reconciliation Lima authority: running up operation is required")
		}
		if authority.Generation != currentGeneration && authority.Generation != currentGeneration+1 {
			return errors.New("save reconciliation Lima authority: generation is not current or next")
		}

		var existingJSON []byte
		err = tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_lima_authority
			WHERE box_id = ? AND generation = ?`, authority.BoxID, authority.Generation).Scan(&existingJSON)
		if err == nil {
			existing, decodeErr := decodeLimaAuthority(existingJSON)
			if decodeErr != nil {
				return decodeErr
			}
			if err := validateLimaAuthorityTransition(existing, authority); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save reconciliation Lima authority: query existing authority: %w", err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO reconciliation_lima_authority
			(box_id, generation, operation_id, plan_digest, authority_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(box_id, generation) DO UPDATE SET
			operation_id = excluded.operation_id, authority_json = excluded.authority_json, updated_at = excluded.updated_at`,
			authority.BoxID, authority.Generation, authority.OperationID, authority.PlanDigest, string(encoded), updatedAt.UnixNano())
		if err != nil {
			return fmt.Errorf("save reconciliation Lima authority: persist authority: %w", err)
		}
		return nil
	})
}

// ReconciliationLimaAuthorityFor loads authority for one exact planned
// generation. Returned proof is recovery authority, not a live observation.
func (s *Store) ReconciliationLimaAuthorityFor(ctx context.Context, boxID string, generation int64) (authority ReconciliationLimaAuthority, found bool, err error) {
	err = s.View(ctx, func(tx *sql.Tx) error {
		var encoded []byte
		var operationID, planDigest string
		err := tx.QueryRowContext(ctx, `SELECT operation_id, plan_digest, authority_json
			FROM reconciliation_lima_authority WHERE box_id = ? AND generation = ?`, boxID, generation).
			Scan(&operationID, &planDigest, &encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query reconciliation Lima authority: %w", err)
		}
		decoded, err := decodeLimaAuthority(encoded)
		if err != nil {
			return err
		}
		if decoded.BoxID != boxID || decoded.Generation != generation || decoded.OperationID != operationID || string(decoded.PlanDigest) != planDigest {
			return errors.New("read reconciliation Lima authority: row identity differs from payload")
		}
		authority, found = decoded, true
		return nil
	})
	if err != nil {
		return ReconciliationLimaAuthority{}, false, err
	}
	return authority, found, nil
}

func retireReconciliationLimaAuthority(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) error {
	var encoded []byte
	err := tx.QueryRowContext(ctx, `SELECT authority_json FROM reconciliation_lima_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("publish READY generation: query partial Lima authority: %w", err)
	}
	authority, err := decodeLimaAuthority(encoded)
	if err != nil {
		return err
	}
	expectedConfigPath := filepath.Join(filepath.Dir(publication.PlanPath), "lima.yaml")
	if authority.BoxID != publication.Plan.BoxID || authority.Generation != publication.Generation ||
		authority.PlanDigest != publication.Plan.PlanDigest || authority.LimaArtifact != publication.RemovalSnapshot.LimaArtifact ||
		!reflect.DeepEqual(authority.Spec, publication.RemovalSnapshot.LimaSpec) || authority.ConfigPath != expectedConfigPath ||
		authority.ConfigEvidence == nil || authority.Instance == nil ||
		!reflect.DeepEqual(*authority.Instance, publication.RemovalSnapshot.Lima) {
		return errors.New("publish READY generation: partial Lima authority differs from removal authority")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reconciliation_lima_authority
		WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation); err != nil {
		return fmt.Errorf("publish READY generation: retire partial Lima authority: %w", err)
	}
	return nil
}

func encodeLimaAuthority(authority ReconciliationLimaAuthority) ([]byte, error) {
	if err := validateLimaAuthority(authority); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(limaAuthorityDocument{SchemaVersion: limaAuthoritySchemaVersion, ReconciliationLimaAuthority: authority})
	if err != nil {
		return nil, fmt.Errorf("encode reconciliation Lima authority: %w", err)
	}
	return encoded, nil
}

func decodeLimaAuthority(encoded []byte) (ReconciliationLimaAuthority, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document limaAuthorityDocument
	if err := decoder.Decode(&document); err != nil {
		return ReconciliationLimaAuthority{}, fmt.Errorf("decode reconciliation Lima authority: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReconciliationLimaAuthority{}, errors.New("decode reconciliation Lima authority: trailing content")
	}
	if document.SchemaVersion != limaAuthoritySchemaVersion {
		return ReconciliationLimaAuthority{}, errors.New("decode reconciliation Lima authority: unsupported schema version")
	}
	if err := validateLimaAuthority(document.ReconciliationLimaAuthority); err != nil {
		return ReconciliationLimaAuthority{}, fmt.Errorf("decode reconciliation Lima authority: %w", err)
	}
	return document.ReconciliationLimaAuthority, nil
}

func validateLimaAuthority(authority ReconciliationLimaAuthority) error {
	spec := authority.Spec
	if invalidFact(authority.BoxID) || authority.Generation < 1 || !validSHA256(authority.PlanDigest) || invalidFact(authority.OperationID) ||
		!validToolchainIdentity(authority.LimaArtifact, toolchain.Lima) || authority.LimaArtifact.Version != lima.SupportedVersion ||
		spec.BoxID != authority.BoxID || spec.PlanDigest != authority.PlanDigest || spec.Generation != authority.Generation ||
		invalidFact(spec.RuntimeHost.Driver) || invalidFact(spec.RuntimeHost.InstanceName) || spec.RuntimeHost.CPUMillicores <= 0 ||
		spec.RuntimeHost.MemoryBytes <= 0 || spec.RuntimeHost.DiskBytes <= 0 ||
		!validToolchainIdentity(spec.GuestImage, toolchain.GuestImage) || !cleanAbsolutePath(spec.ImagePath) ||
		!validPathSet(spec.PathEvidence) || !cleanAbsolutePath(authority.ConfigPath) {
		return errors.New("save reconciliation Lima authority: identity is incomplete")
	}
	if authority.ConfigEvidence != nil {
		if !validPathSet(*authority.ConfigEvidence) || len(authority.ConfigEvidence.Paths) != 1 ||
			authority.ConfigEvidence.Paths[0].CanonicalPath != authority.ConfigPath {
			return errors.New("save reconciliation Lima authority: config proof differs from intent")
		}
	}
	if authority.Instance != nil {
		generation := strconv.FormatInt(authority.Generation, 10)
		if authority.ConfigEvidence == nil || authority.Instance.Name != spec.RuntimeHost.InstanceName ||
			authority.Instance.Metadata[ownership.BoxIDFact] != authority.BoxID ||
			authority.Instance.Metadata["dev.abox.plan-digest"] != string(authority.PlanDigest) ||
			authority.Instance.Metadata["dev.abox.generation"] != generation {
			return errors.New("save reconciliation Lima authority: instance proof differs from intent")
		}
	}
	return nil
}

func validateLimaAuthorityTransition(existing, next ReconciliationLimaAuthority) error {
	existingBase, nextBase := existing, next
	existingBase.OperationID, nextBase.OperationID = "", ""
	existingBase.ConfigEvidence, nextBase.ConfigEvidence = nil, nil
	existingBase.Instance, nextBase.Instance = nil, nil
	if !reflect.DeepEqual(existingBase, nextBase) {
		return errors.New("save reconciliation Lima authority: claimed identity is immutable")
	}
	if existing.ConfigEvidence != nil && next.ConfigEvidence == nil || existing.Instance != nil && next.Instance == nil {
		return errors.New("save reconciliation Lima authority: proof cannot regress")
	}
	if existing.ConfigEvidence != nil && !reflect.DeepEqual(existing.ConfigEvidence, next.ConfigEvidence) {
		return errors.New("save reconciliation Lima authority: config proof is immutable")
	}
	if existing.Instance != nil && !reflect.DeepEqual(existing.Instance, next.Instance) {
		return errors.New("save reconciliation Lima authority: instance proof is immutable")
	}
	return nil
}
