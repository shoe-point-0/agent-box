<!-- sprout-task
{"schema_version":1,"id":"m3-tcp-allocator","status":"open","created_at":"2026-08-29T06:48:36.42631655Z"}
-->

# Allocate and forward stable raw TCP endpoints

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Allocate and forward stable raw TCP endpoints

## Context

- Milestone 3 transport. Dependencies: m1-operation-journal, m1-lima-adapter. Source: THE_BEGINNING.md Section 13.3.

## Requirements

- Own a daemon-held loopback listener for each allocation and proxy to the Box target so reservation and publication use the same kernel object; if direct Lima binding is retained, prove an atomic descriptor handoff before implementation.
- Persist allocation ownership under the Box lease and keep the listener bound while stopped; detect occupied recorded ports after daemon or Machine restart and refuse or explicitly reallocate with an atomic context update.
- Verify target connectivity and recover allocations after restart without races.
- Add TCP allocation Plan/context projection, state migration, and diff rules in the same reviewable slice.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Concurrent allocations cannot receive the same Machine port.
- [ ] No allocation is published before its target is reachable.
- [ ] Removal releases only the selected Box allocation.

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
- [ ] Run allocator race, restart, exhaustion, and TCP reachability tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement allocate and forward stable raw tcp endpoints for Agent Box according to the cited specification and dependencies.
