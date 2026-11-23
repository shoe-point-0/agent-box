# Agent Box Roadmap

Agent Box is pre-alpha. This roadmap communicates direction, not release dates or support claims. A capability is complete only when it passes its verification gates with recorded execution evidence and is documented under [implemented feature documentation](docs/features.md).

## Delivery principles

- Implement and continuously verify on Linux first.
- Treat real Apple Silicon Lima VZ and VirtioFS execution as a final hardware gate; cross-compilation is not runtime evidence.
- Land the smallest end-to-end Box lifecycle before expanding integrations.
- Require ownership proof and deterministic cleanup for every created resource.
- Earn compatibility and performance claims with retained tests and measurements.

## Milestones

### Milestone 0: substrate evidence

Linux-exercisable Git, Compose, routing, and failure probes are recorded in [`docs/architecture/substrate-spikes.md`](docs/architecture/substrate-spikes.md). Full multi-version macOS release validation for Lima VZ, VirtioFS, mount isolation, and concurrent two-VM forwarding remains gated on real Darwin/arm64 hardware in Milestone 5.

### Milestone 1: one-repository Box

Build the user daemon, versioned API and CLI, one private Git worktree, one Lima runtime host, Compose reconciliation, context output, and safe removal. The single-repository `abox up` vertical slice and refusal-first `abox rm` have passed their Apple M1 hardware gate ([`docs/architecture/apple-silicon-m1-gate.md`](docs/architecture/apple-silicon-m1-gate.md)); completed components are recorded under [implemented feature documentation](docs/features.md). Remaining Milestone 1 work covers `abox status`/`abox down`, `abox exec`/`abox logs`/`abox shell`, `abox context`, post-removal SQLite state transitions for `abox list`, default macOS Application Support socket path handling, and the full Milestone 1 end-to-end gate.

### Milestones 2 and 3: multi-repository workspaces and routing

Add remote repository resolution, multi-repository Compose graphs, setup jobs, cache volumes, publication policy, the in-Box HTTP gateway, Machine router, stable TCP endpoints, readiness, and route commands after the single-repository gate.

### Milestone 4: recovery and fault containment

Reconstruct observed state, centralize descriptor-relative destructive ownership proofs, implement repair and diagnostics, serialize concurrent mutation, exercise lifecycle fault injection, and prove orphan collection and recovery end to end.

### Milestone 5: releasable v0.1 candidate

Finish secret delivery and Compose security boundaries, publish the threat model, harden compatibility and tool bootstrap, measure user-visible behavior, produce reproducible signed artifacts with SBOM and provenance, document installation and operations, and pass the acceptance matrix. Real multi-version Darwin/arm64 Lima VZ and VirtioFS validation remains the final pre-RC hardware gate.

On both minimum and current macOS, install the signed or test-signed artifact on a clean Apple Silicon Machine without Docker Desktop or manual Lima setup. Run the same acceptance fixtures through `doctor`, `up`, a prompt via `exec`, `status`, `logs`, `down`/`up`, failure diagnostics, and refusal-first removal. Record exact mounts, Git and workspace visibility, two-VM forwarding, routes, readiness, and context. After crash/restart and removal, verify that dirty or unpublished work survives and inventory remaining resources for leaks. Retain a redacted machine-readable evidence bundle per platform and rerun the gate after fixes.

### Milestone 6: evidence-triggered extensions

IDE integrations, an MCP adapter, native Linux backend, managed TLS, checkpoints, Dev Container import, and alternate engines are deferred. They become candidates only when the triggers in [`THE_BEGINNING.md`](THE_BEGINNING.md) are met; listing them is not a commitment.

## Tracking and changes

The product baseline is [`THE_BEGINNING.md`](THE_BEGINNING.md) and implemented evidence is in [implemented feature documentation](docs/features.md). Durable architectural changes require the ADR process in [`docs/decisions/README.md`](docs/decisions/README.md). Roadmap priority changes use an ordinary public pull request unless they alter a durable contract.

Foundation membership is not a roadmap milestone. The project will consider neutral-home discussions only after the community evidence in [`PROJECT_HEALTH.md`](PROJECT_HEALTH.md) demonstrates repeat adoption, multi-organization maintenance, and sustainable operations.
