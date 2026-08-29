# Architecture Decision Records

This directory contains durable decisions that constrain Agent Box architecture, compatibility, security, persistence, or lifecycle behavior.

## Naming and numbering

Use `NNNN-short-kebab-title.md`. Allocate the next unused four-digit number; never renumber an accepted record. The number is an identifier, not priority or milestone order.

## Required sections

Each ADR must contain:

1. title and metadata (`Status`, `Date`, and `Scope`);
2. context and the invariant or pressure requiring a decision;
3. decision;
4. alternatives considered;
5. consequences, including failure, security, portability, and operational costs where relevant;
6. concrete evidence or upstream contracts; and
7. measurable revisit triggers.

Valid statuses are `Proposed`, `Accepted`, `Superseded by ADR NNNN`, and `Rejected`. Change a proposed ADR in place. Once accepted, preserve its historical decision; use a new ADR to supersede it when the contract changes. Corrections that do not alter the decision may be made in place with clear commit history.

Do not use an ADR to manufacture evidence. Separate verified behavior, design intent, and unresolved platform gates explicitly.

## Decision boundaries

ADR 0001 is authoritative for what the Milestone 0 probes actually demonstrated and for which platform gates remain unaccepted. ADRs 0002 through 0010 define the v0.1 design contracts that product work must implement within that evidence boundary. An `Accepted` design ADR is not a platform support claim: where ADR 0001 records a mechanism as untested or deferred, that limitation wins until the named gate supplies real execution evidence or a later ADR supersedes the design.

Each required domain has one owning ADR below. Cross-links describe dependencies between decisions; they do not create a second owner for the same contract.

## Index

| ADR | Status | Decision |
| --- | --- | --- |
| [0001](0001-milestone-0-substrate-gate.md) | Accepted | Gate product implementation on measured substrate evidence |
| [0002](0002-one-lima-vm-per-box.md) | Accepted | Isolate each Box in one Lima virtual machine |
| [0003](0003-limactl-adapter.md) | Accepted | Integrate Lima through a typed, pinned `limactl` adapter |
| [0004](0004-private-git-stores.md) | Accepted | Contain Git metadata in one private store per repository and Box |
| [0005](0005-invariant-paths.md) | Accepted | Preserve canonical absolute paths across Machine, VM, and container |
| [0006](0006-compose-ownership.md) | Accepted | Keep service graphs in Compose and inject Box policy through generated files |
| [0007](0007-user-daemon-api.md) | Accepted | Expose one versioned daemon API over a user Unix socket |
| [0008](0008-sqlite-observed-state.md) | Accepted | Persist intent and operations in SQLite, but recover from observed state |
| [0009](0009-loopback-routing.md) | Accepted | Route declared services through loopback-only Box endpoints |
| [0010](0010-security-boundaries.md) | Accepted | Bound untrusted workloads with a VM, explicit mounts, and reference-only secrets |
