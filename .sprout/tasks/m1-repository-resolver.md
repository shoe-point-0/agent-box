<!-- sprout-task
{"schema_version":1,"id":"m1-repository-resolver","status":"open","created_at":"2026-08-29T06:55:50.389921423Z"}
-->

# Resolve repository intent without materialization

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Resolve repository intent without materialization

## Context

- Milestone 1. Dependencies: m1-manifest-loader, m1-identity-paths, m1-machine-git-profile. Source: THE_BEGINNING.md Sections 7.2, 8.1, and 9.3.

## Requirements

- Read-only resolve local roots, cleanliness, full commit IDs, trusted destinations, intended branches, worktree paths, and Git-store paths.
- Return typed facts suitable for Plan compilation without creating state rows, files, stores, or worktrees.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Resolution leaves repository, filesystem, state, Lima, Compose, and ports unchanged.
- [ ] Dirty sources fail before any materialization.
- [ ] Malicious Git configuration cannot affect trusted resolved destinations.

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

Implement resolve repository intent without materialization for Agent Box.
