<!-- sprout-task
{"schema_version":1,"id":"m1-reconcile-engine","status":"open","created_at":"2026-08-29T06:48:36.162350553Z"}
-->

# Implement the operation DAG engine

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the operation DAG engine

## Context

- Milestone 1. Dependencies: m1-operation-journal, m1-plan-diff. Source: THE_BEGINNING.md Sections 9.2 and 9.3.

## Requirements

- Represent stable operation keys, dependencies, read-only observation, idempotent apply, and bounded compensation.
- Persist state transitions and before/after events around every external side effect.
- Bound parallelism for independent operations and serialize mutations within one Box.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Cycles and missing dependencies fail before apply.
- [ ] Restart after unknown partial execution re-observes instead of replaying blindly.
- [ ] Compensation touches only resources created by the current failed attempt.

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
- [ ] Run race and deterministic failure-injection tests for the DAG engine

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement the operation dag engine for Agent Box according to the cited specification and dependencies.
