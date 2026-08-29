<!-- sprout-task
{"schema_version":1,"id":"m1-single-repo-up","status":"open","created_at":"2026-08-29T06:48:36.203491553Z"}
-->

# Deliver the single-repository up vertical slice

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Deliver the single-repository up vertical slice

## Context

- Milestone 1. Dependencies: m1-machine-preflight, m1-git-worktrees, m1-lima-adapter, m1-filesystem-verifier, m1-compose-policy-core, m1-compose-adapter, m1-context-document, m1-reconcile-engine, m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Sections 9.3, 14.1, and Milestone 1.

## Requirements

- Wire lease, validation, Git, Plan, runtime host, mounts, Compose, basic readiness, context, and READY transition for one local repository.
- Re-observe each boundary and resume matching partial resources.
- Return success only after the declared workspace service is usable.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A clean fixture reaches READY from zero with abox up demo.
- [ ] A second up is a measured no-op with no duplicate worktree, VM, network, volume, container, or setup effect.
- [ ] Killing the CLI does not kill or duplicate daemon-owned reconciliation.

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
- [ ] Run the real single-repository cold and warm up smoke test

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement deliver the single-repository up vertical slice for Agent Box according to the cited specification and dependencies.
