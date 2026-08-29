<!-- sprout-task
{"schema_version":1,"id":"m1-atomic-writer","status":"open","created_at":"2026-08-29T06:55:50.358054833Z"}
-->

# Implement crash-consistent atomic file writes

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement crash-consistent atomic file writes

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 15.2.

## Requirements

- Write temporary files in the destination directory with restrictive modes, complete writes, file fsync, atomic rename, and parent-directory fsync where supported.
- Expose injectable write, fsync, rename, and directory-fsync failures without hiding unsupported durability behavior.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Injected failure yields either the complete old file or complete new file, never torn content.
- [ ] Temporary files are reclaimed or safely identifiable after interruption.
- [ ] Symlink destinations and unsafe permissions are refused.

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

Implement implement crash-consistent atomic file writes for Agent Box.
