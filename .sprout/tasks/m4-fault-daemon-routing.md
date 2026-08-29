<!-- sprout-task
{"schema_version":1,"id":"m4-fault-daemon-routing","status":"open","created_at":"2026-08-29T06:55:50.638709307Z"}
-->

# Inject daemon, lease, router, and port failures

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Inject daemon, lease, router, and port failures

## Context

- Milestone 4. Dependencies: m4-concurrent-reconciliation, m3-network-e2e. Source: THE_BEGINNING.md Sections 17 and 19.

## Requirements

- Kill around lease commits, socket bind, route install, TCP allocation/forward, context publication, and event streaming.
- Race candidate-port theft, daemon singleton startup, and client disconnects.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] No split-brain daemon or duplicate allocation occurs.
- [ ] Restart reconstructs or explicitly reallocates without routing to an occupying process.
- [ ] Shared operations survive individual client detachment.

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

Implement inject daemon, lease, router, and port failures for Agent Box.
