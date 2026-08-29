<!-- sprout-task
{"schema_version":1,"id":"m5-secret-delivery","status":"open","created_at":"2026-08-29T06:48:36.567270251Z"}
-->

# Resolve and deliver secrets without persistence

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Resolve and deliver secrets without persistence

## Context

- Milestone 5. Dependencies: m4-recovery-e2e, m1-process-runner. Source: THE_BEGINNING.md Section 16.4.

## Requirements

- Resolve host secret references only at apply time and deliver files through guest tmpfs where supported.
- Allow environment delivery only when explicitly selected and warn about in-Box inspection exposure.
- Remove temporary material on generation stop and register redaction before process output.
- Strip or reject credential-bearing repository URLs before Plan construction, never place secret values in argv, and remove stale generation material during daemon startup reconciliation after daemon or guest death.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Missing secrets prevent consumer startup instead of supplying empty values.
- [ ] Canary values and the documented transformed forms are absent from Plan, SQLite including WAL/SHM, streamed NDJSON, process errors, logs, models, Docker inspect for file delivery, and diagnostics under one-byte output chunking.
- [ ] Only declared consumer services receive each secret.

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
- [ ] Run end-to-end secret canary and lifecycle tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement resolve and deliver secrets without persistence for Agent Box according to the cited specification and dependencies.
