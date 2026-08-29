<!-- sprout-task
{"schema_version":1,"id":"m1-toolchain-contract","status":"open","created_at":"2026-08-29T06:55:50.411239326Z"}
-->

# Define verified runtime artifact inputs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Define verified runtime artifact inputs

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 6, 16.5, and 22.3.

## Requirements

- Define the immutable verified Lima, guest image, Docker, Compose, and utility identities consumed by preflight and runtime provisioning.
- Support preinstalled/test-fixture providers now and a production download provider later without implicit network access.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Missing or mismatched artifacts fail with typed prerequisite errors.
- [ ] No consumer can execute an artifact lacking verified identity.
- [ ] Fixture and future production providers share contract tests.

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

Implement define verified runtime artifact inputs for Agent Box.
