<!-- sprout-task
{"schema_version":1,"id":"m1-compose-compiler","status":"open","created_at":"2026-08-29T06:48:36.138968013Z"}
-->

# Compile generated Compose inputs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Compile generated Compose inputs

## Context

- Milestone 1. Dependencies: m1-plan-compiler, m1-schema-contract, m1-atomic-writer, m0-compose-probe. Source: THE_BEGINNING.md Sections 8.4 and 12.1.

## Requirements

- Generate compose.root.yaml and compose.abox.yaml atomically from the Plan.
- Set explicit project identity, Box labels, Milestone 1 declared mounts, network policy, and Machine-port suppression; cache-volume projection lands with m2-cache-volumes.
- Normalize through the pinned Compose CLI and retain a redacted model digest.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Generated files are deterministic for one Plan.
- [ ] Relative repository paths preserve owning project directories.
- [ ] Forbidden publications and secret values are absent from the retained model.

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
- [ ] Run Compose compiler golden tests
- [ ] Normalize generated fixtures with the pinned Compose CLI

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement compile generated compose inputs for Agent Box according to the cited specification and dependencies.
