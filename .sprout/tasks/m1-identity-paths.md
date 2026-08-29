<!-- sprout-task
{"schema_version":1,"id":"m1-identity-paths","status":"open","created_at":"2026-08-29T06:48:35.984103316Z"}
-->

# Implement deterministic identity and platform paths

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement deterministic identity and platform paths

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 3, 8, and 15.

## Requirements

- Derive stable Workspace and Box IDs from canonical inputs using the specified hash and slug contract.
- Centralize macOS application, cache, log, runtime, Box, Git, and workspace paths behind a path provider.
- Validate task keys and path components before filesystem use.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Repeated and randomized runs produce byte-identical IDs.
- [ ] Same task keys in unrelated Workspaces cannot collide.
- [ ] No package outside the path provider constructs state paths ad hoc.

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
- [ ] Run identity golden and collision-property tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement deterministic identity and platform paths for Agent Box according to the cited specification and dependencies.
