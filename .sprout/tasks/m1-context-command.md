<!-- sprout-task
{"schema_version":1,"id":"m1-context-command","status":"open","created_at":"2026-08-29T06:55:50.527137323Z"}
-->

# Expose the live generation context through API and CLI

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Expose the live generation context through API and CLI

## Context

- Milestone 1 command. Dependencies: m1-context-document, m1-single-repo-up, m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Section 14.4.

## Requirements

- Implement the context daemon endpoint and abox context command without redefining the core context model.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Returned context matches the current live generation.
- [ ] Missing, stopped, and stale-context cases use stable errors.
- [ ] Human and JSON output contain no secrets or unrelated Machine paths.

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

Implement expose the live generation context through api and cli for Agent Box.
