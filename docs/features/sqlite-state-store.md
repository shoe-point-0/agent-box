# SQLite state and migrations

Status: implemented.

The `internal/state` package provides the pure-Go SQLite persistence boundary
(`modernc.org/sqlite`) with per-connection foreign-key enforcement, WAL mode,
`FULL` synchronous durability, a 5-second busy timeout, immediate write
transactions, integrity checks, restrictive owner-only database and sidecar
permissions, and transactional forward-only schema migrations.

Schema `v1` established the core Milestone 1 tables (`workspaces`, `boxes`,
`box_generations`, `repositories`, `worktrees`, `runtime_hosts`,
`compose_projects`, `operations`, `operation_events`, and `leases`) with
`box_id` composite foreign keys and explicit separation between non-authoritative
cached observations and live external reality. Subsequent forward-only
migrations evolved the same store for operation leases (`v2` in
[operation journal](operation-journal.md)), TCP port allocations (in
[TCP port allocator](tcp-port-allocator.md)), and atomic `READY` publication plus
monotonic partial recovery authority (`v4`–`v8` in
[`abox up`](abox-up.md)).

## Scope and dependencies

- Milestone 1 state store. Depends on [identity and paths](identity-and-paths.md) and [atomic file writer](atomic-file-writer.md). Source: `THE_BEGINNING.md` Sections 15.1 and 22.1.

## Behavior

- Opens the SQLite database at the path supplied by `internal/paths` with owner-only permissions on the directory, database file, and WAL/SHM sidecars.
- Configures every pooled connection via DSN pragmas for `foreign_keys=ON`, WAL journaling, `synchronous=FULL`, and a 5-second `busy_timeout`.
- Applies embedded forward-only migrations atomically per version step against `PRAGMA user_version`, rolling back both DDL and version markers on failure and refusing databases newer than the binary or failing integrity checks without replacing input files.
- Enforces `box_id` ownership through composite foreign keys so child resources cannot attach across Boxes, while naming cached observation fields explicitly as non-authoritative relative to live external observation.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Unowned raw database connections are never leaked outside bounded read and write transaction helpers.

## Verified acceptance criteria

- Fresh initialization creates the versioned schema and verifies foreign keys, WAL mode, busy timeout, durability, and restrictive DB/WAL/SHM modes on every pooled connection.
- Forward-only migration tests pass from every checked-in schema fixture.
- Foreign-key, ownership, and migration failures roll back atomically without corrupting or replacing existing databases.

## Implementation details

- `internal/state` wraps `database/sql` over pure-Go `modernc.org/sqlite`, requiring no CGo toolchain for cross-platform builds.
- Checked-in SQL fixtures (`schema-v0.sql`, `schema-v1.sql`, and subsequent version fixtures) are exercised on every test run to guard backward migration compatibility.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run migration tests against every schema fixture

## Validation evidence

- `go test -v ./internal/state -run 'Test(OpenInitializesV1AndConfiguresEveryConnection|MigrateEverySchemaFixture|OwnershipViolationRollsBackTransaction|ForeignKeyViolationRollsBackTransaction|FailedMigrationIsAtomic|OpenRejectsNewerSchema|OpenRejectsCorruptionWithoutReplacingInput)$'` passed, migrating `schema-v0.sql` and `schema-v1.sql` cleanly while verifying pooled pragmas, restrictive sidecar permissions, atomic migration rollback, corruption preservation, and transactional ownership rejection.
- `go test -race -count=10 ./internal/state`, `make check` (`0` lint issues), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`, and `git diff --check` passed.
- Real Apple Silicon (`darwin/arm64`) SQLite state durability, atomic `READY` publication, and partial authority retirement were subsequently exercised on real hardware in [`abox up`](abox-up.md).
