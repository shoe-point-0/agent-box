<!-- sprout-task
{"schema_version":1,"id":"m0-failure-matrix","status":"open","created_at":"2026-08-29T06:48:35.892187635Z"}
-->

# Record crash residue at substrate boundaries

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Record crash residue at substrate boundaries

## Context

- Milestone 0 Linux implementation gate. Dependencies: m0-git-path-probe, m0-compose-probe, m0-routing-probe. Darwin VZ and VirtioFS kill points move to m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 9, 17, and 19.2.

## Requirements

- Add deterministic kill points before and after each Linux-exercisable Git, Compose, routing, and cleanup side effect; preserve the same operation IDs for later Darwin Lima/VirtioFS evidence.
- Inventory observed resources after each killed run without trusting the harness journal alone.
- Demonstrate adoption or safe refusal and deterministic cleanup for every residue class.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] The matrix records expected and observed residue for every kill point.
- [ ] Recovery never deletes dirty worktrees, volumes, or resources lacking ownership proof.
- [ ] A rerun converges or emits an actionable conflict instead of duplicating resources.

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
- [ ] Execute the complete substrate kill-point matrix

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement record crash residue at substrate boundaries for Agent Box according to the cited specification and dependencies.
