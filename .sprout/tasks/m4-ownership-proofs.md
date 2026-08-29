<!-- sprout-task
{"schema_version":1,"id":"m4-ownership-proofs","status":"open","created_at":"2026-08-29T06:48:36.487333289Z"}
-->

# Centralize destructive ownership proofs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Centralize destructive ownership proofs

## Context

- Milestone 4. Dependencies: m4-recovery-reconstruction. Source: THE_BEGINNING.md Sections 15.3 and 16.

## Requirements

- Harden the m1-ownership-core contract with second-factor proofs, stale-row and forged-label cases, descriptor-relative or equivalent TOCTOU-resistant deletion, and cross-adapter conformance tests without redesigning its public proof types.
- Verify path containment plus Git registration/lock, opaque runtime IDs plus labels, and Lima identity plus embedded metadata.
- Return inspectable proof failures without policy decisions inside adapters.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every destructive adapter call requires validated ownership evidence.
- [ ] Forged names, labels, paths, symlinks, and stale database rows cannot authorize deletion.
- [ ] Tests cover TOCTOU-sensitive path and identity changes.

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
- [ ] Run adversarial ownership and path-race tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement centralize destructive ownership proofs for Agent Box according to the cited specification and dependencies.
