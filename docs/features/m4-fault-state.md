<!-- sprout-task
{"schema_version":1,"id":"m4-fault-state","status":"implemented","created_at":"2026-08-29T06:55:50.649128882Z","implemented_at":"2026-08-30T08:57:16.578054754Z"}
-->

# SQLite and generated-file durability failures

Status: implemented. Implementation record for `m4-fault-state`.

- Added deterministic Linux durability-fault coverage at the shared generated
  file primitive, SQLite WAL/transaction boundary, and final READY generation
  publication boundary without adding production fault switches.
- Restart now has test evidence for complete old/new generated artifacts,
  rolled-back `SQLITE_FULL` writes, and fresh external observation before a
  retried generation can become READY.
- Migration-specific injection and any future database backup implementation
  remain in `m5-state-compatibility`; this task neither implements nor claims
  backup semantics.

## Scope and dependencies

- Milestone 4. Dependencies: m1-atomic-writer, m1-operation-journal, m1-state-store. Source: THE_BEGINNING.md Sections 15 and 17.

## Behavior

- Inject ENOSPC, write/fsync/rename/directory-fsync, transaction, WAL, backup, and generation-publication failures at every current persistence boundary; migration-specific faults remain in m5-state-compatibility.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- No completion is reported before durable commit.
- Restart sees complete old or new generations and re-observes external reality.
- Failed migration preserves and verifies the original database.

## Implementation details

- Exercise the shared `internal/atomicfile` boundary with explicit Linux
  `ENOSPC` failures in addition to its existing partial-write, file-fsync,
  rename, and directory-fsync matrix. Assert the typed pre/post-rename commit
  state, complete old-or-new bytes, and temporary-file cleanup.
- Exercise SQLite itself rather than adding production-only fault switches:
  force `SQLITE_FULL` through `max_page_count`, force WAL initialization to
  fail with a hostile `state.db-wal` path, and verify failed write transactions
  survive close/reopen without their candidate rows. The store's `FULL`
  synchronous WAL commit remains the completion boundary.
- Inject final READY publication failure through the existing `ReadyStore`
  seam, then retry as a restarted reconciliation with changed live runtime
  evidence. Require the readiness gate to run again and require no READY row
  before the successful durable retry.
- Strengthen the existing failed-migration assertion with `quick_check` and
  the original schema/data after reopen. Migration implementation and
  migration-specific fault hooks remain owned by `m5-state-compatibility`.
- There is no database backup operation in the current product persistence
  surface, so there is no backup boundary to fault in this task; do not invent
  backup semantics or imply that WAL files are backups.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- On Linux/x86_64 with Go 1.26.3, `make check` passed. The pinned
  `golangci-lint` reported `0 issues`; module verification, `go test ./...`,
  `go test -race ./...`, `go vet ./...`, product/repository builds, spike smoke,
  shell validation, and automation tests all passed.
- `go test -race -count=10 ./internal/atomicfile ./internal/state ./internal/up
  -run 'Test(WriterFailureLeavesCompleteVersion|SQLiteFullRollsBackAndSurvivesRestart|OpenFailsClosedWhenWALCannotBeCreated|FailedMigrationIsAtomic|ReconcileReadyRetriesFailedPublicationAfterRestartWithFreshObservation)$'`
  passed. The atomic-file matrix returned complete old bytes for partial-write,
  write-`ENOSPC`, file-fsync, file-fsync-`ENOSPC`, and rename failures; the
  directory-fsync failure returned complete new bytes with `Committed=true`.
- `TestSQLiteFullRollsBackAndSurvivesRestart` forced real SQLite
  `SQLITE_FULL` with `max_page_count`, closed and reopened the WAL database,
  found zero candidate rows, and received `ok` from `quick_check(1)`.
  `TestOpenFailsClosedWhenWALCannotBeCreated` placed a directory at the exact
  WAL path; open failed and preserved that hostile path unchanged.
- `TestReconcileReadyRetriesFailedPublicationAfterRestartWithFreshObservation`
  injected an aborting SQLite trigger at final Box READY update. The complete
  generation transaction rolled back, leaving no READY generation; after
  close/reopen and trigger removal, a changed live health observation passed
  through the readiness gate again and the generation committed successfully.
- `TestFailedMigrationIsAtomic` reopened the rejected database, retained the
  original conflicting schema and sentinel row, observed no partial `boxes`
  table or version advance, and received `ok` from `quick_check(1)`.
- `git diff --check` passed. No backup fault was claimed: the current product
  has no database backup operation, and WAL is explicitly not treated as one.
