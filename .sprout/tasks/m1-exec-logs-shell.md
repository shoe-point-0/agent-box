<!-- sprout-task
{"schema_version":1,"id":"m1-exec-logs-shell","status":"open","created_at":"2026-08-29T06:55:50.54221765Z"}
-->

# Implement exec, logs, and interactive shell semantics

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement exec, logs, and interactive shell semantics

## Context

- Milestone 1. Dependencies: m1-single-repo-up, m1-compose-adapter, m1-cli-foundation. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement noninteractive exec, bounded/follow logs, and PTY-backed shell with exact exit status.
- Own stream goroutines, signal forwarding, resize propagation, cancellation, and terminal restoration.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] PTY resize and signals reach the remote process.
- [ ] Terminal mode is restored on every success/error/cancel path.
- [ ] Race/stress tests leave no stream goroutine or process leak.

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

Implement implement exec, logs, and interactive shell semantics for Agent Box.
