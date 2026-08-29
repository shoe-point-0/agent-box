<!-- sprout-task
{"schema_version":1,"id":"m0-lima-isolation-probe","status":"open","created_at":"2026-08-29T06:48:35.845761578Z"}
-->

# Prove Lima mount isolation and parallel forwarding

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Prove Lima mount isolation and parallel forwarding

## Context

- Final Darwin hardware evidence input, implemented when a testable Darwin/arm64 artifact exists. Dependencies: m0-spike-harness. This task no longer blocks Linux implementation and feeds m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 6, 11, 13, and 19.2.

## Requirements

- Generate minimal VZ configurations for two VMs with exact mount allowlists.
- Expose identical guest ports through distinct loopback-only Machine allocations.
- Inspect live instances and prove the home directory and unrelated repositories are not mounted.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Both VMs run concurrently without forwarding collisions.
- [ ] Every forward binds loopback only and reaches the intended VM.
- [ ] Inspection proves no undeclared Machine path is visible.

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
- [ ] Run the two-VM isolation and forwarding probe

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement prove lima mount isolation and parallel forwarding for Agent Box according to the cited specification and dependencies.
