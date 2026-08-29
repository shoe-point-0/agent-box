<!-- sprout-task
{"schema_version":1,"id":"m6-alternate-engine-evaluation","status":"open","created_at":"2026-08-29T06:55:50.589210975Z"}
-->

# Evaluate alternate VM or container engines without implementation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Evaluate alternate VM or container engines without implementation

## Context

- Milestone 6 deferred. Dependencies: m6-ide-integrations. Source: THE_BEGINNING.md Section 20 candidate 7.

## Requirements

- Define measured requirements, compatibility fixtures, ownership/lifecycle invariants, migration cost, and upstream posture for any proposed alternate engine.
- Produce a go/no-go decision before creating a public plugin API or second implementation.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Evaluation uses the existing acceptance fixtures and real candidate versions.
- [ ] No engine abstraction is added without two implementations needing the same seam.
- [ ] Decision records support, gaps, and maintenance cost without committing implementation.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] Review the evidence-backed engine evaluation ADR

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement evaluate alternate vm or container engines without implementation for Agent Box.
