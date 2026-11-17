<!-- sprout-task
{"schema_version":1,"id":"m1-state-store","status":"implemented","created_at":"2026-08-29T06:48:36.000249511Z","implemented_at":"2026-08-29T21:59:55.924768503Z"}
-->

# SQLite state and migrations

Status: implemented. Implementation record for `m1-state-store`.

- Added the pure-Go SQLite state boundary with per-connection foreign keys, a 5-second busy timeout, WAL, full synchronous durability, immediate write transactions, integrity checks, and user-only database and sidecar permissions.
- Added the forward-only embedded v1 migration and fixtures for every schema version from 0 through 1. The schema intentionally excludes routes, port allocations, and setup stamps until their consuming milestones.
- Box-owned resources carry `box_id`; composite foreign keys prevent cross-Box repository, operation-event, and lease attachment. Cached external observations are named and documented as non-authoritative.
- Newer or corrupt databases are refused without replacement. A failed migration leaves both schema objects and `user_version` rolled back.

## Scope and dependencies

- Milestone 1. Dependencies: m1-identity-paths, m1-atomic-writer. Source: THE_BEGINNING.md Sections 15.1 and 22.1.

## Behavior

- Implement only the Milestone 1 tables consumed by workspaces, Boxes, generations, repositories, worktrees, runtime hosts, Compose projects, operations, events, and leases; add routes, ports, and setup stamps with their consuming milestones.
- Enable foreign keys, WAL mode, explicit durability and busy handling, monotonic migrations, transactions, and Box ownership on resource rows.
- Separate state-cache facts from observed external reality.
- Make schema initialization and migration crash-safe and testable with a pure-Go driver.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Fresh initialization creates the versioned v1 schema and asserts foreign keys, WAL, busy handling, durability, and restrictive DB/WAL/SHM permissions on every connection.
- Migration tests pass from every checked-in schema fixture.
- Foreign-key and ownership violations fail transactionally.

## Implementation details

- Add an `internal/state` package backed by the pure-Go `modernc.org/sqlite`
  `database/sql` driver. Open the path supplied by the central path provider,
  restrict the owning directory and database to the Machine user, and configure
  every pooled connection through DSN pragmas for foreign keys, WAL, full
  synchronous durability, and a bounded busy timeout.
- Embed ordered, forward-only migrations. Read `PRAGMA user_version`, reject a
  database newer than this binary, and apply each pending migration in its own
  transaction so schema changes and the version marker commit atomically.
- Keep the v1 schema to the Milestone 1 consumers named above. Every resource
  row carries `box_id`; composite foreign keys prevent a child resource from
  referring to an operation or repository owned by another Box. Recorded state
  and observation timestamps are explicitly cache facts, not proof that an
  external resource still exists.
- Expose bounded read and write transaction helpers rather than leaking an
  unowned connection. Test fresh initialization, multiple pooled connections,
  permissions, rollback, foreign-key/ownership rejection, incompatible and
  corrupt inputs, and migration from every checked-in fixture.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run migration tests against every schema fixture

## Validation evidence

- `go test -v ./internal/state -run 'Test(OpenInitializesV1AndConfiguresEveryConnection|MigrateEverySchemaFixture|OwnershipViolationRollsBackTransaction|ForeignKeyViolationRollsBackTransaction|FailedMigrationIsAtomic|OpenRejectsNewerSchema|OpenRejectsCorruptionWithoutReplacingInput)$'` passed. Both checked-in fixtures (`schema-v0.sql` and `schema-v1.sql`) reached schema version 1; initialization, pooled-connection pragmas, restrictive sidecar modes, atomic migration failure, corruption preservation, and transactional ownership failures passed.
- `go test -race -count=10 ./internal/state` passed.
- `make check` passed: pinned `golangci-lint` reported `0 issues`; module tidy and verification, all unit and race tests, vet, builds, spike smoke, and automation tests passed.
- `go test -race -count=10 ./...` passed across all packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as pure-Go cross-compilation evidence only; it is not Darwin runtime evidence.
- `git diff --check` passed.
