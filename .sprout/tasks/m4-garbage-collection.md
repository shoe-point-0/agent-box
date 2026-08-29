<!-- sprout-task
{"schema_version":1,"id":"m4-garbage-collection","status":"open","created_at":"2026-08-29T06:55:50.557633169Z"}
-->

# Reclaim only proven orphaned Box resources

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Reclaim only proven orphaned Box resources

## Context

- Milestone 4. Dependencies: m4-recovery-reconstruction, m4-ownership-proofs. Source: THE_BEGINNING.md Sections 6.1 and 17.

## Requirements

- Define orphan eligibility, retention, dry-run inventory, ownership prerequisites, dirty/unpublished exclusions, bounded work, cancellation, and crash recovery.
- Garbage collection must observe every resource class and never infer ownership from prefixes.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Dirty worktrees, unpublished commits, live leases, sibling resources, and ambiguous state are never reclaimed.
- [ ] Interrupted GC safely resumes or refuses.
- [ ] Dry-run exactly predicts eligible mutations unless observed facts drift.

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

Implement reclaim only proven orphaned box resources for Agent Box.
