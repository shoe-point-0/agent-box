ALTER TABLE operations ADD COLUMN request_digest TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX operations_one_running_per_box
    ON operations (box_id)
    WHERE recorded_state = 'running';

ALTER TABLE leases RENAME TO leases_v1;

CREATE TABLE leases (
    box_id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    process_start_id TEXT NOT NULL,
    boot_id TEXT NOT NULL,
    fencing_token INTEGER NOT NULL CHECK (fencing_token > 0),
    acquired_at INTEGER NOT NULL,
    renewed_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at > renewed_at),
    released_at INTEGER CHECK (released_at IS NULL OR released_at >= acquired_at),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    FOREIGN KEY (operation_id, box_id) REFERENCES operations(id, box_id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

INSERT INTO leases (
    box_id, operation_id, owner_id, process_start_id, boot_id,
    fencing_token, acquired_at, renewed_at, expires_at
)
SELECT
    box_id, operation_id, owner_id, 'legacy', 'legacy',
    1, acquired_at, acquired_at, expires_at
FROM leases_v1;

DROP TABLE leases_v1;
