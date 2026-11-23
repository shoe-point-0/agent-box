# SQLite and generated-file durability failures

Status: implemented.

This verification suite provides deterministic durability-fault coverage across
Agent Box's three current persistence boundaries—`internal/atomicfile`
generated-file replacement, `internal/state` SQLite WAL and transaction
commits, and `internal/up` final `READY` generation publication—without adding
production fault-injection switches.

Restart recovery is verified to observe either complete old or complete new
generated artifacts, rolled-back `SQLITE_FULL` transactions with intact
`quick_check(1)` integrity, and mandatory fresh external observation before a
retried generation can commit as `READY`. Migration-specific fault injection
and any future database backup implementation remain
[Milestone 5 compatibility work](../../ROADMAP.md#milestone-5-releasable-v01-candidate); Agent Box currently has no database backup feature
and explicitly does not treat SQLite WAL files as backups.

## Scope and dependencies

- Milestone 4 durability fault testing. Depends on [atomic file writer](atomic-file-writer.md), [operation journal](operation-journal.md), and [SQLite state store](sqlite-state-store.md). Source: `THE_BEGINNING.md` Sections 15 and 17.

## Behavior

- Exercises `ENOSPC`, partial-write, file-fsync, rename, and directory-fsync failures across `internal/atomicfile`, verifying typed pre/post-rename commit status (`Committed`), complete old or complete new file bytes, and zero temporary-file residue.
- Forces real SQLite durability failures without production fault flags: triggers `SQLITE_FULL` via `PRAGMA max_page_count`, blocks WAL creation with a hostile `state.db-wal` directory, and validates failed migrations with `PRAGMA quick_check(1)` after close and reopen.
- Injects final `READY` publication failure via an aborting SQLite trigger across `ReadyStore` in `internal/up`, verifying full generation rollback and requiring fresh live readiness observation on retry after reopen before `READY` commits.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- No production fault-injection switches are added to persistence code, and no backup semantics are claimed before a backup surface exists.

## Verified acceptance criteria

- No operation reports completion before durable commit succeeds.
- Restart observes only complete old or complete new generations and re-observes live external state before retrying publication.
- Failed migrations and `SQLITE_FULL` writes preserve original schema, data, and `quick_check(1)` integrity.

## Implementation details

- `internal/atomicfile` extends its deterministic fault matrix with explicit Linux `ENOSPC` injection on both write and file-fsync paths.
- `internal/state` uses native SQLite constraints (`max_page_count`), hostile filesystem paths (`state.db-wal`), and post-reopen `PRAGMA quick_check(1)` assertions rather than mock databases.
- `internal/up` exercises `Store.PublishReady` rollback and restart re-observation through an aborting SQLite trigger on the final Box `READY` update.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- On Linux/x86_64 with Go `1.26.3`, `go test -race -count=10 ./internal/atomicfile ./internal/state ./internal/up -run 'Test(WriterFailureLeavesCompleteVersion|SQLiteFullRollsBackAndSurvivesRestart|OpenFailsClosedWhenWALCannotBeCreated|FailedMigrationIsAtomic|ReconcileReadyRetriesFailedPublicationAfterRestartWithFreshObservation)$'` passed:
  - `TestWriterFailureLeavesCompleteVersion`: returned complete old bytes on partial-write, write-`ENOSPC`, file-fsync, file-fsync-`ENOSPC`, and rename failures, and complete new bytes with `Committed=true` on directory-fsync failure, with zero temporary-file residue.
  - `TestSQLiteFullRollsBackAndSurvivesRestart`: forced `SQLITE_FULL` via `max_page_count`, reopened WAL cleanly with zero candidate rows, and returned `ok` from `quick_check(1)`.
  - `TestOpenFailsClosedWhenWALCannotBeCreated`: failed closed when a directory occupied `state.db-wal` and left the directory untouched.
  - `TestReconcileReadyRetriesFailedPublicationAfterRestartWithFreshObservation`: rolled back the full generation transaction when an aborting SQLite trigger fired at final `READY` update, left zero `READY` rows, and required a fresh readiness-gate pass with updated live health observations after reopen before committing.
  - `TestFailedMigrationIsAtomic`: preserved original conflicting schema and sentinel data with no partial `boxes` table or `user_version` advance and returned `ok` from `quick_check(1)`.
- `make check` (`0` lint issues across all packages, builds, smoke, and automation checks) and `git diff --check` passed.
