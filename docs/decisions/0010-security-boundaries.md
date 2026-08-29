# ADR 0010: Bound untrusted workloads with a VM, explicit mounts, and reference-only secrets

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 security and trust boundaries

## Context

Repository code, dependencies, images, and coding-agent commands may be buggy or malicious. The Machine user intentionally grants write access to one Box's source and usually its private Git metadata, but unrelated Machine files, credentials, sibling Boxes, runtime daemons, and control-plane state must not become ambient inputs. VM isolation can reduce blast radius; it does not make mounted data safe or defend against every hypervisor, kernel, or same-user attack.

## Decision

Treat the Machine, Lima runtime host, container runtime, workload containers, and daemon as distinct trust boundaries.

The Lima mount allowlist contains only declared worktrees, corresponding Box-private Git directories at the selected mode, and required generated guest inputs. It excludes the home directory, SSH/cloud/browser credentials, Machine Docker socket, unrelated repositories, and state database.

Docker runs rootful only inside the Box VM for Compose compatibility. Workspace and support services receive no Docker socket by default. Reject privileged mode, host PID/IPC/network namespaces, device mounts, Docker socket mounts, and equivalent boundary escapes. A future unsafe mode requires a separate decision and explicit user opt-in.

Secrets remain references in manifests and Plans. The daemon resolves them at apply time, registers exact and common encoded forms with redaction before reading child output, prefers guest `tmpfs` files, and permits environment delivery only when declared. Secret values are excluded from SQLite, operation events, stored normalized models, diagnostics, errors, and normal logs. Temporary material is removed with the generation. Failure to resolve a secret fails closed.

Arbitrary repository hooks never run on the Machine. Machine-side Git, Lima, and filesystem operations use typed argv adapters; setup runs inside the workspace service. Release inputs and downloaded tools/images are version-pinned and digest-verified under the release supply-chain tasks.

This boundary protects unrelated Machine assets from ordinary workload compromise. It does not promise containment from a hypervisor exploit, malicious guest kernel, Machine-user compromise, or corruption/deletion of the active Box's mounted work.

## Alternatives considered

- Mount the Machine home directory for convenience: rejected because credentials and unrelated repositories would become ambient workload access.
- Mount the Machine Docker socket: rejected because it grants effective control of Machine containers and mounts.
- Rootless Docker inside the VM for v0.1: deferred because Compose/image compatibility and nested user-namespace/storage behavior require their own measured matrix.
- Pass all secrets as environment variables: rejected because container inspection and process environments expose them more broadly than file delivery.
- Run repository setup hooks on the Machine: rejected because untrusted project content would cross the VM boundary as code execution.
- Claim the VM protects mutually hostile tenants: rejected; that is not the v0.1 threat model.

## Consequences

- Some Compose projects are rejected until dangerous settings are removed or a future explicit policy exists.
- The active Box can lose mounted source, private Git state, or exposed secrets if its workload is compromised.
- Rootful Docker increases guest compromise impact but remains inside the per-Box VM and explicit mount boundary.
- Redaction is defense in depth, not permission to log secrets; transformed or application-generated derivatives may evade exact-value filters.
- Mount denial, secret residue, diagnostics, socket absence, and host-command injection need negative tests before security claims.

## Evidence

- [ADR 0001](0001-milestone-0-substrate-gate.md) accepts ownership/refusal mechanisms on Linux but explicitly leaves Lima mount isolation unaccepted.
- [The substrate matrix](../architecture/substrate-spikes.md) records ownership-aware Docker cleanup and source/Git containment; it created no VM and exercised no secret-delivery implementation.
- [`m0-lima-isolation-probe`](../../.sprout/tasks/m0-lima-isolation-probe.md), `m5-compose-security`, `m5-secret-delivery`, and [`m5-darwin-arm64-hardware-gate`](../../.sprout/tasks/m5-darwin-arm64-hardware-gate.md) own the negative boundary tests.
- The current [security policy](../../SECURITY.md) states that pre-alpha Agent Box is not yet a supported security boundary.

## Revisit triggers

Supersede this ADR if:

- an undeclared Machine canary, credential directory, Docker socket, state database, or sibling-Box resource is visible from a guest or workload;
- a forbidden Compose capability, namespace, device, socket, or publication survives normalized-model validation;
- any fixture secret appears in persisted state, Plans, events, diagnostics, normal output, process argv, or residue after generation removal;
- rootless Docker passes the full Compose compatibility, storage, cgroup, networking, performance, restart, and cleanup matrix with no critical regression; or
- mutually hostile tenants, unsafe privileged workloads, remote control, or broader Machine integration enters supported scope and receives a new threat model.
