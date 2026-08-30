CREATE TABLE removal_ownership_snapshots (
    box_id TEXT NOT NULL,
    generation INTEGER NOT NULL CHECK (generation > 0),
    plan_digest TEXT NOT NULL,
    snapshot_json TEXT NOT NULL CHECK (json_valid(snapshot_json)),
    PRIMARY KEY (box_id, generation),
    FOREIGN KEY (box_id, generation) REFERENCES box_generations(box_id, generation) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;
