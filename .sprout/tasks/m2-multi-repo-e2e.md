<!-- sprout-task
{"schema_version":1,"id":"m2-multi-repo-e2e","status":"open","created_at":"2026-08-29T06:48:36.364848975Z"}
-->

# Gate multi-repository edit, commit, and publish flows

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Gate multi-repository edit, commit, and publish flows

## Context

- Milestone 2 gate. Dependencies: m2-remote-repositories, m2-multi-repo-compose, m2-cache-volumes, m2-setup-jobs, m2-git-metadata-policy, m2-publish, m2-multi-repo-up. Source: THE_BEGINNING.md Sections 19.2 scenarios 1, 5, 6, 15, and 17.

## Requirements

- Exercise local and remote sources, two worktrees, caches, setup jobs, Git policies, and explicit publication.
- Prove source repositories remain unchanged before publication.
- Run each repository's declared test command from the workspace service, create distinct commits in both repositories, and publish each exact selected ref.
- Assert post-test cleanup and forbidden mount/credential absence.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] The Milestone 2 exit gate passes on Linux CI and emits fixtures consumable unchanged by the final Darwin/arm64 gate.
- [ ] Published refs match selected Box commits exactly.
- [ ] Both repository-specific test commands pass and both exact destination refs receive only their selected commits.
- [ ] No source metadata, credential location, sibling Box, or undeclared path is mutated.

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
- [ ] Run the dedicated multi-repository E2E suite

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement gate multi-repository edit, commit, and publish flows for Agent Box according to the cited specification and dependencies.
