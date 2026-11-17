# Implemented features

These documents explain implemented components, their boundaries, and the evidence recorded when they were completed. They are implementation records, not a promise that the full Agent Box runtime is supported. Later work may extend a component; use the [roadmap](../ROADMAP.md) and [product baseline](../THE_BEGINNING.md) for the overall delivery and support gates.

Work in progress stays in `.sprout/tasks/`. A task becomes a feature document only after all acceptance criteria and verification items pass, concrete evidence is recorded, and `sprout close <id>` succeeds. Partial implementations and hardware-blocked tasks remain open; cross-compilation is not Darwin runtime evidence.

## Find a component

### Substrate evidence

- [Multi-repository Compose include semantics](features/m0-compose-probe.md)
- [Crash residue at substrate boundaries](features/m0-failure-matrix.md)
- [Private Git-store path invariance](features/m0-git-path-probe.md)
- [Disposable Go spike workspace](features/m0-project-bootstrap.md)
- [Wildcard localhost routing through one listener](features/m0-routing-probe.md)
- [Substrate spike harness](features/m0-spike-harness.md)
- [Substrate evidence and architecture gates](features/m0-substrate-report.md)

### Single-repository foundation

- [v0.1 architecture decisions](features/m1-architecture-decisions.md)
- [Crash-consistent atomic file writes](features/m1-atomic-writer.md)
- [CLI transport and output contracts](features/m1-cli-foundation.md)
- [Compose lifecycle, exec, logs, and observation](features/m1-compose-adapter.md)
- [Generated Compose inputs](features/m1-compose-compiler.md)
- [Pre-apply Compose security policy](features/m1-compose-policy-core.md)
- [Versioned agent context and output schemas](features/m1-context-document.md)
- [Versioned daemon API over a Unix socket](features/m1-daemon-api.md)
- [Machine and Box prerequisite diagnostics](features/m1-doctor-command.md)
- [Mounted filesystem verification](features/m1-filesystem-verifier.md)
- [Self-contained Box-private Git stores](features/m1-git-private-store.md)
- [Box commit publication status](features/m1-git-publication-status.md)
- [Linked worktree lifecycle and observation](features/m1-git-worktrees.md)
- [Guest toolchain provisioning and verification](features/m1-guest-toolchain.md)
- [Deterministic identity and platform paths](features/m1-identity-paths.md)
- [Starter workspace manifests with abox init](features/m1-init-command.md)
- [Lima runtime-host adapter](features/m1-lima-adapter.md)
- [Box inventory with abox list](features/m1-list-command.md)
- [Hardened Machine-side Git execution](features/m1-machine-git-profile.md)
- [Machine preflight checks](features/m1-machine-preflight.md)
- [Strict v1alpha1 Workspace loader](features/m1-manifest-loader.md)
- [Operation events and Box leases](features/m1-operation-journal.md)
- [Destructive-operation ownership proofs](features/m1-ownership-core.md)
- [Read-only reconciliation plans with abox plan](features/m1-plan-command.md)
- [Canonical immutable Plans](features/m1-plan-compiler.md)
- [Plan change and conflict classification](features/m1-plan-diff.md)
- [Safe subprocess execution](features/m1-process-runner.md)
- [Go project and contributor foundation](features/m1-project-foundation.md)
- [Operation dependency graph engine](features/m1-reconcile-engine.md)
- [Read-only repository intent resolution](features/m1-repository-resolver.md)
- [Refusal-first Box removal](features/m1-safe-remove.md)
- [Versioned Plan, context, output, and error contracts](features/m1-schema-contract.md)
- [SQLite state and migrations](features/m1-state-store.md)
- [Verified runtime artifact inputs](features/m1-toolchain-contract.md)

### Networking

- [Stable raw TCP endpoints](features/m3-tcp-allocator.md)

### Recovery and durability

- [SQLite and generated-file durability failures](features/m4-fault-state.md)

### Project governance

- [Project governance and maintenance](features/m5-governance-readiness.md)

## Task-to-document workflow

1. Keep active requirements, dependencies, checklists, and partial evidence in `.sprout/tasks/<id>.md`.
2. Run `sprout check <id>` while working. Record the actual commands and observed results, including remaining limitations.
3. Run `sprout check --complete <id>` only when every criterion is true, then `sprout close <id>`. Do not close a task to tidy the backlog.
4. Edit the promoted `docs/features/<id>.md` into reader-facing documentation: a descriptive title, delivered behavior, scope, constraints, implementation details, verified acceptance criteria, and retained verification evidence. Remove the execution checklist, original agent request, and instructions to execute the task. Preserve the hidden Sprout metadata for provenance and compatibility.
5. Add its link to this index and update any affected public guides. Run `sprout check --all`, `python3 scripts/sprout_frontier.py --json`, and `make automation-check` before review.

Historical Sprout checks in validation evidence describe commands that actually ran; they are not requirements for readers to install Sprout to understand a feature. Sprout is contributor tooling, not an Agent Box runtime dependency.
