# ADR 0002: Isolate each Box in one Lima virtual machine

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 runtime-host and Box lifecycle boundary

## Context

A Box must own one workspace, its container graph, networks, volumes, and failure lifecycle without sharing a container daemon or mounted source with sibling Boxes. Containers isolate processes but share a guest kernel and daemon; they do not provide the runtime-host boundary required by the v0.1 threat model. The product also needs one cleanup and recovery unit whose identity is stable across CLI and daemon restarts.

## Decision

Each Box owns exactly one Lima-managed Linux VM. That VM is the Box's runtime host and contains one rootful Docker Engine, one Compose project, Box-scoped networks and volumes, and no runtime resources from sibling Boxes.

The Machine-side worktrees and Box-private Git stores remain canonical; replacing the VM must not normally replace them. Runtime-host creation, adoption, stop, and deletion are serialized under the Box lease and require the recorded Lima instance identity plus embedded Box metadata. A name alone is not ownership proof.

Rootful Docker is confined to the disposable VM. The Docker socket is not mounted into workspace or support containers. A container escape reaches the Box runtime host and explicitly mounted Box data; this decision does not claim protection from hypervisor exploits or Machine-user compromise.

Darwin VZ isolation, mount allowlisting, VirtioFS, forwarding, and cleanup are design obligations, not accepted runtime facts, until the real-hardware gate passes.

## Alternatives considered

- One shared VM for all Boxes: rejected for v0.1 because it shares the daemon, kernel, networks, volumes, failure domain, and cleanup authority across tasks.
- Containers directly on the Machine: rejected because it weakens filesystem and daemon containment and makes Machine runtime differences part of the product contract.
- One VM per service: rejected because it discards Compose networking and makes ordinary development graphs operationally expensive.
- A custom hypervisor integration: rejected because Lima already owns cross-platform VM lifecycle and guest access; Agent Box should own coordination, not virtual hardware.

## Consequences

- Box cleanup has a coarse, inspectable runtime boundary and sibling Boxes do not share Docker state.
- Concurrent Boxes consume more memory, disk, and startup time than a shared VM.
- VM loss can destroy VM-local caches and volumes. It must not be described as deleting the canonical Machine worktree, though the accepted one-Box corruption envelope still includes uncommitted and unpushed work.
- A stopped Box retains its Machine-side worktrees, private Git stores, allocation records, and volumes until explicit removal.
- Linux engineering can implement lifecycle logic, but shipped Darwin support remains blocked on real Apple Silicon execution.

## Evidence

- [ADR 0001](0001-milestone-0-substrate-gate.md) defines the evidence boundary and leaves Lima isolation unaccepted.
- [Milestone 0 substrate matrix](../architecture/substrate-spikes.md) records that Lima and a guest image were unavailable on the executed Linux matrix.
- [`m0-lima-isolation-probe`](../../.sprout/tasks/m0-lima-isolation-probe.md) and [`m5-darwin-arm64-hardware-gate`](../../.sprout/tasks/m5-darwin-arm64-hardware-gate.md) own the unchanged mount, forwarding, concurrency, and leak checks.
- Lima documents VZ, VirtioFS, explicit mounts, and static port-forward configuration in its [mount](https://lima-vm.io/docs/config/mount) and [`limactl create`](https://lima-vm.io/docs/reference/limactl_create) contracts. Those upstream capabilities are not Agent Box runtime evidence.

## Revisit triggers

Supersede this ADR if any of these conditions occurs:

- the real Darwin gate shows cross-Box mount, daemon, network, forwarding, or cleanup leakage;
- three concurrent fixture Boxes cannot run within the documented CPU, memory, disk, and startup budgets on the minimum supported Machine;
- measured usage shows VM startup or footprint prevents the accepted end-to-end latency target for the golden path;
- a shared-rootless or alternate runtime prototype passes the same sibling-isolation, ownership, crash-recovery, and cleanup suite with a smaller measured footprint; or
- mutually hostile tenants, a native Linux backend, or a non-Lima runtime enters supported scope.
