<!-- sprout-task
{"schema_version":1,"id":"m5-observability-performance","status":"open","created_at":"2026-08-29T06:48:36.642296297Z"}
-->

# Instrument and benchmark user-visible runtime behavior

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Instrument and benchmark user-visible runtime behavior

## Context

- Milestone 5. Dependencies: m4-recovery-e2e. Source: THE_BEGINNING.md Sections 18.1 and 19.3.

## Requirements

- Add structured logs with stable operation, Box, generation, adapter, timing, and error keys without secrets.
- Benchmark warm up, daemon overhead, routes, filesystem visibility, watchers, status, and restart using documented methodology.
- Profile before optimizing and preserve bounded logging/output behavior.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Published results include hardware, OS, versions, fixtures, samples, and percentile method.
- [ ] No performance target is presented as a guarantee unless measured in release CI.
- [ ] Warm no-op traces identify every remaining external observation cost.

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
- [ ] Run release benchmark suite and archive raw machine-readable results

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement instrument and benchmark user-visible runtime behavior for Agent Box according to the cited specification and dependencies.
