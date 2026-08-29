<!-- sprout-task
{"schema_version":1,"id":"m5-release-supply-chain","status":"open","created_at":"2026-08-29T06:48:36.65650569Z"}
-->

# Produce signed reproducible release artifacts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Produce signed reproducible release artifacts

## Context

- Milestone 5 aggregate gate. Dependencies: m5-artifact-build, m5-sbom-provenance-signing, m5-release-operations. Source: THE_BEGINNING.md Sections 16.5 and 22.3.

## Requirements

- Aggregate reproducibility, SBOM, provenance, vulnerability, signing, verification, rollback, revocation, and release-operations evidence without implementing those concerns again.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Two clean builds produce matching artifacts or documented platform-specific exceptions.
- [ ] SBOM and provenance correspond to shipped binaries and guest metadata.
- [ ] Release fails on unwaived vet, race, vulnerability, signing, or verification errors.

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
- [ ] Run govulncheck ./...
- [ ] Run the release pipeline in dry-run mode and verify artifacts

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement produce signed reproducible release artifacts for Agent Box according to the cited specification and dependencies.
