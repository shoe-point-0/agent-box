<!-- sprout-task
{"schema_version":1,"id":"m2-cache-volumes","status":"open","created_at":"2026-08-29T06:48:36.309214171Z"}
-->

# Provision Box-scoped development cache volumes

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Provision Box-scoped development cache volumes

## Context

- Milestone 2. Dependencies: m2-multi-repo-compose. Source: THE_BEGINNING.md Sections 7 and 11.2.

## Requirements

- Translate declared caches into labeled Box-scoped named volumes with target validation.
- Add the cache-volume Plan projection, state migration, diff rules, and generated Compose fields with golden compatibility tests.
- Keep platform-specific dependencies and database data off VirtioFS by default.
- Preserve caches across down/up and gate deletion behind volume policy.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Cache names do not collide across Boxes.
- [ ] Only declared services receive each cache.
- [ ] Safe removal preserves volumes unless explicit deletion is requested.

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
- [ ] Run cache isolation and lifecycle integration tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement provision box-scoped development cache volumes for Agent Box according to the cited specification and dependencies.
