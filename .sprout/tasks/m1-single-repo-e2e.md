<!-- sprout-task
{"schema_version":1,"id":"m1-single-repo-e2e","status":"open","created_at":"2026-08-29T06:48:36.264812951Z"}
-->

# Gate the single-repository milestone end to end

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Gate the single-repository milestone end to end

## Context

- Milestone 1 gate. Dependencies: m1-project-foundation, m1-architecture-decisions, m1-atomic-writer, m1-schema-contract, m1-process-runner, m1-identity-paths, m1-state-store, m1-operation-journal, m1-toolchain-contract, m1-machine-git-profile, m1-manifest-loader, m1-repository-resolver, m1-plan-compiler, m1-plan-diff, m1-ownership-core, m1-git-private-store, m1-git-worktrees, m1-git-publication-status, m1-lima-adapter, m1-guest-toolchain, m1-filesystem-verifier, m1-compose-compiler, m1-compose-policy-core, m1-compose-adapter, m1-reconcile-engine, m1-daemon-api, m1-cli-foundation, m1-machine-preflight, m1-context-document, m1-single-repo-up, m1-core-commands, m1-exec-logs-shell, m1-context-command, m1-init-command, m1-plan-command, m1-list-command, m1-doctor-command, m1-safe-remove. Source: THE_BEGINNING.md Sections 19.1, 19.2 scenarios 2 through 4, 9, 13 through 16, and Milestone 1 exit gate.

## Requirements

- Build a dedicated Linux fixture covering cold up, no-op up, Git edits, exec/logs, down/up, CLI death, and safe removal refusal; reuse it unchanged in the final Darwin/arm64 gate.
- Assert exact bytes, mode, rename, deletion, watcher result, and exact effective mount set in both directions at the final up success boundary.
- Assert desired resources and forbidden mounts, sockets, listeners, labels, and duplicates.
- Retain scoped diagnostics on failure and clean only verified fixture resources.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] The Milestone 1 exit gate passes from a clean Machine state.
- [ ] The test fails if an undeclared Machine path or port is exposed.
- [ ] A post-run leak check finds no owned VM, process, mount, socket, network, container, or temporary file.

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
- [ ] Run the dedicated single-repository E2E suite on Linux

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement gate the single-repository milestone end to end for Agent Box according to the cited specification and dependencies.
