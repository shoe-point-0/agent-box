<!-- sprout-task
{"schema_version":1,"id":"m3-box-gateway","status":"open","created_at":"2026-08-29T06:48:36.395462334Z"}
-->

# Generate and run the in-Box HTTP gateway

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Generate and run the in-Box HTTP gateway

## Context

- Milestone 3 transport. Dependencies: m2-multi-repo-compose. Source: THE_BEGINNING.md Sections 6.2 and 13.2.

## Requirements

- Generate deterministic routing configuration from declared routes and attach one gateway to the Box network.
- Proxy HTTP/1.1, WebSocket, SSE, and streaming bodies with bounded headers and idle timeouts.
- Preserve original host/scheme metadata and strip untrusted control headers.
- Add HTTP route Plan/context projection, state migration, and diff rules with deterministic golden tests.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Each route reaches only its declared service and target port.
- [ ] Gateway configuration may exist before publication readiness but is not reported ready until the full published path passes.
- [ ] Gateway resources carry complete Box ownership labels.

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
- [ ] Run gateway protocol and adversarial-header integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement generate and run the in-box http gateway for Agent Box according to the cited specification and dependencies.
