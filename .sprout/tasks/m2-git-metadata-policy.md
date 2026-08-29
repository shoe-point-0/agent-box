<!-- sprout-task
{"schema_version":1,"id":"m2-git-metadata-policy","status":"open","created_at":"2026-08-29T06:48:36.333605037Z"}
-->

# Enforce read-write and read-only Git metadata policies

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Enforce read-write and read-only Git metadata policies

## Context

- Milestone 2. Dependencies: m2-multi-repo-compose, m1-git-worktrees. Source: THE_BEGINNING.md Sections 10.6 and Milestone 2.

## Requirements

- Mount private Git metadata read-write by default only into authorized services.
- Provide inspect-only mode where status and diff work but mutations fail without corrupting metadata.
- Report the durability consequences of read-only metadata in context and CLI output.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Workspace-service add/commit succeeds in default mode.
- [ ] The hardened fixture can inspect but cannot mutate refs, index, objects, config, or hooks.
- [ ] Support services receive no Git metadata unless declared.

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
- [ ] Run default and hardened Git-policy E2E scenarios

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement enforce read-write and read-only git metadata policies for Agent Box according to the cited specification and dependencies.
