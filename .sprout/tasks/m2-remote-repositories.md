<!-- sprout-task
{"schema_version":1,"id":"m2-remote-repositories","status":"open","created_at":"2026-08-29T06:48:36.279230418Z"}
-->

# Support remote repositories and safe caches

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Support remote repositories and safe caches

## Context

- Milestone 2. Dependencies: m1-single-repo-e2e. Source: THE_BEGINNING.md Sections 10.2 and Milestone 2.

## Requirements

- Fetch remote sources on the Machine with existing user credential mechanisms.
- Maintain optional read-only caches keyed by canonical remote identity and seed Box stores without persistent alternates.
- Keep credentials and Machine Git configuration outside the VM and Box-writable metadata.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Remote and cached cold/warm paths resolve the same commit.
- [ ] Box stores remain usable after cache removal.
- [ ] Credential canaries never appear in guest state, Plan, events, or diagnostics.

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
- [ ] Run remote/cache integration tests against local credential-safe fixtures

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement support remote repositories and safe caches for Agent Box according to the cited specification and dependencies.
