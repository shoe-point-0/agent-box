<!-- sprout-task
{"schema_version":1,"id":"m4-fault-state","status":"open","created_at":"2026-08-29T06:55:50.649128882Z"}
-->

# Inject SQLite and generated-file durability failures

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Inject SQLite and generated-file durability failures

## Context

- Milestone 4. Dependencies: m1-atomic-writer, m1-operation-journal, m1-state-store. Source: THE_BEGINNING.md Sections 15 and 17.

## Requirements

- Inject ENOSPC, write/fsync/rename/directory-fsync, transaction, WAL, backup, and generation-publication failures at every current persistence boundary; migration-specific faults remain in m5-state-compatibility.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] No completion is reported before durable commit.
- [ ] Restart sees complete old or new generations and re-observes external reality.
- [ ] Failed migration preserves and verifies the original database.

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

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement inject sqlite and generated-file durability failures for Agent Box.
