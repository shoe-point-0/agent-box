<!-- sprout-task
{"schema_version":1,"id":"m1-plan-command","status":"open","created_at":"2026-08-29T06:55:50.479423731Z"}
-->

# Render a read-only reconciliation plan

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Render a read-only reconciliation plan

## Context

- Milestone 1 command. Dependencies: m1-plan-diff, m1-daemon-api, m1-cli-foundation, m1-repository-resolver. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement abox plan with human and JSON proposed operations based on read-only resolution and observed state.
- Expose stable conflicts and restart/destructive implications without materializing desired resources.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Running plan from an absent Box creates no private store, worktree, resource row, VM, port, or generated Box file.
- [ ] Repeated plan output is deterministic for unchanged facts.
- [ ] JSON validates against the output schema.

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

Implement render a read-only reconciliation plan for Agent Box.
