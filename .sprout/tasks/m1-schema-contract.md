<!-- sprout-task
{"schema_version":1,"id":"m1-schema-contract","status":"open","created_at":"2026-08-29T06:55:50.368726934Z"}
-->

# Define versioned Plan, context, output, and error contracts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Define versioned Plan, context, output, and error contracts

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 8, 14, and 22.

## Requirements

- Define minimal Milestone 1 schemas and stable symbolic error/exit-code mapping before implementations consume them.
- Keep later milestone fields additive and reject unsupported executable features explicitly.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Schemas validate checked-in golden examples.
- [ ] Plan, daemon, CLI, and context packages consume one contract rather than redefining fields.
- [ ] No schema permits secret values.

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

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement define versioned plan, context, output, and error contracts for Agent Box.
