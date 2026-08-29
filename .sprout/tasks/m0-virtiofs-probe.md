<!-- sprout-task
{"schema_version":1,"id":"m0-virtiofs-probe","status":"open","created_at":"2026-08-29T06:48:35.832926579Z"}
-->

# Measure VirtioFS coherency and watcher behavior

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Measure VirtioFS coherency and watcher behavior

## Context

- Final Darwin hardware evidence input, implemented when a testable Darwin/arm64 artifact exists. Dependencies: m0-spike-harness. This task no longer blocks Linux implementation and feeds m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 11 and 19.2.

## Requirements

- Provision a minimal Lima VZ VM and mount only probe worktrees through VirtioFS.
- Measure Machine-to-container and container-to-Machine bytes, modes, rename, deletion, and event visibility.
- Record OS, Lima, guest, filesystem, hardware, sample count, latency distribution, and watcher caveats.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Bidirectional exact-byte probes pass or fail with reproducible evidence.
- [ ] Undeclared canary directories are absent from guest and container.
- [ ] Watcher results distinguish supported native events from polling-required behavior.

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
- [ ] Run the VirtioFS probe on a supported Apple Silicon Mac

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement measure virtiofs coherency and watcher behavior for Agent Box according to the cited specification and dependencies.
