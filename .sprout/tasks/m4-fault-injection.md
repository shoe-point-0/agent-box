<!-- sprout-task
{"schema_version":1,"id":"m4-fault-injection","status":"open","created_at":"2026-08-29T06:48:36.539215218Z"}
-->

# Execute the lifecycle fault-injection matrix

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Execute the lifecycle fault-injection matrix

## Context

- Milestone 4 aggregate gate. Dependencies: m4-fault-git-filesystem, m4-fault-runtime-compose, m4-fault-daemon-routing, m4-fault-state, m4-targeted-repair. Source: THE_BEGINNING.md Sections 17.2 and 19.2 scenarios 8 through 17 and 19 through 22; secret scenario 18 is owned by m5-secret-delivery and m5-acceptance-matrix.

## Requirements

- Aggregate the machine-readable fault-domain matrices and retained evidence; do not implement new product behavior in this gate.
- Observe real residue and exercise reconcile or targeted repair.
- Check one-Box blast radius and post-run resource leaks.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every required fault scenario has a reproducible expected outcome.
- [ ] No interruption loses dirty work or unpublished commits through cleanup.
- [ ] Failure or repair of one Box never mutates source or sibling Boxes.

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
- [ ] Run the complete dedicated fault matrix in temporary roots

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement execute the lifecycle fault-injection matrix for Agent Box according to the cited specification and dependencies.
