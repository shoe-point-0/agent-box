<!-- sprout-task
{"schema_version":1,"id":"m1-list-command","status":"implemented","created_at":"2026-08-29T06:55:50.494935974Z","implemented_at":"2026-08-29T23:55:30.836118413Z"}
-->

# List Boxes from observed and recorded state

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

List Boxes from observed and recorded state

## Context

- Milestone 1 command. Dependencies: m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement abox list in human and JSON forms with identity, state, task, workspace, and observed drift summary.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Empty, active, stopped, degraded, and stale-record cases render deterministically.
- [x] Listing is read-only and never reconciles or deletes resources.
- [x] Output validates against the versioned schema.

## Technical approach

- Add a dedicated `abox.list/v1alpha1` contract and strict JSON Schema rather
  than overloading the single-Box operation output.
- Read Box, Workspace, cached runtime-host, and cached Compose observations in
  one read transaction ordered by Box ID. Derive `active`, `stopped`,
  `degraded`, or `stale` from those facts and emit stable drift reasons when
  recorded intent and observed components disagree.
- Expose the result through the daemon's version-negotiated `GET /v1/boxes`
  endpoint and bounded Unix-socket client. This path has no reconciler,
  mutation, or cleanup capability.
- Render either schema-valid JSON or a deterministic tabular human view in the
  thin CLI. Cover empty output and every required state with persisted fixtures,
  plus a read-only regression check.

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

## Validation evidence

- `make check`: passed, including formatting, `0 issues` from the pinned
  linter, dependency verification, unit tests, race tests, vet, builds, smoke,
  and automation checks.
- `go test -race -count=10 ./...`: passed across every package.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: passed as
  compilation evidence only; no Darwin runtime support is inferred.
- Unit and transport tests cover empty, active, stopped, degraded, and stale
  inventories; stable Box-ID ordering; strict bounded client decoding; schema
  validation; human rendering; and unchanged persisted records before/after a
  list call.
- Live Linux daemon/CLI smoke used an isolated state database and Unix socket.
  Empty JSON was `{"schemaVersion":"abox.list/v1alpha1","boxes":[]}`. After
  seeding all four states, both human and JSON output were deterministic and a
  before/after SQL snapshot was byte-identical. The owned daemon process and
  temporary root were removed afterward.
- The first smoke hypothesis used a `0775` runtime parent and was correctly
  rejected by the daemon's secure-directory preflight. The rerun used `0700`;
  do not weaken the preflight or repeat the permissive fixture.

## Outcome and follow-ups

Implemented `abox list` through the version-negotiated daemon API. The daemon
reads Box/Workspace identity and the latest persisted runtime-host and Compose
observations in one read transaction, derives a stable state and drift summary,
and exposes no reconciliation or cleanup capability to the listing service.
Human output is tabular; `--json` conforms to `abox.list/v1alpha1`.

Live external observation remains owned by the future Lima and Compose adapters;
this command intentionally reports their persisted observation cache rather than
shelling out or inventing a second observer before those dependency tasks land.

## Original request

Implement list boxes from observed and recorded state for Agent Box.
