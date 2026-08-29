<!-- sprout-task
{"schema_version":1,"id":"m1-plan-diff","status":"open","created_at":"2026-08-29T06:48:36.078727449Z"}
-->

# Classify Plan changes and conflicts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Classify Plan changes and conflicts

## Context

- Milestone 1. Dependencies: m1-plan-compiler. Source: THE_BEGINNING.md Sections 9.4 and Issue 6.

## Requirements

- Compare current and desired Plans into no-op, in-place, restart, growth, removal, and conflict changes.
- Reject disk shrink and base-ref or branch replacement for dirty existing worktrees.
- Expose deterministic human and machine-readable change reasons.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every reconcile-change row in Section 9.4 has a test.
- [ ] No-op classification is stable and allocation-free enough for the warm path target.
- [ ] Conflicts never mutate observed resources.

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
- [ ] Run Plan diff golden and property tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement classify plan changes and conflicts for Agent Box according to the cited specification and dependencies.
