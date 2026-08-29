<!-- sprout-task
{"schema_version":1,"id":"m1-operation-journal","status":"open","created_at":"2026-08-29T06:48:36.016072514Z"}
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

- [ ] Concurrent processes establish one Box and one active mutation operation.
- [ ] A dead owner lease is recoverable without stealing a live lease.
- [ ] Events survive restart in stable sequence order.
- [ ] Disconnecting one client detaches that subscriber and does not cancel a shared daemon-owned operation.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run cross-process lease and idempotency integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement operation events and box leases for Agent Box according to the cited specification and dependencies.
