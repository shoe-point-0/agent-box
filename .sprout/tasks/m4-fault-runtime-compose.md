<!-- sprout-task
{"schema_version":1,"id":"m4-fault-runtime-compose","status":"open","created_at":"2026-08-29T06:55:50.628313797Z"}
-->

# Inject Lima, guest, and Compose lifecycle failures

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Inject Lima, guest, and Compose lifecycle failures

## Context

- Milestone 4. Dependencies: m4-recovery-reconstruction, m1-compose-policy-core. Source: THE_BEGINNING.md Sections 17 and 19.

## Requirements

- Kill before/after VM create/start, guest provisioning, Compose pull/build/create/start, health, stop, and remove side effects.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Restart adopts matching resources without duplicates.
- [ ] Mismatched immutable config is refused.
- [ ] Observed containers, networks, volumes, labels, mounts, and listeners match expected residue.

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

Implement inject lima, guest, and compose lifecycle failures for Agent Box.
