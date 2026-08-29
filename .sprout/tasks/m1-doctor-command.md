<!-- sprout-task
{"schema_version":1,"id":"m1-doctor-command","status":"open","created_at":"2026-08-29T06:55:50.510725152Z"}
-->

# Inspect Machine and Box filesystem prerequisites

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Inspect Machine and Box filesystem prerequisites

## Context

- Milestone 1 command. Dependencies: m1-machine-preflight, m1-filesystem-verifier, m1-cli-foundation. Source: THE_BEGINNING.md Sections 10.5, 11.4, 14.1, and 17.

## Requirements

- Implement general, Box-specific, and fs doctor modes using read-only observations.
- Report relative-worktree safety, watcher behavior/polling guidance, tools, mounts, ports, disk, runtime, and ownership evidence.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Doctor never provisions or repairs resources.
- [ ] Every failed prerequisite has a stable symbolic result and remediation text.
- [ ] JSON output distinguishes tested fact, warning, unsupported case, and unknown.

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

Implement inspect machine and box filesystem prerequisites for Agent Box.
