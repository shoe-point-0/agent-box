<!-- sprout-task
{"schema_version":1,"id":"m1-safe-remove","status":"open","created_at":"2026-08-29T06:48:36.249156679Z"}
-->

# Implement refusal-first Box removal

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement refusal-first Box removal

## Context

- Milestone 1. Dependencies: m1-git-publication-status, m1-ownership-core, m1-operation-journal, m1-reconcile-engine, m1-compose-adapter, m1-lima-adapter. Source: THE_BEGINNING.md Sections 10.8 and 14.1.

## Requirements

- Check canonical containment, Git registration, branch, dirtiness including ignored/untracked files, unpublished commits, and resource ownership.
- Require separate explicit discard flags plus --yes for each applicable non-interactive destructive condition.
- Remove in dependency order while retaining enough state to diagnose partial cleanup.
- Complete a read-only preflight before mutation, hold the exclusive Box lease, quiesce Box writers, re-observe every safety fact immediately before each destructive step, and bind confirmation to a digest of the exact loss inventory.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Default removal refuses dirty or unpublished work and leaves every file and resource intact.
- [ ] Explicit removal lists exact losses before confirmation and touches only the selected Box.
- [ ] Rerunning after interruption safely converges.
- [ ] Any drift in dirtiness, unpublished commits, inode identity, labels, or volume policy after preview forces a fresh refusal or confirmation before mutation.

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
- [ ] Run destructive safety tests only inside isolated temporary roots

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement refusal-first box removal for Agent Box according to the cited specification and dependencies.
