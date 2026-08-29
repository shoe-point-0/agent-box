CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    manifest_path TEXT NOT NULL UNIQUE,
    manifest_digest TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE boxes (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    task_key TEXT NOT NULL,
    recorded_state TEXT NOT NULL,
    current_generation INTEGER NOT NULL DEFAULT 0 CHECK (current_generation >= 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    UNIQUE (workspace_id, task_key)
) STRICT;

CREATE TABLE box_generations (
    box_id TEXT NOT NULL,
    generation INTEGER NOT NULL CHECK (generation > 0),
    plan_digest TEXT NOT NULL,
    plan_path TEXT NOT NULL,
    recorded_state TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (box_id, generation),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

CREATE TABLE repositories (
    box_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_path TEXT NOT NULL,
    git_store_path TEXT NOT NULL,
    base_commit TEXT NOT NULL,
    branch TEXT NOT NULL,
    publish_destination TEXT,
    PRIMARY KEY (box_id, name),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    UNIQUE (git_store_path)
) STRICT;

CREATE TABLE worktrees (
    box_id TEXT NOT NULL,
    repository_name TEXT NOT NULL,
    path TEXT NOT NULL,
    git_common_dir TEXT NOT NULL,
    current_commit TEXT NOT NULL,
    branch TEXT NOT NULL,
    recorded_dirty INTEGER NOT NULL CHECK (recorded_dirty IN (0, 1)),
    observed_at INTEGER NOT NULL,
    PRIMARY KEY (box_id, repository_name),
    FOREIGN KEY (box_id, repository_name) REFERENCES repositories(box_id, name) ON UPDATE RESTRICT ON DELETE CASCADE,
    UNIQUE (path)
) STRICT;

CREATE TABLE runtime_hosts (
    box_id TEXT PRIMARY KEY,
    driver TEXT NOT NULL,
    instance_name TEXT NOT NULL UNIQUE,
    config_digest TEXT NOT NULL,
    recorded_state TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

CREATE TABLE compose_projects (
    box_id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL UNIQUE,
    config_digest TEXT NOT NULL,
    recorded_state TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

CREATE TABLE operations (
    id TEXT PRIMARY KEY,
    box_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    recorded_state TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    error_code TEXT,
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    UNIQUE (box_id, idempotency_key),
    UNIQUE (id, box_id)
) STRICT;

CREATE TABLE operation_events (
    box_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    kind TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    occurred_at INTEGER NOT NULL,
    PRIMARY KEY (operation_id, sequence),
    FOREIGN KEY (operation_id, box_id) REFERENCES operations(id, box_id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

CREATE TABLE leases (
    box_id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    acquired_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at > acquired_at),
    FOREIGN KEY (box_id) REFERENCES boxes(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    FOREIGN KEY (operation_id, box_id) REFERENCES operations(id, box_id) ON UPDATE RESTRICT ON DELETE CASCADE
) STRICT;

PRAGMA user_version = 1;
