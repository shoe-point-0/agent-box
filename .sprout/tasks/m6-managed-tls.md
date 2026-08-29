<!-- sprout-task
{"schema_version":1,"id":"m6-managed-tls","status":"open","created_at":"2026-08-29T06:48:36.777055399Z"}
-->

# Add opt-in managed local TLS

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Add opt-in managed local TLS

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Sections 13.4 and 23.

## Requirements

- Define explicit CA installation, trust, rotation, removal, and consent behavior before enabling TLS.
- Terminate only declared routes and preserve loopback/network policy.
- Support users who decline Machine trust changes.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Installation and removal leave no unowned trust material.
- [ ] Certificates rotate and validate for exact declared names.
- [ ] Plain HTTP and project-owned TCP TLS remain supported alternatives.

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
- [ ] Run certificate lifecycle and clean-uninstall tests on supported macOS versions

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement add opt-in managed local tls for Agent Box according to the cited specification and dependencies.
