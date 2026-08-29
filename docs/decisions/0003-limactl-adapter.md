# ADR 0003: Integrate Lima through a typed `limactl` adapter

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 Machine-to-Lima integration boundary

## Context

Agent Box must create, inspect, start, stop, and delete Lima instances; execute guest commands; and configure mounts and forwards. Importing unstable Lima implementation packages would couple the product to internal Go APIs. Blindly shelling out would instead couple correctness to human output, ambient configuration, unbounded processes, and unsafe interpolation.

## Decision

Use a narrow Go adapter around a pinned, capability-checked `limactl` executable. The adapter is a first-class Lima integration boundary, not an arbitrary command wrapper.

Every invocation must:

- receive `context.Context` and honor cancellation and deadlines;
- pass an argv array directly, never an interpolated shell command;
- capture stdout and stderr separately with explicit size bounds;
- request stable machine-readable output when Lima exposes it and decode it strictly;
- classify exit status, cancellation, timeout, malformed output, incompatibility, and not-found conditions as typed adapter errors;
- record the supported Lima version and required capabilities before mutation; and
- derive instance identity, mounts, forwards, and ownership from the immutable Plan, then confirm observed state independently after mutation.

The adapter may use Lima's control/SSH channel for pinned guest tools. v0.1 adds no custom guest daemon. Product code depends on the adapter's domain contract, not command prose or Lima's internal Go packages.

## Alternatives considered

- Import Lima internal Go packages: rejected because their source-level compatibility is not the supported external contract and would spread Lima types through the core.
- Invoke `limactl` ad hoc from each lifecycle path: rejected because process ownership, version checks, parsing, cancellation, and error mapping would drift.
- Add an Agent Box guest daemon: rejected until event streaming or guest coordination has a measured requirement that Lima and Docker cannot satisfy.
- Reimplement Lima lifecycle and SSH: rejected as redundant, high-risk virtualization machinery.

## Consequences

- Lima remains an explicit architectural dependency while the rest of the runtime sees a small replaceable contract.
- Supported Lima versions and output capabilities become a compatibility matrix and preflight requirement.
- CLI contract changes can block a release and require fixture updates.
- Cancellation kills or terminates the owned client process, but reconciliation must still inspect the VM because the remote side effect may have completed.
- Guest commands inherit the same argv, timeout, bounded-output, redaction, and postcondition requirements.

## Evidence

- [ADR 0001](0001-milestone-0-substrate-gate.md) requires preflight and observed-state recovery while recording that no Lima runtime was exercised.
- Lima's [`limactl create`](https://lima-vm.io/docs/reference/limactl_create) and [`limactl start`](https://lima-vm.io/docs/reference/limactl_start) references expose explicit VM type, mount type, mount list, static forward, resource, and instance-name controls.
- [Lima mount documentation](https://lima-vm.io/docs/config/mount) distinguishes VZ VirtioFS from QEMU and documents mount-specific caveats.
- No product adapter exists yet; its executable behavior belongs to `m1-process-runner`, `m1-lima-adapter`, and the real Darwin gate. These upstream references prove interface availability, not Agent Box support.

## Revisit triggers

Supersede this ADR if:

- a supported Lima release removes or destabilizes machine-readable lifecycle output needed by the adapter;
- compatibility tests cannot support two consecutive selected Lima releases without parsing human prose;
- Lima publishes a stable, versioned Go API covering the required lifecycle, inspection, guest-exec, mount, and forwarding operations;
- failure injection shows cancelled or killed invocations cannot be reconciled without duplicate or foreign resources; or
- measured guest event latency or command overhead requires a guest service and that service has a specified upgrade, authentication, recovery, and removal contract.
