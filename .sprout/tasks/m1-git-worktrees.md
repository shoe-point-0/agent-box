<!-- sprout-task
{"schema_version":1,"id":"m1-git-worktrees","status":"open","created_at":"2026-08-29T06:48:36.047741734Z"}
-->

# Manage linked worktree lifecycle and observation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Manage linked worktree lifecycle and observation

## Context

- Milestone 1. Dependencies: m1-git-private-store. Source: THE_BEGINNING.md Sections 10.1, 10.4, 10.6, and Issue 5.

## Requirements

- Create the generated branch and worktree, lock it with the Box identity, and parse NUL-delimited Git porcelain.
- Observe registration, branch, commit, dirty state, common directory, lock reason, and device/inode identity.
- Reject active submodules and unsupported relative-worktree clients.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A second ensure call is a no-op for a matching worktree.
- [ ] Mismatched registration, branch, or ownership produces a conflict without reset.
- [ ] Git operates from Machine and canonical mounted paths in supported fixtures.

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
- [ ] Run real-Git worktree lifecycle integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement manage linked worktree lifecycle and observation for Agent Box according to the cited specification and dependencies.
