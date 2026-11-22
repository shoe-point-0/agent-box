# Implemented features

These documents explain implemented Agent Box components, their boundaries, and the verification evidence recorded for each. They document delivered components rather than claiming full release support for every future milestone; see the [roadmap](../ROADMAP.md), [Apple Silicon M1 hardware gate](architecture/apple-silicon-m1-gate.md), and [product baseline](../THE_BEGINNING.md) for overall delivery and support boundaries.

Work in progress stays in `.sprout/tasks/`. A task becomes a feature document only after all acceptance criteria and verification items pass, concrete evidence is recorded, and `sprout close <id>` succeeds. Partial implementations and hardware-blocked tasks remain open; cross-compilation is not Darwin runtime evidence.

## Find a component

### CLI commands

- [Starter workspace manifests with `abox init`](features/abox-init.md)
- [Read-only reconciliation plans with `abox plan`](features/abox-plan.md)
- [Single-repository up vertical slice (`abox up`)](features/abox-up.md)
- [Box inventory with `abox list`](features/abox-list.md)
- [Machine and Box prerequisite diagnostics (`abox doctor`)](features/abox-doctor.md)
- [Refusal-first Box removal (`abox rm`)](features/abox-rm.md)

### Control plane and contracts

- [CLI transport and output contracts](features/cli-transport.md)
- [Versioned daemon API over a Unix socket](features/daemon-api.md)
- [Strict v1alpha1 Workspace loader](features/manifest-loader.md)
- [Versioned Plan, context, output, and error contracts](features/schema-contracts.md)
- [Deterministic identity and platform paths](features/identity-and-paths.md)
- [Canonical immutable Plans](features/plan-compiler.md)
- [Plan change and conflict classification](features/plan-diff.md)
- [Operation dependency graph engine](features/reconciliation-engine.md)
- [Versioned agent context and output schemas](features/context-document.md)
- [Operation events and Box leases](features/operation-journal.md)
- [Destructive-operation ownership proofs](features/ownership-proofs.md)

### Git and repository isolation

- [Hardened Machine-side Git execution](features/git-machine-profile.md)
- [Read-only repository intent resolution](features/git-repository-resolver.md)
- [Self-contained Box-private Git stores](features/git-private-store.md)
- [Linked worktree lifecycle and observation](features/git-worktrees.md)
- [Box commit publication status](features/git-publication-status.md)

### Runtime host, Compose, and filesystem

- [Machine preflight checks](features/machine-preflight.md)
- [Verified runtime artifact inputs](features/toolchain-artifacts.md)
- [Lima runtime-host adapter](features/lima-runtime-host.md)
- [Guest toolchain provisioning and verification](features/guest-toolchain.md)
- [Generated Compose inputs](features/compose-compiler.md)
- [Pre-apply Compose security policy](features/compose-security-policy.md)
- [Compose lifecycle, exec, logs, and observation](features/compose-runtime.md)
- [Mounted filesystem verification](features/filesystem-verification.md)

### State, networking, and durability

- [Crash-consistent atomic file writes](features/atomic-file-writer.md)
- [Safe subprocess execution](features/process-runner.md)
- [SQLite state and migrations](features/sqlite-state-store.md)
- [Stable raw TCP endpoints](features/tcp-port-allocator.md)
- [SQLite and generated-file durability failures](features/durability-fault-testing.md)

### Substrate validation spikes

- [Disposable Go spike workspace](features/spike-workspace-bootstrap.md)
- [Substrate spike harness](features/spike-harness.md)
- [Private Git-store path invariance](features/spike-git-path.md)
- [Multi-repository Compose include semantics](features/spike-compose-include.md)
- [Wildcard localhost routing through one listener](features/spike-localhost-routing.md)
- [Crash residue at substrate boundaries](features/spike-failure-matrix.md)
- [Substrate evidence and architecture gates](features/spike-substrate-report.md)

### Project foundation and governance

- [Go project and contributor foundation](features/project-foundation.md)
- [v0.1 architecture decisions](features/architecture-decisions.md)
- [Project governance and maintenance](features/project-governance.md)

## Task-to-document workflow

1. Keep active requirements, dependencies, checklists, and partial evidence in `.sprout/tasks/<id>.md`.
2. Run `sprout check <id>` while working. Record the actual commands and observed results, including remaining limitations.
3. Run `sprout check --complete <id>` only when every criterion is true, then `sprout close <id>`. Do not close a task to tidy the backlog.
4. Rename the promoted `docs/features/<id>.md` to a descriptive, reader-facing kebab-case filename (without the `mN-` milestone prefix) and rewrite it for readers: a clear title, delivered behavior, scope, constraints, implementation details, verified acceptance criteria, and retained verification evidence. Remove the execution checklist, original agent request, and instructions to execute the task. Preserve the hidden `<!-- sprout-task ... -->` metadata header verbatim for provenance and frontier tracking.
5. Add its link to this index and update any affected public guides. Run `sprout check --all`, `python3 scripts/sprout_frontier.py --json`, and `make automation-check` before review.

Historical Sprout checks in validation evidence describe commands that actually ran; they are not requirements for readers to install Sprout to understand a feature. Sprout is contributor tooling, not an Agent Box runtime dependency.
