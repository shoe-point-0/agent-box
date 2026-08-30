CREATE TABLE reconciliation_lima_authority (
    box_id TEXT NOT NULL,
    generation INTEGER NOT NULL CHECK (generation > 0),
    operation_id TEXT NOT NULL,
    plan_digest TEXT NOT NULL,
    authority_json TEXT NOT NULL CHECK (json_valid(authority_json)),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (box_id, generation),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    FOREIGN KEY (operation_id, box_id) REFERENCES operations(id, box_id) ON UPDATE RESTRICT ON DELETE RESTRICT
) STRICT;
