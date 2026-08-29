<!-- sprout-task
{"schema_version":1,"id":"m3-readiness-engine","status":"open","created_at":"2026-08-29T06:48:36.380059015Z"}
-->

# Evaluate service and endpoint readiness

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Evaluate service and endpoint readiness

## Context

- Milestone 3 publication gate. Dependencies: m2-multi-repo-e2e, m3-box-gateway, m3-machine-router, m3-tcp-allocator. Source: THE_BEGINNING.md Sections 12.3 and 13.

## Requirements

- Model running, healthy, reachable, and ready as distinct facts.
- Evaluate target readiness from Compose health and direct in-Box reachability, then published readiness through the configured gateway/Lima/router or TCP proxy path with deadlines, cancellation, bounded retry, and diagnostics.
- Require a meaningful gate for each required route.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A Box never reports READY while any required gate is failing.
- [ ] Timeout produces exit code 6 semantics and an actionable symbolic error.
- [ ] Probe cancellation and retry loops leak no goroutines or sockets.
- [ ] Direct service success with a broken gateway, router, forward, or proxy never produces route-ready or port-ready output.

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
- [ ] Run readiness timing, cancellation, and unhealthy dependency tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement evaluate service and endpoint readiness for Agent Box according to the cited specification and dependencies.
