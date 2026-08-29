<!-- sprout-task
{"schema_version":1,"id":"m5-sbom-provenance-signing","status":"open","created_at":"2026-08-29T06:55:50.669940178Z"}
-->

# Generate and verify SBOM, provenance, and signatures

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Generate and verify SBOM, provenance, and signatures

## Context

- Milestone 5. Dependencies: m5-artifact-build. Source: THE_BEGINNING.md Sections 16.5 and 22.3.

## Requirements

- Generate SBOM and provenance for shipped binaries and guest metadata, scan vulnerabilities, sign artifacts, and verify before publication.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Release fails on missing/mismatched SBOM, provenance, signature, checksum, or unwaived vulnerability.
- [ ] Verification works from a clean consumer environment.

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

Implement generate and verify sbom, provenance, and signatures for Agent Box.
