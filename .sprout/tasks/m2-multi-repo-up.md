<!-- sprout-task
{"schema_version":1,"id":"m2-multi-repo-up","status":"open","created_at":"2026-08-29T06:48:36.353428213Z"}
-->

# Reconcile complete multi-repository workspaces

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Reconcile complete multi-repository workspaces

## Context

- Milestone 2. Dependencies: m2-multi-repo-compose, m2-cache-volumes, m2-setup-jobs, m2-git-metadata-policy. Source: THE_BEGINNING.md Milestone 2 exit gate.

## Requirements

- Extend up, status, context, exec, and removal to one worktree and Git store per declared repository.
- Observe and classify repository addition/removal while refusing dirty removals.
- Keep one runtime host, one Compose project, and one lifecycle identity for the worktree set.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A two-repository Workspace reaches READY and remains idempotent.
- [ ] Edits and commits in either repository remain isolated from source repositories and sibling Boxes.
- [ ] Removing a declared dirty repository is refused without partial mutation.

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
- [ ] Run cold, warm, add-repository, and remove-repository integration scenarios

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement reconcile complete multi-repository workspaces for Agent Box according to the cited specification and dependencies.
