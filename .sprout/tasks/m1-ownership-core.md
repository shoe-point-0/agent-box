<!-- sprout-task
{"schema_version":1,"id":"m1-ownership-core","status":"open","created_at":"2026-08-29T06:55:50.42129706Z"}
-->

# Authorize destructive operations with typed ownership evidence

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Authorize destructive operations with typed ownership evidence

## Context

- Milestone 1 safety prerequisite. Dependencies: m1-identity-paths. Source: THE_BEGINNING.md Sections 9.2 and 15.3.

## Requirements

- Define consuming-package proof types for paths, Git worktrees, Lima instances, Compose resources, volumes, sockets, and listeners.
- Require canonical containment and independent recorded/live identity facts before mutation or deletion.
- Keep policy decisions outside leaf adapters.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Forged names, labels, paths, symlinks, and stale records cannot authorize deletion.
- [ ] Every destructive adapter API requires proof rather than raw identifiers.
- [ ] Temporary-root adversarial tests show unrelated resources survive.

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

Implement authorize destructive operations with typed ownership evidence for Agent Box.
