<!-- sprout-task
{"schema_version":1,"id":"m1-state-store","status":"open","created_at":"2026-08-29T06:48:36.000249511Z"}
-->

# Implement SQLite state and migrations

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement SQLite state and migrations

## Context

- Milestone 1. Dependencies: m1-identity-paths, m1-atomic-writer. Source: THE_BEGINNING.md Sections 15.1 and 22.1.

## Requirements

- Implement only the Milestone 1 tables consumed by workspaces, Boxes, generations, repositories, worktrees, runtime hosts, Compose projects, operations, events, and leases; add routes, ports, and setup stamps with their consuming milestones.
- Enable foreign keys, WAL mode, explicit durability and busy handling, monotonic migrations, transactions, and Box ownership on resource rows.
- Separate state-cache facts from observed external reality.
- Make schema initialization and migration crash-safe and testable with a pure-Go driver.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Fresh initialization creates the versioned v1 schema and asserts foreign keys, WAL, busy handling, durability, and restrictive DB/WAL/SHM permissions on every connection.
- [ ] Migration tests pass from every checked-in schema fixture.
- [ ] Foreign-key and ownership violations fail transactionally.

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
- [ ] Run migration tests against every schema fixture

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement sqlite state and migrations for Agent Box according to the cited specification and dependencies.
