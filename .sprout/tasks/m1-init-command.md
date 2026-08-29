<!-- sprout-task
{"schema_version":1,"id":"m1-init-command","status":"open","created_at":"2026-08-29T06:55:50.464348415Z"}
-->

# Generate a strict starter manifest without provisioning

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Generate a strict starter manifest without provisioning

## Context

- Milestone 1 command. Dependencies: m1-manifest-loader, m1-cli-foundation. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement abox init with deterministic strict starter YAML and explicit overwrite behavior.
- Do not contact the daemon or create Box state.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Generated YAML passes the strict loader.
- [ ] Existing files are preserved unless the documented explicit overwrite path is used.
- [ ] No Git store, state row, VM, port, or Box file is created.

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

Implement generate a strict starter manifest without provisioning for Agent Box.
