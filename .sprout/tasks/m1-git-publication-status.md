<!-- sprout-task
{"schema_version":1,"id":"m1-git-publication-status","status":"open","created_at":"2026-08-29T06:55:50.449897416Z"}
-->

# Detect unpublished Box commits from trusted destinations

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Detect unpublished Box commits from trusted destinations

## Context

- Milestone 1. Dependencies: m1-machine-git-profile, m1-git-worktrees. Source: THE_BEGINNING.md Sections 10.3 and 10.8.

## Requirements

- Compute per-ref reachability against trusted destinations recorded outside Box-writable Git config.
- Treat missing/unavailable destinations conservatively and return exact commit evidence.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Clean worktrees with Box-only commits are classified unpublished.
- [ ] Malicious Box config cannot redirect reachability checks.
- [ ] Local, missing, rewritten, detached, and multiple-destination cases pass real-Git tests.

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

Implement detect unpublished box commits from trusted destinations for Agent Box.
