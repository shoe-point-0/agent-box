<!-- sprout-task
{"schema_version":1,"id":"m5-installation-docs","status":"open","created_at":"2026-08-29T06:48:36.671176622Z"}
-->

# Ship installation, quickstart, operations, and examples

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Ship installation, quickstart, operations, and examples

## Context

- Milestone 5. Dependencies: m5-release-supply-chain, m3-route-commands. Source: THE_BEGINNING.md Sections 14, 17, 22.3, and 24.

## Requirements

- Provide a signed package or Homebrew formula plus uninstall behavior.
- Document strict manifest authoring, one-command workflow, durability, security, recovery, diagnostics, and compatibility limits.
- Include realistic single- and two-repository examples validated in CI.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A new user can install and reach a READY example without Docker Desktop or manual VM/network setup.
- [ ] Uninstall does not silently remove Boxes or worktrees.
- [ ] Every documented command and manifest is exercised automatically.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] Run clean-Machine install, quickstart, upgrade, and uninstall tests
- [ ] Validate documentation examples in CI

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement ship installation, quickstart, operations, and examples for Agent Box according to the cited specification and dependencies.
