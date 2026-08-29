<!-- sprout-task
{"schema_version":1,"id":"m2-setup-jobs","status":"open","created_at":"2026-08-29T06:48:36.322789872Z"}
-->

# Execute setup jobs as a stamped DAG

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Execute setup jobs as a stamped DAG

## Context

- Milestone 2. Dependencies: m2-multi-repo-compose, m2-cache-volumes. Source: THE_BEGINNING.md Section 12.4.

## Requirements

- Validate job dependencies and execute argv commands in declared services with bounded concurrency.
- Compute stamps from normalized command, image/build digest, declared input digests, and non-secret environment.
- Support rerunWhenChanged and everyStart generation semantics and invalidate dependents.
- Add setup-job Plan projection, setup-stamp state migration, diff rules, and context fields with golden compatibility tests.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Unchanged jobs do not rerun on a no-op up.
- [ ] Changed inputs rerun only the affected job and dependents.
- [ ] Failures block dependents, retain bounded output, and set the documented Box state.

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
- [ ] Run setup DAG, stamp invalidation, and cancellation integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement execute setup jobs as a stamped dag for Agent Box according to the cited specification and dependencies.
