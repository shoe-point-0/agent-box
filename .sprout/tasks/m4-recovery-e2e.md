<!-- sprout-task
{"schema_version":1,"id":"m4-recovery-e2e","status":"open","created_at":"2026-08-29T06:48:36.552103895Z"}
-->

# Gate recovery, repair, and safety behavior

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Gate recovery, repair, and safety behavior

## Context

- Milestone 4 gate. Dependencies: m4-recovery-reconstruction, m4-ownership-proofs, m4-targeted-repair, m4-diagnostics, m4-concurrent-reconciliation, m4-fault-git-filesystem, m4-fault-runtime-compose, m4-fault-daemon-routing, m4-fault-state, m4-fault-injection, m4-garbage-collection. Source: THE_BEGINNING.md Milestone 4 exit gate.

## Requirements

- Run the complete interrupted lifecycle suite on Linux and preserve the same operation/evidence identifiers for the final Darwin/arm64 hardware rerun.
- Validate recovery from Machine reboot simulation, VM disk loss, and state corruption refusal.
- Publish residue and cleanup evidence for each phase.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every recoverable partial execution converges on the next up without duplication or unrelated deletion; only checked-in mismatch or corruption classes may require a named narrow repair.
- [ ] Dirty and unpublished work survives all non-explicit destructive paths.
- [ ] Leak checks pass after each fixture.

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
- [ ] Run the dedicated Milestone 4 recovery E2E suite

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement gate recovery, repair, and safety behavior for Agent Box according to the cited specification and dependencies.
