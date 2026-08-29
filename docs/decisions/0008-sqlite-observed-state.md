# ADR 0008: Persist intent and operations in SQLite, but recover from observed state

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 local persistence, ownership, and recovery authority

## Context

Agent Box coordinates state across Git, filesystems, Lima, Docker, listeners, and ports. A process can die between any external side effect and its database update. Treating either logs or a last-known status row as reality would create duplicates or unsafe deletion. The daemon still needs transactional local records for identity, leases, allocation, operation history, and schema evolution.

## Decision

Use one user-scoped SQLite database with WAL mode and foreign-key enforcement for durable control-plane records. Schema migrations are monotonic and tested from every released schema. Every Box-owned row carries `box_id`; secrets never enter SQLite.

Each mutating request creates an operation with a stable ID, request digest, and monotonically sequenced append-only events. Before a side effect, persist its intended stable operation step and ownership inputs. After the call, inspect the external system and persist the observed result. Commit state-changing transactions before reporting completion.

SQLite is the record of intent, identity, leases, and prior observations; it is not sole proof that a VM, worktree, container, volume, route, or listener exists. On startup and after uncertain outcomes, reconciliation inspects typed external state, adopts only resources matching independent ownership facts, and refuses destructive ambiguity.

Generated Plan and context files are written atomically in their destination directory: complete temporary write, file sync, rename, and parent-directory sync where supported. Plans are immutable per reconciliation attempt and contain secret references only.

If the database is corrupt, stop mutation, preserve external resources, and require an explicit repair workflow. Recovery never guesses ownership in order to delete.

## Alternatives considered

- JSON files as the only state store: rejected because multi-record leases, uniqueness, allocation, operation sequencing, and migrations need transactions and constraints.
- Treat SQLite as exact external truth: rejected because crashes can occur between database and external side effects.
- Event sourcing as the only model: rejected because replay still cannot prove external state and adds unnecessary projection complexity.
- A remote database or distributed consensus: rejected because one user daemon owns local resources on one Machine.
- Automatically rebuild a corrupt database and delete unmatched resources: rejected because missing state is not evidence of ownership.

## Consequences

- Reconciliation and typed observation are product behavior, not a cleanup fallback.
- Database transactions remain short and do not wrap slow external commands.
- WAL/checkpoint, disk-full, corruption, migration, backup, and concurrent-reader behavior require tests.
- Operations can be audited and reattached without using logs as state.
- Filesystem durability depends on the host filesystem honoring sync/rename contracts; source work is still governed by the documented one-Box loss envelope.

## Evidence

- [`m0-failure-matrix`](../features/m0-failure-matrix.md) killed the harness before and after eight Linux side effects, then converged from observed residue without duplicates and refused foreign or dirty resources. It validates the recovery model, not SQLite.
- [ADR 0001](0001-milestone-0-substrate-gate.md) makes stable operation IDs, independent observation, exact ownership, and reverse cleanup normative for product work.
- SQLite documents [write-ahead logging](https://sqlite.org/wal.html), [foreign-key enforcement](https://sqlite.org/foreignkeys.html), and its [atomic commit model](https://sqlite.org/atomiccommit.html). The eventual driver and durability pragmas must be verified by `m1-state-store`.
- State corruption and migration behavior remain acceptance work in Milestones 1, 4, and 5.

## Revisit triggers

Supersede this ADR if:

- crash or disk-full tests lose a committed operation sequence, violate a uniqueness/foreign-key invariant, or permit mutation after corruption;
- startup reconciliation creates duplicates or deletes a resource using only database state or a name prefix;
- measured checkpoint or writer contention exceeds the documented daemon latency budget under the concurrent-CLI fixture;
- a supported filesystem cannot satisfy the tested atomic Plan/context replacement contract; or
- multi-user or multi-Machine ownership becomes supported and requires an authorization or consistency model SQLite cannot provide.
