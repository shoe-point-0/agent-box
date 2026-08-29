<!-- sprout-task
{"schema_version":1,"id":"m6-checkpoints","status":"open","created_at":"2026-08-29T06:48:36.74681925Z"}
-->

# Export explicit Git and selected-volume checkpoints

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Export explicit Git and selected-volume checkpoints

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Sections 11.3 and 23.

## Requirements

- Design opt-in checkpoints as Git bundles or patches plus explicitly selected volume archives.
- Verify restore integrity across supported versions and distinguish checkpoint from backup guarantees.
- Never depend on experimental VM snapshots.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A checkpoint restores selected source history and data into an isolated fixture.
- [ ] Corrupt or incompatible archives fail before modifying an existing Box.
- [ ] Documentation states retention, encryption, consistency, and loss limits.

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
- [ ] Run checkpoint round-trip and corruption tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement export explicit git and selected-volume checkpoints for Agent Box according to the cited specification and dependencies.
