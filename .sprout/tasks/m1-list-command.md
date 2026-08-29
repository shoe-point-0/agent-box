<!-- sprout-task
{"schema_version":1,"id":"m1-list-command","status":"open","created_at":"2026-08-29T06:55:50.494935974Z"}
-->

# List Boxes from observed and recorded state

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

List Boxes from observed and recorded state

## Context

- Milestone 1 command. Dependencies: m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement abox list in human and JSON forms with identity, state, task, workspace, and observed drift summary.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Empty, active, stopped, degraded, and stale-record cases render deterministically.
- [ ] Listing is read-only and never reconciles or deletes resources.
- [ ] Output validates against the versioned schema.

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

Implement list boxes from observed and recorded state for Agent Box.
