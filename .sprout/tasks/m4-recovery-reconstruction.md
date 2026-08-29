<!-- sprout-task
{"schema_version":1,"id":"m4-recovery-reconstruction","status":"open","created_at":"2026-08-29T06:48:36.472109163Z"}
-->

# Reconstruct observed state after daemon restart

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Reconstruct observed state after daemon restart

## Context

- Milestone 4. Dependencies: m3-network-e2e. Source: THE_BEGINNING.md Section 17.1.

## Requirements

- Expire stale leases only after owner liveness checks and inspect transitional Boxes across Git, filesystem, Lima, Compose, routes, and ports.
- Classify observed state as READY, DEGRADED, STOPPED, or FAILED without destructive recovery.
- Make the next up resume from observed reality.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Matching resources are adopted without duplication.
- [ ] Mismatched ownership or immutable config produces explicit repair-required conflicts.
- [ ] Startup never automatically deletes worktrees or volumes.

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
- [ ] Run daemon-restart recovery tests from every transitional state

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement reconstruct observed state after daemon restart for Agent Box according to the cited specification and dependencies.
