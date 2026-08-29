<!-- sprout-task
{"schema_version":1,"id":"m5-artifact-build","status":"open","created_at":"2026-08-29T06:55:50.659372102Z"}
-->

# Build reproducible release artifacts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Build reproducible release artifacts

## Context

- Milestone 5. Dependencies: m5-toolchain-bootstrap, m5-state-compatibility. Source: THE_BEGINNING.md Sections 16.5 and 22.3.

## Requirements

- Build versioned macOS binaries and guest metadata from pinned inputs in clean environments.
- Continuously cross-build Darwin/arm64 on Linux, but label it unvalidated until m5-darwin-arm64-hardware-gate installs and executes it on real Apple Silicon.
- Record checksums, toolchains, source identity, and reproducibility exceptions.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Two clean builds match bit-for-bit or fail with a documented, reviewed cause.
- [ ] Artifacts contain correct versions and no development credentials.

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

Implement build reproducible release artifacts for Agent Box.
