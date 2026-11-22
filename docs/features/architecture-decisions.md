<!-- sprout-task
{"schema_version":1,"id":"m1-architecture-decisions","status":"implemented","created_at":"2026-08-29T06:48:35.940488707Z","implemented_at":"2026-08-29T15:20:27.059020152Z"}
-->

# v0.1 architecture decisions

Status: implemented.

Agent Box maintains one indexed Architecture Decision Record (ADR) per v0.1
design domain under [docs/decisions/](../decisions/README.md), covering
Box/VM isolation, pinned `limactl` integration, private Git stores, invariant
paths, Compose ownership, the user daemon API, SQLite plus observed-state
recovery, loopback routing, and security boundaries.

Each ADR separates accepted design intent from measured platform evidence and
lists rejected alternatives, operational/security consequences, and measurable
revisit triggers. [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md)
subsequently amended [ADR 0005](../decisions/0005-invariant-paths.md)'s
filesystem timing and event assumptions following Apple M1 VZ runs.

## Scope and dependencies

- Milestone 1 architecture baseline. Depends on [substrate evidence report](spike-substrate-report.md) and [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Sections 5, 10, 12, 14, 15, and 16.

## Behavior

- Defines one owning ADR per core v0.1 contract (`0002` through `0010`) alongside the Milestone 0 evidence gate in [ADR 0001](../decisions/0001-milestone-0-substrate-gate.md).
- Documents context, decision, alternatives considered, operational/security consequences, concrete evidence or upstream contracts, and measurable revisit triggers in every record.
- Reconciles ADR support statements with measured substrate results so design records never overstate unvalidated platform support.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Every required v0.1 decision domain has one indexed ADR: Box isolation, `limactl` integration, private Git stores, path invariance, Compose ownership, daemon API, state, routing, and security boundaries.
- ADR 0001 remains authoritative for measured Milestone 0 evidence, while ADRs 0002–0010 separate design intent, Linux evidence, and real-hardware obligations.
- Every ADR includes context, decision, alternatives, consequences, evidence, and concrete revisit conditions.

## Implementation details

- [docs/decisions/README.md](../decisions/README.md) indexes all ADRs, defines valid statuses (`Proposed`, `Accepted`, `Superseded by ADR NNNN`, `Rejected`), and establishes conflict precedence between evidence-gate records and design records.
- Coupled domains cross-link rather than duplicating ownership: VM isolation (`0002`), typed `limactl` integration (`0003`), private Git storage (`0004`), and invariant paths (`0005`) each own one distinct boundary.
- Records cite Milestone 0 probe artifacts where mechanisms executed on Linux and explicitly mark unexercised Darwin/Lima behavior as hardware-gate obligations.

## Verification

- [x] Verify ADR index links and decision status
- [x] `sprout check --all`

## Validation evidence

- Repository contract validation checked all 10 initial indexed ADRs (`0001`–`0010`): contiguous unique numbering, existing index targets, valid statuses, all required sections, at least three revisit conditions each, and resolving relative links.
- `sprout check --all`, `make check` (`0` lint issues, unit/race tests, vet, builds, smoke, and automation checks), and `git diff --check` passed.
