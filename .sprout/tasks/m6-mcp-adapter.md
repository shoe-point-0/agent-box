<!-- sprout-task
{"schema_version":1,"id":"m6-mcp-adapter","status":"open","created_at":"2026-08-29T06:48:36.762554614Z"}
-->

# Expose the daemon contract through an MCP adapter

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Expose the daemon contract through an MCP adapter

## Context

- Milestone 6 deferred. Dependencies: m5-rc-acceptance. Source: THE_BEGINNING.md Sections 4.3, 14.5, and 20.

## Requirements

- Wrap the existing versioned daemon API without a privileged side channel.
- Expose bounded task-oriented tools and machine-readable errors while preserving Box authorization.
- Keep MCP optional and vendor-neutral.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] MCP and CLI observe identical Box state and operation IDs.
- [ ] Adapter failure cannot orphan or bypass daemon-owned operations.
- [ ] No secret or Machine credential path appears in tool results.

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
- [ ] Run MCP contract tests against the daemon API

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement expose the daemon contract through an mcp adapter for Agent Box according to the cited specification and dependencies.
