<!-- sprout-task
{"schema_version":1,"id":"m1-reconcile-engine","status":"implemented","created_at":"2026-08-29T06:48:36.162350553Z","implemented_at":"2026-08-29T22:27:29.168605862Z"}
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

- [x] Cycles and missing dependencies fail before apply.
- [x] Restart after unknown partial execution re-observes instead of replaying blindly.
- [x] Compensation touches only resources created by the current failed attempt.

## Technical approach

- Add an `internal/reconcile` package around the Section 9.2 operation seam. An
  observation explicitly reports whether the resource is ready; an apply result
  explicitly reports whether this attempt created a resource and is therefore
  eligible for compensation.
- Validate all stable keys, duplicate keys, missing dependencies, and cycles
  before invoking `Observe` or any mutating method. Execute deterministic
  topological batches with a configured positive fan-out bound.
- Re-observe every operation on every execution. Never infer readiness from a
  prior journal event, so an interrupted `apply-started` transition causes the
  next execution to inspect external reality before deciding whether to apply.
- Persist typed before/after events through the existing operation journal for
  every `Apply` and `Compensate` call. Treat failure to persist a before-event as
  a hard stop before mutation and failure to persist an after-event as unknown
  execution requiring future re-observation.
- Track successful `Result.Created` values only in the current `Execute` call.
  On failure, compensate that set in reverse completion order; never compensate
  pre-existing or merely observed resources.
- Keep one engine execution scoped to one already-journaled Box mutation. The
  journal's active-operation constraint and fenced Box lease serialize daemon
  mutation attempts; DAG fan-out applies only within that owned attempt.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run race and deterministic failure-injection tests for the DAG engine

## Validation evidence

- `go test -race -count=10 ./internal/reconcile` passed. This repeatedly
  exercised graph rejection before observation, unknown-partial restart
  re-observation, bounded fan-out, serialized concurrent executions, recorder
  failure before mutation, partial-create compensation, current-attempt-only
  compensation, and bounded cleanup after request cancellation.
- `go test -race -count=10 ./...` passed across every Go package after closure.
- `make check` passed: golangci-lint reported `0 issues`; module tidiness and
  verification, `go test ./...`, `go test -race ./...`, `go vet ./...`, Linux
  builds, spike smoke, shell syntax, and Python automation tests all passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime support claim is made.
- The journal integration test queried the real SQLite-backed operation journal
  and verified ordered `reconcile.apply-started` and
  `reconcile.apply-succeeded` events with secret-free stable metadata.

## Outcome and follow-ups

Implemented `internal/reconcile` as a small operation-DAG execution boundary.
The engine validates the entire graph before callbacks, always observes before
apply, executes deterministic bounded batches, serializes executions on one
engine, brackets mutations and compensation with durable recorder events, and
uses a fresh timeout-bounded context to compensate only results marked created
by the current call. Box-wide cross-engine exclusion remains the caller's
existing fenced journal lease responsibility, which is explicit in the API.

## Original request

Implement implement the operation dag engine for Agent Box according to the cited specification and dependencies.
