<!-- sprout-task
{"schema_version":1,"id":"m1-compose-adapter","status":"open","created_at":"2026-08-29T06:48:36.151865633Z"}
-->

# Implement Compose lifecycle, exec, logs, and observation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement Compose lifecycle, exec, logs, and observation

## Context

- Milestone 1. Dependencies: m1-process-runner, m1-compose-compiler, m1-compose-policy-core, m1-guest-toolchain, m1-ownership-core. Source: THE_BEGINNING.md Sections 9.3, 12.2, and Issue 8.

## Requirements

- Observe, pull/build, apply, inspect, stop, and remove using an explicit project name and generated files only after normalized policy validation succeeds.
- Observe containers, health, networks, volumes, images, labels, and actual port bindings.
- Propagate cancellation and preserve bounded diagnostics while supporting streaming commands.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A second apply does not duplicate Compose resources.
- [ ] Every observed resource is attributable by recorded ID and labels.
- [ ] Exec returns exact exit status and logs stop cleanly on cancellation.

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
- [ ] Run Compose adapter contract and real-runtime integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement compose lifecycle, exec, logs, and observation for Agent Box according to the cited specification and dependencies.
