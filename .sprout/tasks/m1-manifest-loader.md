<!-- sprout-task
{"schema_version":1,"id":"m1-manifest-loader","status":"open","created_at":"2026-08-29T06:48:35.969030304Z"}
-->

# Implement the strict v1alpha1 Workspace loader

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the strict v1alpha1 Workspace loader

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 7 and Issue 2.

## Requirements

- Define the Workspace schema and strict YAML decoding with unknown-field rejection.
- Validate uniqueness, references, argv-only setup commands, secret references, templates, and normalized quantities.
- Canonicalize paths against explicit roots and reject traversal and symlink escapes.
- Separate the complete v1alpha1 syntax model from the smaller Milestone 1 executable subset; recognized remote sources, caches, setup jobs, routes, TCP ports, and secrets must fail planning with a stable field-aware feature-not-supported error until their owning milestone lands.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] All documented manifest examples parse to the expected model.
- [ ] Every validation rule and malicious path fixture has a table-driven test.
- [ ] Errors identify a stable field path and never expose secret values.
- [ ] Parser limits for input size, nesting, collection counts, path length, and template output are enforced and fuzzed.

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
- [ ] Run manifest fuzz tests for parser and path boundaries

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement implement the strict v1alpha1 workspace loader for Agent Box according to the cited specification and dependencies.
