CREATE TABLE port_allocations (
    box_id TEXT NOT NULL,
    name TEXT NOT NULL,
    service TEXT NOT NULL,
    target_port INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    machine_port INTEGER NOT NULL UNIQUE CHECK (machine_port BETWEEN 1 AND 65535),
    listener_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    lease_fence INTEGER NOT NULL CHECK (lease_fence > 0),
    ready INTEGER NOT NULL CHECK (ready IN (0, 1)),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (box_id, name),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    FOREIGN KEY (operation_id, box_id) REFERENCES operations(id, box_id) ON UPDATE RESTRICT ON DELETE RESTRICT
) STRICT;
