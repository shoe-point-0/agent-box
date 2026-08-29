<!-- sprout-task
{"schema_version":1,"id":"m5-toolchain-bootstrap","status":"open","created_at":"2026-08-29T06:48:36.611595797Z"}
-->

# Download and verify pinned runtime components

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Download and verify pinned runtime components

## Context

- Milestone 5. Dependencies: m1-guest-toolchain, m5-security-model. Source: THE_BEGINNING.md Sections 16.5 and 22.3.

## Requirements

- Implement first-use metadata, consent, download, digest/signature verification, atomic installation, and version recording for Lima and guest artifacts.
- Use Agent Box-owned tool directories and never execute unverified updates.
- Support resumable downloads and deterministic cleanup of corrupt partials.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Fresh bootstrap reports exact components and sizes before download in interactive mode.
- [ ] Digest or signature failure prevents execution and removes/quarantines the artifact.
- [ ] Offline and preinstalled paths fail or succeed with actionable diagnostics.

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
- [ ] Run bootstrap tests against local signed, corrupt, interrupted, and offline fixtures

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement download and verify pinned runtime components for Agent Box according to the cited specification and dependencies.
