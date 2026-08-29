<!-- sprout-task
{"schema_version":1,"id":"m1-daemon-api","status":"implemented","created_at":"2026-08-29T06:48:36.172957524Z","implemented_at":"2026-08-29T23:08:25.272276755Z"}
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

- [x] A compatible client can start and reattach to one long operation.
- [x] Incompatible API ranges produce stable version errors.
- [x] Other local users cannot use the control socket.

## Technical approach

- Add an `internal/daemon` transport boundary that owns one HTTP server, its
  Unix listener, active requests, shutdown deadline, socket cleanup, and an
  exclusive advisory lock for the daemon lifetime. Startup accepts explicit
  runtime paths so platform path selection remains outside the transport.
- Prepare and verify the runtime directory and lock/socket entries with
  `lstat`/no-follow opens, owner-only modes, a restrictive creation umask, and
  post-bind UID/type/mode checks. Refuse every pre-existing socket rather than
  unlinking an endpoint whose stale ownership cannot be proven.
- Authenticate every accepted connection from kernel peer credentials before
  HTTP parsing. Use `SO_PEERCRED` on Linux and `LOCAL_PEERCRED`/`Xucred` on
  Darwin; failure to obtain credentials or a UID mismatch closes the
  connection. Keep OS-specific code in build-tagged files and cross-compile the
  Darwin path without claiming Darwin runtime evidence.
- Negotiate an integer API range before dispatch and return the shared stable
  `ABOX_API_INCOMPATIBLE` failure on disjoint ranges. Require bounded
  idempotency keys on mutation handlers, use the operation journal as the
  durable identity/reattachment boundary, and stream persisted monotonic events
  as NDJSON from a caller-supplied sequence. Client disconnect only detaches the
  stream; it does not cancel daemon-owned work.
- Exercise the real Linux Unix socket with integration tests for permissions,
  same-UID access, injected foreign-UID refusal, duplicate-key reattachment,
  digest conflict, resumable streaming, and bounded graceful shutdown.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run Unix-socket permission, handshake, streaming, and shutdown integration tests

## Validation evidence

- `go test -race -count=20 ./cmd/aboxd ./internal/daemon` passed. The
  integration suite started the real `aboxd` command and connected over its
  Unix socket; verified owner UID plus `0700` runtime and `0600` socket modes;
  rejected symlink and stale endpoints, a second flock owner, and a
  kernel-credential connection against a mismatched UID; negotiated compatible
  and incompatible API ranges; started one daemon-owned operation, attached a
  second caller without duplicate execution, resumed NDJSON from a sequence,
  preserved distinct large JSON integers in request digests, and exercised
  detach, cancellation, shutdown timeout, retry, and exact socket cleanup.
- `go test -race -count=10 ./internal/journal` passed, including persisted
  operation/event restart behavior and the transport's new operation lookup.
- `make check` passed: formatting, pinned lint (`0 issues`), module integrity,
  unit tests, race tests, vet, builds, smoke, and automation checks.
- `go test -race -count=10 ./...` passed across every package. The longest
  package was `internal/compose` at `132.384s`; daemon and journal completed at
  `11.800s` and `54.578s` respectively.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed. This proves
  only that the `LOCAL_PEERCRED`/`Xucred` path compiles; it is not macOS runtime
  evidence.

## Outcome and follow-ups

- Added a supervised `aboxd` serve path with explicit Linux/Darwin state and
  runtime locations, signal-bounded shutdown, and no TCP listener. Unimplemented
  Box mutation logic returns a typed feature-not-supported result rather than
  pretending the later single-repository slice exists.
- Added a secure Unix transport with a restrictive umask, no-follow lock open,
  kernel flock, stale-path refusal, post-bind ownership/mode checks, Linux
  `SO_PEERCRED`, Darwin `LOCAL_PEERCRED`, exact inode cleanup, and retained
  exclusion when a shutdown deadline expires.
- Added API-range negotiation, strict bounded request decoding, lossless
  canonical request digests, durable idempotency through the operation journal,
  same-daemon attachment, sequenced API-versioned NDJSON events, typed terminal
  results, and resume from a caller sequence. Client disconnect only detaches.
- A running operation recovered from a dead daemon is now refused with a
  retryable operation conflict instead of hanging or executing twice. Adoption
  and reconstruction remain owned by `m4-recovery-reconstruction`; this task
  does not claim restart recovery before that dependency exists.
- Real Darwin peer-credential and socket lifecycle execution remains part of
  `m5-darwin-arm64-hardware-gate`; cross-compilation is not support evidence.

## Original request

Implement serve the versioned daemon api over a unix socket for Agent Box according to the cited specification and dependencies.
