<!-- sprout-task
{"schema_version":1,"id":"m5-acceptance-matrix","status":"open","created_at":"2026-08-29T06:55:50.573332718Z"}
-->

# Define the release acceptance evidence manifest

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Define the release acceptance evidence manifest

## Context

- Milestone 5. Dependencies: m4-recovery-e2e, m5-secret-delivery. Source: THE_BEGINNING.md Sections 4.2 and 19.2.

## Requirements

- Create a machine-readable row for every hard guarantee and mandatory E2E scenario with test ID, prerequisites, platform, observed-state evidence, positive/negative assertions, cleanup, and artifact path.
- Reject missing, skipped, simulated hardware-required, or evidence-free rows.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] All nine hard guarantees and 22 scenarios have distinct release-blocking records.
- [ ] Every applicable row asserts forbidden presence and post-test leaks.
- [ ] Secret scenario evidence includes DB/WAL/SHM and streamed output bytes.

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
- [ ] Validate the acceptance manifest schema and exact row counts

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement define the release acceptance evidence manifest for Agent Box.
