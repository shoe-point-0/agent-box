<!-- sprout-task
{"schema_version":1,"id":"m1-process-runner","status":"open","created_at":"2026-08-29T06:48:35.952766065Z"}
-->

# Implement the safe subprocess runner

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the safe subprocess runner

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 18.1 and Issue 4.

## Requirements

- Execute argv arrays with explicit environment allowlists, working directories, context cancellation, separate bounded-capture and streaming sinks, timing, and typed exit, cancellation, sink, and output-limit errors.
- Terminate owned process groups on cancellation and bound shutdown waits.
- Register a fixed documented, size-bounded set of exact and transformed secret forms before process start and redact statefully across arbitrary stdout/stderr chunk boundaries.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Shell metacharacters remain literal arguments.
- [ ] Cancellation leaves no owned descendant process running.
- [ ] Canary secrets never appear in captured output, errors, events, or logs.
- [ ] One-byte output chunking, overlapping secrets, blocked readers, sink failure, partial output, and cancellation races leave no secret, pipe goroutine, or descendant process behind.

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
- [ ] Run focused process cancellation and redaction stress tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement the safe subprocess runner for Agent Box according to the cited specification and dependencies.
