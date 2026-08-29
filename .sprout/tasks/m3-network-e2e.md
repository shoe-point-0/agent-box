<!-- sprout-task
{"schema_version":1,"id":"m3-network-e2e","status":"open","created_at":"2026-08-29T06:48:36.456482742Z"}
-->

# Gate collision-free routes and real readiness

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Gate collision-free routes and real readiness

## Context

- Milestone 3 gate. Dependencies: m3-box-gateway, m3-machine-router, m3-tcp-allocator, m3-readiness-engine, m3-route-commands. Source: THE_BEGINNING.md Sections 19.2 scenarios 7 and 12 and Milestone 3 exit gate.

## Requirements

- Run two concurrent Boxes exposing identically named HTTP and TCP services.
- Exercise health transitions, WebSocket, SSE, streaming, daemon restart, and stopped allocation stability.
- Assert listener scope, route isolation, and leak-free cleanup.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Both Boxes remain independently reachable with zero port, network, volume, or route collision.
- [ ] An unhealthy required dependency prevents READY and route-ready reporting.
- [ ] All listeners and forwards are reclaimed after verified removal.

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
- [ ] Run the dedicated networking/readiness E2E suite

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement gate collision-free routes and real readiness for Agent Box according to the cited specification and dependencies.
