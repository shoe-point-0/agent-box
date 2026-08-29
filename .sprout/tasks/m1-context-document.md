<!-- sprout-task
{"schema_version":1,"id":"m1-context-document","status":"open","created_at":"2026-08-29T06:48:36.233617336Z"}
-->

# Generate versioned agent context and output schemas

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Generate versioned agent context and output schemas

## Context

- Milestone 1 core contract. Dependencies: m1-schema-contract, m1-atomic-writer, m1-state-store. Source: THE_BEGINNING.md Sections 14.2 and 14.4.

## Requirements

- Implement the versioned context model, redaction rules, live-generation invariant, and atomic writer using the shared schema contract.
- Include identity, repositories, service state, routes/ports when available, commands, policy, and durability language.
- Exclude secrets, credential locations, Docker sockets, and unrelated Machine paths.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] The core writer produces a document matching supplied live observations and refuses an incomplete or stale generation.
- [ ] Schema validation and golden files cover stable field names and additive compatibility.
- [ ] Secret canaries and unrelated paths are absent.

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
- [ ] Validate all generated examples against checked-in JSON schemas

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement generate versioned agent context and output schemas for Agent Box according to the cited specification and dependencies.
