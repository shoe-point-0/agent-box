<!-- sprout-task
{"schema_version":1,"id":"m1-architecture-decisions","status":"implemented","created_at":"2026-08-29T06:48:35.940488707Z","implemented_at":"2026-08-29T15:20:27.059020152Z"}
-->

# Record v0.1 architecture decisions

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Record v0.1 architecture decisions

## Context

- Milestone 1. Dependencies: m0-substrate-report, m1-project-foundation. Source: THE_BEGINNING.md Sections 5, 10, 12, 14, 15, and 16.

## Requirements

- Write ADRs for Box isolation, limactl integration, private Git stores, path invariance, Compose ownership, daemon API, state, routing, and security boundaries.
- Include context, decision, alternatives, consequences, evidence, and revisit triggers.
- Reconcile ADR language with measured Milestone 0 results.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Every named decision domain has one indexed ADR: Box isolation, `limactl` integration, private Git stores, path invariance, Compose ownership, daemon API, state, routing, and security boundaries.
- [x] ADR 0001 remains the measured substrate gate; each broader ADR distinguishes accepted design intent, Linux evidence, and untested Darwin/Lima behavior instead of restating conflicting support claims.
- [x] Every ADR contains context, decision, alternatives, consequences, evidence, and concrete conditions that require reconsideration.

## Technical approach

- Keep ADR 0001 as the authoritative evidence gate and add one narrowly scoped accepted v0.1 design ADR for each required decision domain.
- Cross-link coupled decisions rather than merging them: isolation owns the VM lifecycle boundary, the Lima adapter owns the external integration contract, Git storage owns metadata containment, and path invariance owns filesystem topology.
- Cite the retained Milestone 0 probe artifacts where mechanisms ran. Mark Lima, VirtioFS, guest, and Darwin behavior as unverified design obligations and point to the real-hardware gate.
- Expand the ADR index into the discoverability and conflict-resolution map, then verify every indexed target, status, required section, and measurable revisit trigger mechanically.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] Verify ADR index links and decision status
- [x] sprout check --all

## Validation evidence

- A repository-local Python contract check passed for all 10 indexed ADRs: contiguous unique numbering, existing index targets, matching valid statuses, every required section, at least three explicit revisit conditions, and every relative Markdown link resolving to an existing file.
- `sprout check --all` passed for every active packet after the ADR set and task packet were updated.
- `make check` passed: `golangci-lint` reported `0 issues`; module tidy/verification, unit tests, race tests, vet, product and spike builds, spike listing, shell syntax, Git wrapper smoke, and all three automation tests passed.
- `git diff --check` passed with no whitespace errors.
- ADR 0001 now uses the repository's valid `Accepted` status spelling without changing its Linux-only evidence scope. The index explicitly makes ADR 0001 authoritative for measured support and ADRs 0002-0010 authoritative for design intent.

## Outcome and follow-ups

- Added one indexed ADR for each required v0.1 domain: Box/VM isolation, typed `limactl` integration, private Git stores, invariant paths, Compose ownership, the user daemon API, SQLite plus observed-state recovery, loopback routing, and security boundaries.
- Each ADR records rejected/deferred alternatives, operational and security consequences, concrete repository or upstream evidence, and measurable conditions that force reconsideration.
- Lima, VZ, VirtioFS, guest mount isolation, forwarding, and Darwin resolver behavior remain explicit real-hardware obligations. No cross-build or design record is presented as runtime support evidence.

## Original request

Implement record v0.1 architecture decisions for Agent Box according to the cited specification and dependencies.
