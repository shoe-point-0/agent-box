<!-- sprout-task
{"schema_version":1,"id":"m1-operation-journal","status":"implemented","created_at":"2026-08-29T06:48:36.016072514Z","implemented_at":"2026-08-29T22:19:43.062882161Z"}
-->

# Implement operation events and Box leases

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement operation events and Box leases

## Context

- Milestone 1. Dependencies: m1-state-store. Source: THE_BEGINNING.md Sections 9, 14.5, 15.1, and Issue 3.

## Requirements

- Persist operations and monotonically sequenced events with idempotency keys.
- Acquire, renew, expire, and release one fenced lease per Workspace/task identity using a random daemon instance ID, process-start or OS-lock identity, and boot/session identity rather than PID alone.
- Allow concurrent callers to attach to the same active operation.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Concurrent processes establish one Box and one active mutation operation.
- [x] A dead owner lease is recoverable without stealing a live lease.
- [x] Events survive restart in stable sequence order.
- [x] Disconnecting one client detaches that subscriber and does not cancel a shared daemon-owned operation.

## Technical approach

- Add a narrow `internal/journal` boundary over `state.Store` transactions. One
  immediate transaction establishes the deterministic Workspace/Box record and
  either creates a running mutation operation or reattaches a duplicate
  idempotency key with the same kind and request digest. Conflicting reuse and a
  different active mutation are explicit typed conflicts.
- Advance the SQLite schema with a tested v2 migration. Persist request digests,
  enforce at most one running operation per Box, and replace the provisional
  lease row with owner instance, process-start, boot/session, expiry, release,
  and monotonically increasing fencing fields.
- Acquire, renew, and release leases through compare-and-update transactions.
  An unexpired lease is never stolen; an expired or explicitly released lease
  is reacquired by incrementing its fencing token. Renewal and release require
  the complete owner identity and fence, so a stale daemon cannot mutate a
  successor's lease.
- Append events by allocating `max(sequence)+1` inside the serialized write
  transaction and replay them ordered by sequence. A subscriber owns only its
  polling context; detaching closes that stream and has no cancellation path to
  the daemon-owned operation.
- Exercise duplicate operation creation and lease contention with separate test
  processes against one SQLite file, then reopen the store to prove ordered
  event replay and test subscriber detachment independently of operation state.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run cross-process lease and idempotency integration tests

## Validation evidence

- `go test -v ./internal/journal -run 'Test(ConcurrentProcessesEstablishOneBoxAndOperation|DeadProcessLeaseExpiresWithoutLiveLeaseTheft|EventsSurviveRestartInStableConcurrentSequence|SubscriberDetachDoesNotCancelOperation)$'` passed. Eight independently opened test processes converged on one Box and operation; a process-owned lease remained protected until exact expiry and was then recovered at fence 2; 32 concurrently appended events replayed in sequence after reopening SQLite; and subscriber cancellation did not prevent later event append or operation completion.
- `go test -v ./internal/state -run TestMigrateEverySchemaFixture` passed from checked-in schema fixtures v0, v1, and v2.
- `go test -race -count=10 ./internal/journal ./internal/state` passed after the final implementation and lint fixes.
- `make check` passed: pinned `golangci-lint` reported `0 issues`; module tidy and verification, all unit and race tests, vet, builds, spike smoke, and automation tests passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation evidence only; it is not Darwin runtime evidence.
- `git diff --check` passed.

## Outcome and follow-ups

- Added `internal/journal` as the transaction boundary for deterministic Box establishment, idempotent operation creation/reattachment, terminal operation state, durable events, subscriber replay, and fenced leases.
- Schema v2 records request digests, enforces one running mutation per Box, and persists owner instance, process-start, boot/session, expiry, release, and monotonically increasing fence facts. Existing v1 leases retain their expiry and cannot be renewed without the new complete identity.
- Linux daemon identity uses a cryptographically random incarnation ID, kernel boot ID, and `/proc` process start ticks. Other build targets use a process-session identity until their native runtime implementation is exercised; Darwin cross-compilation is not a support claim.
- Lease renewal and release compare the complete owner identity and fence. A released or expired lease remains as the fencing counter, so stale owners cannot mutate a successor lease.
- Event details require JSON objects and reject nested sensitive field names. Secret resolution remains responsible for never handing secret values to this boundary.

## Original request

Implement implement operation events and box leases for Agent Box according to the cited specification and dependencies.
