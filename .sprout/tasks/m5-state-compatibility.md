<!-- sprout-task
{"schema_version":1,"id":"m5-state-compatibility","status":"open","created_at":"2026-08-29T06:48:36.626964102Z"}
-->

# Harden state and API compatibility migrations

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Harden state and API compatibility migrations

## Context

- Milestone 5. Dependencies: m4-recovery-e2e, m1-state-store, m1-daemon-api. Source: THE_BEGINNING.md Section 22.1.

## Requirements

- Version CLI/daemon API, manifest, Plan, context, output, database, and guest bundle independently.
- Implement API-range compatibility and previewable manifest/state migration behavior.
- Back up state before forward-only migration and stop mutation on corruption.
- Use SQLite's online backup mechanism, fsync and integrity-check the backup before migration, and preserve the original byte-for-byte on injected migration failure.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every released schema fixture upgrades successfully or fails without modifying the original.
- [ ] JSON fields are not removed within the promised line.
- [ ] Newer/older client-daemon combinations obey the declared compatibility matrix.

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
- [ ] Run full migration and API compatibility matrix tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement harden state and api compatibility migrations for Agent Box according to the cited specification and dependencies.
