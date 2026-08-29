<!-- sprout-task
{"schema_version":1,"id":"m6-ide-integrations","status":"open","created_at":"2026-08-29T06:48:36.793457735Z"}
-->

# Build IDE integrations over stable context and daemon APIs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Build IDE integrations over stable context and daemon APIs

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Section 20 Milestone 6.

## Requirements

- Use context and daemon APIs rather than direct state-database or Lima access.
- Support attach, status, routes, commands, and logs without inventing a second lifecycle.
- Keep editor-specific code outside the Go control-plane core.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] IDE state matches CLI state after restart and reconciliation.
- [ ] No integration receives a privileged control path or unrelated Machine filesystem access.
- [ ] Compatibility is tested for every advertised editor version.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] Run integration contract tests against the versioned daemon API
- [ ] Run smoke tests in every advertised editor environment

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement build ide integrations over stable context and daemon apis for Agent Box according to the cited specification and dependencies.
