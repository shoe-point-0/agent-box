<!-- sprout-task
{"schema_version":1,"id":"m1-machine-preflight","status":"open","created_at":"2026-08-29T06:55:50.40048004Z"}
-->

# Validate Machine prerequisites before mutation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Validate Machine prerequisites before mutation

## Context

- Milestone 1. Dependencies: m1-identity-paths, m1-toolchain-contract. Source: THE_BEGINNING.md operation 2 and Section 17.

## Requirements

- Read-only validate supported OS/architecture/accelerator, free space, state/runtime paths, router availability, verified tool versions, and artifact integrity.
- Return stable prerequisite errors and exact observed versions without downloading or provisioning.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every failure occurs before Git, database resource rows, VM, ports, or generated Box files are created.
- [ ] Unsupported and missing prerequisites produce actionable doctor-compatible facts.
- [ ] The check is deterministic against injected platform observations.

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

Implement validate machine prerequisites before mutation for Agent Box.
