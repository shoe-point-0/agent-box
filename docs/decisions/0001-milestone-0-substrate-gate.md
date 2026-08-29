# ADR 0001: Gate implementation on measured substrate evidence

- Status: Accepted for the Linux implementation lane
- Date: 2026-08-29
- Scope: Milestone 0 substrate decisions only

## Context

Agent Box's v0.1 architecture depends on private Git stores, invariant paths across runtime layers, Compose include/override semantics, wildcard-localhost routing, Lima VZ, and VirtioFS. Treating those mechanisms as portable assumptions would let product code harden around behavior that has not been observed. The Linux lane can exercise Git, Docker Compose, local routing, ownership, cleanup, and crash residue now. It cannot prove Apple Virtualization.framework, VirtioFS, Darwin resolver behavior, or Lima forwarding without real Apple Silicon hardware.

## Decision

Product implementation may advance from Milestone 0 only while every Linux-exercisable critical gate in [`substrate-spikes.md`](../architecture/substrate-spikes.md) is green on its stated matrix.

We accept these mechanism-level decisions for the Linux implementation lane:

- Box-private bare Git stores and locked linked worktrees, with no hardlinks or alternates to the source repository.
- Stock Git at identical absolute paths, conditional on a later successful VirtioFS transport test; relative-pointer rewriting is not a fallback.
- Compose-owned service graphs assembled with `include`, plus a generated override that removes Machine publications before apply, conditional on a pinned Compose version demonstrating the required normalization.
- Exact-authority `.abox.localhost` routing through one unprivileged IPv4 loopback listener, with fail-closed routing, trusted forwarding-header reconstruction, and explicit proxy/resolver preflight.
- Reconciliation from independently observed state using stable operation IDs, exact ownership evidence, idempotent adoption, reverse-order cleanup, and refusal when ownership or dirty-state proof is insufficient.

A red probe blocks affected product implementation. Changing or waiving a critical gate requires a superseding ADR and updated support boundary; test failure may not be relabeled as support.

Darwin VZ, VirtioFS, mount isolation, resolver/browser behavior, concurrent Lima forwarding, and their crash/cleanup boundaries remain unaccepted. They are mandatory obligations of `m0-virtiofs-probe`, `m0-lima-isolation-probe`, and `m5-darwin-arm64-hardware-gate` on real Darwin/arm64 hardware. A Darwin cross-build does not satisfy them.

## Alternatives considered

- Implement first and validate on macOS later: rejected because filesystem and forwarding behavior shape persistent paths, ownership, reconciliation, and cleanup contracts.
- Block all implementation until Apple hardware is available: rejected because the unchanged Linux-exercisable mechanisms have real passing evidence and the platform-specific gates are isolated and explicit.
- Rewrite linked-worktree pointers when path transport fails: rejected because it creates unsupported Git metadata and version-dependent corruption risk.
- Reimplement Compose merging or route discovery: rejected because pinned upstream semantics and explicit route intent are smaller and testable boundaries.

## Consequences

- Linux is an engineering and continuous-verification lane, not a shipped native backend claim.
- Tool versions and capability checks become Plan/preflight inputs rather than ambient assumptions.
- Product code must expose gate failures before mutation and retain enough evidence to diagnose them.
- Darwin support remains blocked until the final unchanged fixtures pass on the minimum and current supported macOS releases with leak-free cleanup.
- The broader v0.1 architecture ADR set remains the responsibility of `m1-architecture-decisions`; this ADR records only the substrate gate that those decisions must cite.

## Evidence

- [`m0-git-path-probe`](../features/m0-git-path-probe.md)
- [`m0-compose-probe`](../features/m0-compose-probe.md)
- [`m0-routing-probe`](../features/m0-routing-probe.md)
- [`m0-failure-matrix`](../features/m0-failure-matrix.md)
- [Aggregate matrix and reproduction commands](../architecture/substrate-spikes.md)

## Revisit triggers

Supersede or amend this ADR when any of the following occurs:

- the pinned Git, Docker, Compose, Lima, guest image, kernel, or supported host matrix changes;
- a critical probe turns red or a recovery path leaves residue;
- the real Darwin/arm64 gate disproves path invariance, VirtioFS coherency, mount isolation, resolver/proxy behavior, or forwarding assumptions;
- rootless runtime, alternate VM/runtime, IPv6 routing, or a native Linux backend enters supported scope.
