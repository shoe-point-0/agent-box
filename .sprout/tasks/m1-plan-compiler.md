<!-- sprout-task
{"schema_version":1,"id":"m1-plan-compiler","status":"open","created_at":"2026-08-29T06:48:36.064104371Z"}
-->

# Compile canonical immutable Plans

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Compile canonical immutable Plans

## Context

- Milestone 1. Dependencies: m1-schema-contract, m1-manifest-loader, m1-identity-paths, m1-repository-resolver. Source: THE_BEGINNING.md Section 8.2 and Issue 6.

## Requirements

- Compile the Milestone 1 executable subset—one local repository, host resources, one Compose include, workspace service, and required mounts—plus platform and repository facts into the versioned Plan model.
- Add routes, ports, caches, setup keys, remote sources, and opaque secret references only in the milestone that first consumes each field; reject recognized unsupported intent before side effects.
- Serialize canonical JSON and compute a stable digest excluding secret values.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Random map order and repeated runs yield byte-identical Plans and digests.
- [ ] The Plan contains every ownership and observation key required by adapters.
- [ ] No secret value can enter the Plan or its hash input.

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
- [ ] Run canonical Plan golden tests repeatedly with randomized input order

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement compile canonical immutable plans for Agent Box according to the cited specification and dependencies.
