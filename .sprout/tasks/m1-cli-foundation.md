<!-- sprout-task
{"schema_version":1,"id":"m1-cli-foundation","status":"open","created_at":"2026-08-29T06:48:36.18714109Z"}
-->

# Implement thin CLI transport and output contracts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement thin CLI transport and output contracts

## Context

- Milestone 1. Dependencies: m1-daemon-api. Source: THE_BEGINNING.md Sections 14.1 through 14.3.

## Requirements

- Discover the manifest, validate local arguments, start or connect to aboxd, send versioned requests, and render events.
- Keep stdout parseable in --json/NDJSON mode while human progress uses stderr.
- Map stable symbolic errors to the documented exit codes.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] CLI help exposes only implemented commands and never claims unavailable behavior.
- [ ] JSON output validates against its versioned schema.
- [ ] Transport, usage, not-found, safety, readiness, and compatibility errors map consistently.

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
- [ ] Run CLI golden tests for human, JSON, NDJSON, and exit-code behavior

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement thin cli transport and output contracts for Agent Box according to the cited specification and dependencies.
