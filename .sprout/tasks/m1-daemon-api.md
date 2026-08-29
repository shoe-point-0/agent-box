<!-- sprout-task
{"schema_version":1,"id":"m1-daemon-api","status":"open","created_at":"2026-08-29T06:48:36.172957524Z"}
-->

# Serve the versioned daemon API over a Unix socket

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Serve the versioned daemon API over a Unix socket

## Context

- Milestone 1. Dependencies: m1-operation-journal, m1-reconcile-engine. Source: THE_BEGINNING.md Sections 6.1 and 14.5.

## Requirements

- Serve HTTP+JSON and NDJSON operations from a user-owned mode-0700 runtime directory over a mode-0600 Unix socket, with no-follow path validation, restrictive umask, an exclusive kernel-backed daemon lock, safe stale-socket refusal, and post-bind UID/mode verification.
- Implement API-range handshake, idempotency keys, operation attachment, and peer-identity checks where available.
- Enforce getpeereid or equivalent peer credentials on every supported macOS connection and own startup, shutdown, router lifetime, active requests, and socket cleanup explicitly.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A compatible client can start and reattach to one long operation.
- [ ] Incompatible API ranges produce stable version errors.
- [ ] Other local users cannot use the control socket.

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
- [ ] Run Unix-socket permission, handshake, streaming, and shutdown integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement serve the versioned daemon api over a unix socket for Agent Box according to the cited specification and dependencies.
