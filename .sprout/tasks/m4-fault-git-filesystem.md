<!-- sprout-task
{"schema_version":1,"id":"m4-fault-git-filesystem","status":"open","created_at":"2026-08-29T06:55:50.617615619Z"}
-->

# Inject Git and filesystem lifecycle failures

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Inject Git and filesystem lifecycle failures

## Context

- Milestone 4. Dependencies: m4-ownership-proofs, m1-atomic-writer. Source: THE_BEGINNING.md Sections 17 and 19.

## Requirements

- Kill and fault source resolution, private-store creation, worktree add/lock, mount validation, file persistence, removal preflight, and deletion boundaries.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every partial state automatically converges, safely refuses, or is classified repair-only by a checked-in matrix.
- [ ] Dirty and unpublished data survive every non-explicit destructive path.
- [ ] No outside path is mutated.

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

Implement inject git and filesystem lifecycle failures for Agent Box.
