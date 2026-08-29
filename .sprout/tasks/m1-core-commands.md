<!-- sprout-task
{"schema_version":1,"id":"m1-core-commands","status":"open","created_at":"2026-08-29T06:48:36.218899269Z"}
-->

# Deliver status and down

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Deliver status and down

## Context

- Milestone 1. Dependencies: m1-single-repo-up. Source: THE_BEGINNING.md Section 14.1 and Milestone 1.

## Requirements

- Implement status and down as separate daemon use cases using live observation and refusal-first lifecycle rules.
- Preserve stopped-Box identity, worktrees, Git stores, allocations, and volumes.
- Observe reality before reporting status and distinguish READY, DEGRADED, STOPPED, and FAILED.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Status and down work in human and JSON modes.
- [ ] Down stops services and VM without deleting worktrees or volumes.
- [ ] Up after down returns the same Box to READY.

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
- [ ] Run command integration tests against a live single-repository Box

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement deliver status, exec, logs, down, and shell for Agent Box according to the cited specification and dependencies.
