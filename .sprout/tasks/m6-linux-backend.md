<!-- sprout-task
{"schema_version":1,"id":"m6-linux-backend","status":"open","created_at":"2026-08-29T06:48:36.730713098Z"}
-->

# Add a native Linux Machine backend

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Add a native Linux Machine backend

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Sections 4.3, 20, and 23.

## Requirements

- Define Linux-specific isolation, filesystem, networking, cgroup, and runtime-host contracts rather than copying macOS assumptions.
- Preserve the Box lifecycle and agent API while making platform behavior explicit.
- Add real CI for every claimed distribution, architecture, kernel, and runtime combination.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Linux support claims match exercised CI configurations.
- [ ] Rootless/privileged boundaries and teardown behavior are documented and tested.
- [ ] macOS behavior remains unchanged behind consuming-package interfaces.

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
- [ ] Run Linux backend integration and E2E suites on every claimed target

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement add a native linux machine backend for Agent Box according to the cited specification and dependencies.
