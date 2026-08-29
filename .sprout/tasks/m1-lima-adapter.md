<!-- sprout-task
{"schema_version":1,"id":"m1-lima-adapter","status":"open","created_at":"2026-08-29T06:48:36.094129071Z"}
-->

# Implement the Lima runtime-host adapter

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the Lima runtime-host adapter

## Context

- Milestone 1 Linux implementation. Dependencies: m1-process-runner, m1-plan-compiler, m1-ownership-core, m1-toolchain-contract. Exercise machine-readable Lima contracts and Linux-supported runtime paths now; real VZ behavior is a final Darwin hardware gate. Source: THE_BEGINNING.md Sections 5.1, 6.2, 9, and Issue 7.

## Requirements

- Generate minimal pinned VZ Lima configuration with explicit resources, metadata, and exact mount allowlist.
- Create, start, stop, inspect, invoke, and delete through version-checked machine-readable limactl contracts.
- Classify unsupported versions, partial creation, mismatched immutable config, and cancellation.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Lifecycle methods are idempotent against observed Lima state.
- [ ] Deletion requires recorded instance identity plus embedded Box metadata.
- [ ] Adapter contract tests cover expected supported Lima outputs and errors.

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
- [ ] Run Lima adapter contract tests
- [ ] Run the adapter smoke path on Linux with a real supported Lima configuration where available; retain Darwin/arm64 execution for m5-darwin-arm64-hardware-gate

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement the lima runtime-host adapter for Agent Box according to the cited specification and dependencies.
