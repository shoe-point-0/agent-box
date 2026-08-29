package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// BoxRecord is the persisted Box identity plus the latest cached observations
// available to read-only control-plane queries. Missing component rows remain
// nil; callers must not treat database presence as live ownership proof.
type BoxRecord struct {
	ID                   string
	WorkspaceID          string
	TaskKey              string
	ManifestPath         string
	RecordedState        string
	CurrentGeneration    int64
	RuntimeRecordedState *string
	ComposeRecordedState *string
}

// BoxRecords returns a stable identity-ordered snapshot in one read
// transaction. It does not refresh observations or mutate external resources.
func (s *Store) BoxRecords(ctx context.Context) (records []BoxRecord, err error) {
	err = s.View(ctx, func(tx *sql.Tx) (returnErr error) {
		rows, err := tx.QueryContext(ctx, `SELECT
			b.id, b.workspace_id, b.task_key, w.manifest_path,
			b.recorded_state, b.current_generation,
			rh.recorded_state, cp.recorded_state
		FROM boxes AS b
		JOIN workspaces AS w ON w.id = b.workspace_id
		LEFT JOIN runtime_hosts AS rh ON rh.box_id = b.id
		LEFT JOIN compose_projects AS cp ON cp.box_id = b.id
		ORDER BY b.id`)
		if err != nil {
			return fmt.Errorf("query Box records: %w", err)
		}
		defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
		for rows.Next() {
			var record BoxRecord
			var runtimeState, composeState sql.NullString
			if err := rows.Scan(
				&record.ID, &record.WorkspaceID, &record.TaskKey, &record.ManifestPath,
				&record.RecordedState, &record.CurrentGeneration,
				&runtimeState, &composeState,
			); err != nil {
				return fmt.Errorf("scan Box record: %w", err)
			}
			if runtimeState.Valid {
				record.RuntimeRecordedState = &runtimeState.String
			}
			if composeState.Valid {
				record.ComposeRecordedState = &composeState.String
			}
			records = append(records, record)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("read Box records: %w", err)
	}
	return records, nil
}
