# Implemented features

These documents explain implemented Agent Box components, their boundaries, and the verification evidence recorded for each. They document delivered components rather than claiming full release support for every future milestone; see the [roadmap](../ROADMAP.md), [Apple Silicon M1 hardware gate](architecture/apple-silicon-m1-gate.md), and [product baseline](../THE_BEGINNING.md) for overall delivery and support boundaries.

A capability is documented here only after its acceptance criteria and verification checks pass with concrete recorded evidence. Cross-compilation is not Darwin runtime evidence.

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

## Maintaining feature documentation

1. Document each delivered component under `docs/features/` using a descriptive kebab-case filename.
2. Include the component's scope and dependencies, behavior, constraints, verified acceptance criteria, implementation details, verification checklist, and concrete validation evidence.
3. Index the document from this page, update any affected top-level guides or ADRs, and run `make automation-check` to verify link integrity and document structure before review.

Commands in “Validation evidence” are historical records. Use `make check` for the current contributor workflow.
