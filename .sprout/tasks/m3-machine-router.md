<!-- sprout-task
{"schema_version":1,"id":"m3-machine-router","status":"open","created_at":"2026-08-29T06:48:36.410369172Z"}
-->

# Route all Box HTTP traffic through one loopback listener

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Route all Box HTTP traffic through one loopback listener

## Context

- Milestone 3 unpublished transport. Dependencies: m3-box-gateway, m1-daemon-api. Source: THE_BEGINNING.md Sections 6.1 and 13.2.

## Requirements

- Own a configurable unprivileged loopback listener in aboxd and select Boxes by exact Host rules.
- Install exact per-Box forwarding entries in an unpublished/unready state, forward through stable Lima gateway allocations, and expose observations for published-path probes and restart reconstruction.
- Fail clearly when the listener is occupied without touching the owner process.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Two Boxes with identical logical route names are independently reachable.
- [ ] Unknown, malformed, or stale hosts cannot reach another Box.
- [ ] The listener binds loopback only and shuts down deterministically.

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
- [ ] Run router isolation, restart, and occupied-port integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement route all box http traffic through one loopback listener for Agent Box according to the cited specification and dependencies.
