<!-- sprout-task
{"schema_version":1,"id":"m1-project-foundation","status":"open","created_at":"2026-08-29T06:48:35.92485512Z"}
-->

# Establish the Go project and contributor foundation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Establish the Go project and contributor foundation

## Context

- Milestone 1. Dependencies: m0-substrate-report. Source: THE_BEGINNING.md Sections 18, 22, and 25.

## Requirements

- Evolve the Milestone 0 module into cmd/abox and cmd/aboxd entrypoints, production Makefile targets, and only the package layout earned by the first slice.
- Configure formatting, vet, unit tests, race tests, linting, dependency review, and supported macOS CI.
- Add README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, and ADR conventions without claiming unfinished capabilities.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Both binaries build and report version information.
- [ ] CI runs the documented local quality gates.
- [ ] A new contributor can run the checks from a clean clone.

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
- [ ] go build ./cmd/abox ./cmd/aboxd

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement establish the go project and contributor foundation for Agent Box according to the cited specification and dependencies.
