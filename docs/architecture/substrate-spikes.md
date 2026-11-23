# Milestone 0 substrate evidence

Status: Linux implementation gate GREEN; Darwin/arm64 M1 single-repository VZ gate GREEN (revised filesystem contract in [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md), report in [apple-silicon-m1-gate.md](apple-silicon-m1-gate.md), evidence in [apple-silicon-m1-evidence.json](apple-silicon-m1-evidence.json)); full Darwin/arm64 M5 release matrix OPEN.

This report is the support boundary for Milestone 0. It accepts only mechanisms exercised on the matrix below plus the recorded M1 Apple Silicon single-repository evidence. It does not claim full multi-version macOS release support until the [Milestone 5 Darwin/arm64 hardware gate](../../ROADMAP.md#milestone-5-releasable-v01-candidate) passes.

## Reproduce

Build once, then run each retained-evidence scenario. Each command prints an `abox.spike.result/v1` document whose `root` must be inspected before cleanup.

```text
go build -o /tmp/abox-spike-m0-report ./cmd/abox-spike
/tmp/abox-spike-m0-report run --parent /tmp/abox-substrate-report git-path
/tmp/abox-spike-m0-report inspect <root-from-result>
/tmp/abox-spike-m0-report cleanup <root-from-result>

/tmp/abox-spike-m0-report run --parent /tmp/abox-substrate-report compose-include
/tmp/abox-spike-m0-report inspect <root-from-result>
/tmp/abox-spike-m0-report cleanup <root-from-result>

/tmp/abox-spike-m0-report run --parent /tmp/abox-substrate-report localhost-routing
/tmp/abox-spike-m0-report inspect <root-from-result>
/tmp/abox-spike-m0-report cleanup <root-from-result>

/tmp/abox-spike-m0-report run --parent /tmp/abox-substrate-failure-report failure-matrix
/tmp/abox-spike-m0-report inspect <root-from-result>
/tmp/abox-spike-m0-report cleanup <root-from-result>
```

A probe result other than `status: passed`, a failed ownership-aware cleanup, or any remaining resource carrying `dev.abox.spike.owner` makes this gate red. Product implementation must stop until the failed mechanism is corrected or the affected architecture decision is changed explicitly.

## Executed development matrix

Fresh aggregate run: 2026-08-29T09:31:16Z through 2026-08-29T09:36:24Z.

| Fact | Executed value | Claim boundary |
| --- | --- | --- |
| Machine OS | Ubuntu 24.04.4 LTS; Linux `7.0.0-28-generic` | Linux development evidence only |
| Machine architecture | `x86_64` / Go `linux/amd64` | No arm64 runtime evidence |
| Go | `go1.26.3` | Probe implementation and tests |
| Machine Git | `2.43.0` | Machine-side Git cases |
| Container Git | `2.47.3`, cached `golang:1.26` image ID `sha256:73204d15b474cccdcf60645cfc88bf3d8d68cbc1687334c70f66bd03832e8259` | Stock Git through a Linux bind mount; networking disabled |
| Docker client/server | `29.6.1`, API `1.55` | Local Linux daemon only |
| Docker runtime | containerd `2.2.5`, runc `1.3.6`, overlay2 on extfs, cgroup v2 | Observed development daemon; not a guest-image contract |
| Docker Compose | `5.3.0` | `include` plus `!reset []` behavior only at this version |
| Compose base image | cached `alpine@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc` | Fixture only; no pull required |
| curl | `8.5.0` | Linux routing client |
| Chromium | `151.0.7922.108` snap | Linux browser resolver path |
| Lima | unavailable | No Linux Lima result and no Lima support claim |
| Guest image | none | No VM or guest-image result |
| Darwin/macOS | not executed | Must remain unclaimed until the real-hardware gate passes |

## Probe matrix

| Milestone 0 gate | Linux result and reproducible evidence | Cleanup evidence | Remaining boundary |
| --- | --- | --- | --- |
| Private Git store, locked linked worktree, identical absolute paths, and path edge cases | GREEN: [`git-path`](../features/spike-git-path.md). Fresh run passed from `09:31:16Z` to `09:31:28Z`; plain, spaces, Unicode, 200-character, and symlinked-source fixtures passed Machine/container status, add, commit, log, lock, inode-isolation, and source-metadata checks. | Harness cleanup removed the marker-owned fixture tree and run root; no probe container remained. | Identical paths are proven through a Linux bind mount, not VirtioFS. Git 2.43.0 and 2.47.3 did not advertise `worktree.useRelativePaths`; manual pointer rewriting remains forbidden. |
| Two-repository Compose `include` and relative build contexts | GREEN: [`compose-include`](../features/spike-compose-include.md). Fresh run passed from `09:31:28Z` to `09:31:30Z`; normalized contexts remained owned by their repositories and both identity-specific images ran. | Exact Compose and custom owner labels were checked before project-scoped teardown; built fixture images, containers, network, tree, and run root were absent afterward. | Proven only with Compose 5.3.0 and the local Linux daemon. |
| Remove repository-owned Machine port publications | GREEN: same [`compose-include`](../features/spike-compose-include.md) run. Baseline publications existed; the generated override's `!reset []` removed them before apply; Docker inspection and `docker port` showed no publication. | Same ownership-verified teardown; no owner-labeled Docker resource remained. | A future toolchain contract must reject Compose versions that cannot normalize the required reset behavior. |
| Route two `*.abox.localhost` names through one unprivileged listener | GREEN: [`localhost-routing`](../features/spike-localhost-routing.md). Fresh run passed from `09:31:41Z` to `09:31:42Z`; exact-host fail-closed routing, trusted-header reconstruction, curl, Chromium, WebSocket, SSE, and bidirectional streaming checks passed on IPv4. | In-process listeners closed synchronously; the marker-owned Chromium profile and run root were removed. | Resolver returned IPv4 only. `.localhost` bypassed a controlled proxy only when no-proxy behavior applied; ambient enterprise proxies, IPv6, Lima forwarding, and Darwin resolver behavior are unclaimed. |
| Kill before/after each Linux substrate side effect and recover from observed residue | GREEN: [`failure-matrix`](../features/spike-failure-matrix.md). Fresh run passed from `09:31:57Z` to `09:36:24Z`; all 16 real-`SIGKILL` cases for eight stable operations matched expected residue, adopted exact ownership, avoided duplicates, and converged. Dirty-worktree and foreign-volume cases refused without mutation. | Reverse-order recovery reached zero residue in every case. Harness cleanup removed the run root; independent container/network/volume queries for `dev.abox.spike.owner` returned empty. | Lima, VirtioFS, and Darwin forwarding kill points retain the stable-operation-ID obligation at the final hardware gate. |
| VirtioFS read/write, rename/delete visibility, mode, and watcher behavior in both directions | Single-repo M1 verified in [apple-silicon-m1-gate.md](apple-silicon-m1-gate.md) under [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md); multi-version matrix OPEN | Verified on Apple M1 (`mountInotify: true`, bounded 2s visibility retries); zero probe or watcher residue. | Minimum and current supported macOS release matrix remains required in Milestone 5. |
| Lima mount allowlist excludes home and undeclared canaries | Single-repo M1 verified in [apple-silicon-m1-gate.md](apple-silicon-m1-gate.md); multi-version matrix OPEN | Verified exact mount allowlist and undeclared-canary absence on Apple M1 with clean VM deletion. | Multi-version negative mount tests remain required in Milestone 5. |
| Two Lima VMs share a guest gateway port while using distinct Machine forwards | DEFERRED to [Milestone 5 Darwin/arm64 hardware gate](../../ROADMAP.md#milestone-5-releasable-v01-candidate) | Not yet executed with concurrent multi-VM gateways. | Mandatory concurrent forwarding collision and leak check on real Darwin/arm64 hardware. |

The executable harness itself and its refusal-first ownership model are documented in [spike harness](../features/spike-harness.md). The complete Milestone 0 decision is recorded in [ADR 0001](../decisions/0001-milestone-0-substrate-gate.md).

## Failures that changed the design

The probes falsified or narrowed several assumptions rather than merely confirming the happy path:

- Container Git initially wrote root-owned objects. The accepted fixture maps the Machine UID/GID and cleanup now preflights complete ownership and directory permissions.
- Neither tested Git version advertised relative-worktree support. Relative pointers are not a fallback; exact-path viability remains conditional on the final VirtioFS probe.
- Compose policy depends on normalization of `!reset []`. The implementation must version-check this capability before mutation rather than assuming arbitrary Compose compatibility.
- Routing must preserve the original authority, including its port, while normalizing only for route selection. `.localhost` does not override an explicit proxy by itself, so Machine preflight must verify bypass behavior.
- Docker network absence diagnostics are not stable prose, and `git worktree remove` leaves the branch. Recovery observes typed resource state and surviving refs instead of matching one message or recreating a branch blindly.

## Accepted decisions and final-gate obligations

Accepted for Linux product implementation, within this exact matrix:

1. Continue with Box-private bare Git stores, locked linked worktrees, and stock Git at invariant paths, while treating VirtioFS path transport as unproven.
2. Continue with repository-owned Compose includes and an Agent Box override, with pinned/capability-checked Compose semantics and pre-apply port suppression.
3. Continue with exact-authority `.abox.localhost` routing on one unprivileged IPv4 loopback listener, with proxy/resolver preflight and no IPv6 claim.
4. Build reconciliation around independent observation, stable operation IDs, exact ownership proofs, idempotent adoption, and refusal before destructive ambiguity.

All remaining Darwin release obligations converge on the [Milestone 5 Darwin/arm64 hardware gate](../../ROADMAP.md#milestone-5-releasable-v01-candidate): real Apple Silicon hardware across minimum and current supported macOS; Lima with `vz`; pinned Lima and guest image; VirtioFS coherency, modes, rename/delete, and watcher behavior; exact mount allowlist and undeclared-canary denial; two-VM forwarding collision test; unchanged Git/Compose/routing fixtures; kill/recovery at Lima, mount, and forwarding boundaries; and leak-free ownership-verified cleanup. Cross-compilation is artifact evidence only and cannot satisfy any of these obligations.
