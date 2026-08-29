<!-- sprout-task
{"schema_version":1,"id":"m3-route-commands","status":"open","created_at":"2026-08-29T06:48:36.441803232Z"}
-->

# Expose routes, ports, and open through CLI and context

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Expose routes, ports, and open through CLI and context

## Context

- Milestone 3. Dependencies: m3-machine-router, m3-tcp-allocator, m1-context-document. Source: THE_BEGINNING.md Sections 14.1 and 14.4.

## Requirements

- Implement routes, port, and open commands over the daemon API.
- Report stable URLs and endpoints with live readiness in human and JSON forms.
- Use the platform opener only for validated declared routes.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Commands distinguish not-found, allocated-but-unready, and ready endpoints.
- [ ] Context matches the router and allocator generation.
- [ ] No arbitrary URL or shell interpolation reaches the Machine opener.

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
- [ ] Run CLI/schema tests and opener argument-injection tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement expose routes, ports, and open through cli and context for Agent Box according to the cited specification and dependencies.
