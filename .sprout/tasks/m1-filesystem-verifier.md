<!-- sprout-task
{"schema_version":1,"id":"m1-filesystem-verifier","status":"open","created_at":"2026-08-29T06:48:36.123097622Z"}
-->

# Verify mounted filesystem contracts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Verify mounted filesystem contracts

## Context

- Milestone 1. Dependencies: m1-lima-adapter, m1-guest-toolchain. Source: THE_BEGINNING.md Sections 11.4, 11.5, and Issue 7.

## Requirements

- Probe Machine-to-workspace and workspace-to-Machine bytes, modes, atomic rename, deletion, and watcher delivery under .abox-probe.
- Verify every declared canonical path and prove undeclared canaries are absent.
- Record stable allowed-root identity, revalidate every ancestor immediately before Lima and Compose apply, reject symlink or inode drift, and compare the Plan with live Lima inspection, guest mountinfo, Docker inspect, and container mountinfo as an exact set.
- Clean probe artifacts and configure an Agent Box-managed global Git exclude.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A Box cannot become ready when required visibility or ownership checks fail.
- [ ] Probe failures retain bounded, non-secret diagnostics.
- [ ] Cleanup never changes repository .gitignore or unrelated global Git configuration.
- [ ] Concurrent symlink and rename substitution of worktrees, Git stores, generated files, and bind sources is refused before READY.

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
- [ ] Run filesystem verifier integration tests through a real VM and container

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement verify mounted filesystem contracts for Agent Box according to the cited specification and dependencies.
