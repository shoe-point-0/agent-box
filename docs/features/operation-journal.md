<!-- sprout-task
{"schema_version":1,"id":"m1-operation-journal","status":"implemented","created_at":"2026-08-29T06:48:36.016072514Z","implemented_at":"2026-08-29T22:19:43.062882161Z"}
-->

# Operation events and Box leases

Status: implemented.

The `internal/journal` package provides the transactional boundary over
`state.Store` for deterministic Workspace/Box establishment, idempotent
operation creation and reattachment, terminal state recording, monotonically
sequenced durable events, decoupled subscriber replay, and fenced Box leases.
Schema migration `v2` records canonical request digests, enforces at most one
running mutation per Box, and persists full lease-owner identity (random daemon
incarnation ID, process-start identity, boot/session identity, expiry, release,
and monotonically increasing fence counter).

Lease renewal and release compare the complete owner identity and fence so a
stale daemon can never mutate a successor lease; expired or released rows
remain as the fencing high-water mark. Event detail payloads must be JSON
objects and reject nested sensitive field names. Live Darwin/arm64 lease
fencing and detached operation continuity were subsequently exercised on real
Apple M1 hardware in [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 operation journal and lease fencing. Depends on [SQLite state store](sqlite-state-store.md). Source: `THE_BEGINNING.md` Sections 9, 14.5, 15.1, and Issue 3.

## Behavior

- Establishes deterministic Workspace and Box records and either starts one running mutation operation or reattaches a duplicate idempotency key with matching kind and request digest inside one immediate SQLite transaction, rejecting conflicting key reuse or concurrent distinct mutations with typed conflicts.
- Acquires, renews, expires, and releases one fenced lease per Box using owner incarnation ID, process-start or OS-lock identity, and boot/session identity rather than PID alone; never steals an unexpired lease and monotonically increments the fence upon reacquisition.
- Allocates monotonic `max(sequence)+1` event sequences inside serialized write transactions, rejects nested sensitive field names in event JSON payloads, and supports ordered replay after daemon restart.
- Decouples subscriber polling contexts from daemon-owned operation contexts so client detachment closes only that subscriber stream.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Existing v1 provisional leases retain their expiry and cannot be renewed without the complete v2 owner identity.

## Verified acceptance criteria

- Concurrent processes converge on one Box record and one active mutation operation.
- Dead-owner leases recover at the next fence value after expiry without stealing live leases.
- Persisted events replay in exact monotonic sequence order across SQLite restarts.
- Disconnecting a subscriber detaches only that stream and never cancels daemon-owned work.

## Implementation details

- `internal/journal` runs compare-and-update SQLite transactions over `internal/state` (`v2` schema migration verified from checked-in `v0`, `v1`, and `v2` fixtures).
- On Linux, daemon identity combines a cryptographically random incarnation ID, kernel boot ID, and `/proc` process start ticks; other build targets derive a process-session identity paired with the incarnation ID.
- [daemon API](daemon-api.md) and [`abox up`](abox-up.md) consume `internal/journal` as the durable idempotency, streaming, and lease-fencing substrate.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run cross-process lease and idempotency integration tests

## Validation evidence

- Cross-process integration tests (`TestConcurrentProcessesEstablishOneBoxAndOperation`, `TestDeadProcessLeaseExpiresWithoutLiveLeaseTheft`, `TestEventsSurviveRestartInStableConcurrentSequence`, `TestSubscriberDetachDoesNotCancelOperation`) passed: eight separate OS processes converged on one Box and operation; a live lease stayed protected until exact expiry and recovered at fence `2`; `32` concurrently appended events replayed in order after reopening SQLite; and subscriber cancellation left operation append and completion unaffected.
- `TestMigrateEverySchemaFixture` passed across checked-in `v0`, `v1`, and `v2` SQLite fixtures.
- `go test -race -count=10 ./internal/journal ./internal/state`, `make check` (`0` lint issues), `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`, and `git diff --check` passed.
- Live Apple M1 fenced lease lifecycle and detached `SIGKILL` operation completion were subsequently verified on real hardware in [`abox up`](abox-up.md).
