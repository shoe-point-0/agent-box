<!-- sprout-task
{"schema_version":1,"id":"m4-concurrent-reconciliation","status":"open","created_at":"2026-08-29T06:48:36.528085117Z"}
-->

# Attach concurrent callers and serialize Box mutation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Attach concurrent callers and serialize Box mutation

## Context

- Milestone 4. Dependencies: m4-recovery-reconstruction, m1-operation-journal. Source: THE_BEGINNING.md Sections 9.2 and 19.2 scenario 8.

## Requirements

- Attach same-key up callers to one operation and stream the same ordered events.
- Allow bounded independent work across different Boxes without sharing mutable allocations.
- Define cancellation behavior for clients versus daemon-owned operations.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Concurrent same-key calls create exactly one Box and one operation.
- [ ] Different Boxes can progress concurrently without races or cross-events.
- [ ] Race detector and stress runs show no leaked goroutines, sockets, or leases.

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
- [ ] go test -race -count=10 ./...
- [ ] Run multi-process same-key and different-key stress tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement attach concurrent callers and serialize box mutation for Agent Box according to the cited specification and dependencies.
