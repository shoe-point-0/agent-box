<!-- sprout-task
{"schema_version":1,"id":"m6-devcontainer-import","status":"open","created_at":"2026-08-29T06:48:36.714661098Z"}
-->

# Import Dev Container configurations explicitly

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Import Dev Container configurations explicitly

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Sections 12.5 and 20 Milestone 6.

## Requirements

- Use the upstream Dev Container CLI for lifecycle and feature semantics.
- Import supported Compose-backed or standalone configurations into an inspectable generated model.
- Reject unsupported properties rather than silently approximating them.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Imported workspace service behavior matches the upstream CLI fixture.
- [ ] The adapter does not weaken Box mount, socket, or secret policy.
- [ ] No implicit auto-discovery occurs during ordinary up.

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
- [ ] Run upstream-compatible Dev Container fixture tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement import dev container configurations explicitly for Agent Box according to the cited specification and dependencies.
