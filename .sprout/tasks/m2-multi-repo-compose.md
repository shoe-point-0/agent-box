<!-- sprout-task
{"schema_version":1,"id":"m2-multi-repo-compose","status":"open","created_at":"2026-08-29T06:48:36.294368942Z"}
-->

# Compile and apply multi-repository Compose graphs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Compile and apply multi-repository Compose graphs

## Context

- Milestone 2. Dependencies: m1-compose-compiler, m1-compose-adapter, m2-remote-repositories. Source: THE_BEGINNING.md Sections 7, 8.4, and 12.1.

## Requirements

- Resolve every declared repository and include while preserving repository-local Compose path bases.
- Reject global service-name collisions and invalid cross-repository references.
- Mount the coordinated worktree set into the designated workspace service according to policy.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Two repository fixtures build and start as one explicit Compose project.
- [ ] Relative build contexts and env files resolve from the owning repository.
- [ ] No undeclared repository path reaches a service.

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
- [ ] Run the real two-repository Compose integration fixture

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement compile and apply multi-repository compose graphs for Agent Box according to the cited specification and dependencies.
