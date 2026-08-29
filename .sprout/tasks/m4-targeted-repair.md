<!-- sprout-task
{"schema_version":1,"id":"m4-targeted-repair","status":"open","created_at":"2026-08-29T06:48:36.503178491Z"}
-->

# Implement dry-run targeted repair commands

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement dry-run targeted repair commands

## Context

- Milestone 4. Dependencies: m4-ownership-proofs. Source: THE_BEGINNING.md Section 17.4.

## Requirements

- Implement separate worktree, runtime-host, route, and state reconstruction repairs.
- Default to dry-run planning with exact ownership proof and proposed mutation.
- Refuse ambiguous reconstruction rather than guessing resource identity.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Each repair changes only its declared resource class.
- [ ] Dry-run and apply plans match unless observed reality changes, which forces replan.
- [ ] State reconstruction preserves every external resource on uncertainty.

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
- [ ] Run repair dry-run/apply and changed-reality integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement dry-run targeted repair commands for Agent Box according to the cited specification and dependencies.
