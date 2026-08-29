<!-- sprout-task
{"schema_version":1,"id":"m5-release-operations","status":"open","created_at":"2026-08-29T06:55:50.67992646Z"}
-->

# Document and dry-run release, rollback, and revocation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Document and dry-run release, rollback, and revocation

## Context

- Milestone 5. Dependencies: m5-sbom-provenance-signing. Source: THE_BEGINNING.md Sections 16.5 and 22.

## Requirements

- Define release promotion, signing custody, rollback, revocation, advisory, dependency update, and guest-image procedures.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A dry run produces the complete release manifest without publication.
- [ ] Rollback and compromised-key scenarios have tested operator steps.

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

Implement document and dry-run release, rollback, and revocation for Agent Box.
