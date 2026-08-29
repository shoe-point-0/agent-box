<!-- sprout-task
{"schema_version":1,"id":"m1-git-private-store","status":"open","created_at":"2026-08-29T06:48:36.031491662Z"}
-->

# Seed self-contained Box-private Git stores

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Seed self-contained Box-private Git stores

## Context

- Milestone 1. Dependencies: m1-process-runner, m1-machine-git-profile, m1-repository-resolver, m1-plan-compiler, m1-ownership-core. Source: THE_BEGINNING.md Sections 7.2, 10.1, and Issue 5.

## Requirements

- Consume resolved repository facts and the immutable Plan; re-observe cleanliness and full commit IDs immediately before materialization.
- Create a self-contained bare store without writable hardlinks or alternates and enable reflogs.
- Record a trusted publish destination outside Box-writable Git configuration.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Dirty local sources fail with exit semantics equivalent to ABOX_GIT_DIRTY_SOURCE before partial worktree creation.
- [ ] Object writes in the Box store cannot mutate source repository inodes or refs.
- [ ] Detached refs, spaces, Unicode, and branch collisions are covered by real-Git tests.

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
- [ ] Run real-Git private-store integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement seed self-contained box-private git stores for Agent Box according to the cited specification and dependencies.
