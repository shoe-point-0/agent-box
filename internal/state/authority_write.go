package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// requireAuthorityWrite checks eligibility in the transaction that will persist the claim.
func requireAuthorityWrite(ctx context.Context, tx *sql.Tx, resource, boxID, operationID string, generation int64) error {
	var currentGeneration int64
	var kind, state string
	err := tx.QueryRowContext(ctx, `SELECT b.current_generation, o.kind, o.recorded_state
		FROM boxes AS b JOIN operations AS o ON o.box_id = b.id
		WHERE b.id = ? AND o.id = ?`, boxID, operationID).Scan(&currentGeneration, &kind, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("save reconciliation %s authority: Box operation is absent", resource)
	}
	if err != nil {
		return fmt.Errorf("save reconciliation %s authority: query Box operation: %w", resource, err)
	}
	if kind != "up" || state != "running" {
		return fmt.Errorf("save reconciliation %s authority: running up operation is required", resource)
	}
	if generation != currentGeneration && generation != currentGeneration+1 {
		return fmt.Errorf("save reconciliation %s authority: generation is not current or next", resource)
	}
	return nil
}
