# Apple Silicon Milestone 1 hardware gate

Status: **GREEN** for the Milestone 1 single-repository vertical slice on the tested Apple M1 stack (2026-11-22). Full multi-version macOS release support remains gated on [`m5-darwin-arm64-hardware-gate`](../../.sprout/tasks/m5-darwin-arm64-hardware-gate.md).

This report summarizes the real-hardware verification of [`abox up`](../features/abox-up.md) and refusal-first [`abox rm`](../features/abox-rm.md) on Darwin/arm64 with Lima VZ and VirtioFS. The sanitized machine-readable evidence bundle is stored in [apple-silicon-m1-evidence.json](apple-silicon-m1-evidence.json).

## Tested platform matrix

| Component | Tested value |
| --- | --- |
| Machine hardware | Apple M1 (`arm64`) |
| Host OS | macOS 27.0.1 (build `26A434`) |
| Lima | `2.0.0`, `vmType: vz`, `mountType: virtiofs`, `mountInotify: true` |
| Guest kernel | Ubuntu Noble Linux `6.8.0-147-generic` (`aarch64`) |
| Guest image digest | `sha256:a1e5ce9fb88624feaf6dc08268f2999afe58075df62ed248028d62818d17424c` |
| Guest Docker | `29.1.3` (rootful, `overlay2`, cgroup v2) |
| Guest Docker Compose | `2.40.3+ds1-0ubuntu1~24.04.1` |
| Fixture | One local Git repository with three HTTP services (`alpha`, `beta`, `gamma`) and Compose healthchecks |

## Acceptance results

Every Milestone 1 acceptance run used the built production `abox` and `aboxd` binaries against fresh, isolated state directories with a short explicit `--state-db` path.

| Scenario | Observed result | Key invariants verified |
| --- | --- | --- |
| Cold `abox up --json demo` from zero | Exit `0` in `40.79 s`; published generation `1` (`READY`) | Three healthy services (`alpha`, `beta`, `gamma`) with HTTP sibling reachability (`/` -> `200`, `?peer=beta` -> `200`, `?peer=gamma` -> `200`); exact mount modes (`services.git` read-only, `workspace/services` read-write); incidental TCP/UDP forwarding disabled; zero partial recovery rows; zero `.abox-probe`, `/tmp/abox-watch-*`, `/tmp/abox-inputs.*`, or leaked watcher processes/zombies |
| Warm `abox up --json demo` | Exit `0` in `15.36 s`; measured no-op | Identical worktree device/inode/branch/HEAD, private Git store device/inode, all six generated file inodes (`plan.json`, `context.json`, `lima.yaml`, `compose.root.yaml`, `compose.abox.yaml`, `compose.resolved.redacted.json`), VM name and `hostAgentPID`, all three container IDs and `Created` timestamps, network ID, zero volumes, and unchanged generation `1` |
| Detached CLI (`SIGKILL` during cold reconciliation) | CLI killed with `-9` after `up.started`; daemon completed the exact operation to `succeeded` / `READY` | Single operation record, zero duplicate VMs/containers/networks/worktrees; follow-up warm `abox up` preserved every resource identity |
| Refusal-first `abox rm --json demo` (without `--yes`) | Exit `7` (`ABOX_CLEANUP_REFUSED`) | Structured preview digest emitted; every worktree, Git, file, VM, container, and network identity remained untouched |
| Ownership-aware `abox rm --json --yes demo` | Exit `0` in `24.52 s` | Compose stack torn down, running VM gracefully stopped and deleted (`limactl list` empty), worktree, private Git store, and transient generated files removed, and only retained `plan.json` remained in the Box root |

## VirtioFS visibility and bridged watcher measurements

Real VZ execution falsified the original immediate-visibility and unassisted native-inotify assumptions in [ADR 0005](../decisions/0005-invariant-paths.md). The revised contract is recorded in [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md):

- **Bounded read-only visibility retries**: VirtioFS name and attribute caches can retain a deleted or renamed entry for roughly 1–2 seconds, causing `stat` to succeed while `open`/`read` returns `ENOENT`. The workspace probe bounds reads with `head -c 257` and explicit `\nOK\n`/`\nERR\n` framing so stale-stat read races produce an invalid existing observation (neither absent nor valid) and retry within the 2-second deadline without repeating mutations or masking unreadable or NUL-corrupted files.
- **Bridged `IN_ATTRIB` event delivery**: With `mountInotify: true`, Lima 2.0.0 synthesizes Linux `IN_ATTRIB` (`e` in BusyBox `inotifyd`) for host creates and modifies without altering file contents or permissions. Across five trials after the in-container handshake (`71.9–83.8 ms`), bridged `e` delivery completed in `11.6–14.3 ms` (deadline `1000 ms`) with mode `0600` and exact payload bytes intact.
- **Remote watcher cancellation cleanup**: OpenSSH `ControlMaster` retains stdout/stderr pipe writers until the remote `docker exec` exits. Two-phase in-container watchdogs (`3 s` setup, `1.5 s` delivery) with explicit `wait` child reaping ensure canceled watches exit and close pipes within `510.1 ms` (below the `2000 ms` process-group shutdown timeout) with zero leaked processes, zombies, or FIFOs.

## Hardware-discovered fixes folded into Milestone 1

In addition to the filesystem visibility and watcher changes in [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md), live Apple Silicon execution uncovered and fixed four integration defects now covered by regression tests:

1. **Zero-volume READY retirement**: [`cloneRemovalSnapshot`](../../internal/state/removal_snapshot.go) preserved `nil` slices via `slices.Clone` instead of allocating a non-nil empty `Volumes` slice that mismatched partial Compose authority during `PublishReady`.
2. **Health-aware cold Compose apply**: [`compose.Runtime.Apply`](../../internal/compose/runtime.go) passes `--wait` so cold starts wait for declared Compose healthchecks before entering readiness verification instead of observing `Health: "starting"`.
3. **Stop-before-delete for running VMs**: [`lima.Adapter.Delete`](../../internal/lima/lima.go) gracefully stops and re-proves ownership of a `Running` Lima instance before invoking `limactl delete`.
4. **Private guest transport and administrative execution**: Administrative guest operations run via noninteractive `/usr/bin/sudo -n --`, and generated Compose files travel over stdin to mode-`0700` guest temporary directories cleaned on exit and cancellation rather than widening the VM mount allowlist.

## Known limits and remaining gates

- **Multi-version macOS release matrix**: This gate establishes single-repository Milestone 1 behavior on one Apple M1 macOS version. Minimum and current supported macOS versions, two-VM forwarding collision checks, and full fault-injection recovery remain open in [`m5-darwin-arm64-hardware-gate`](../../.sprout/tasks/m5-darwin-arm64-hardware-gate.md).
- **Default macOS socket path length**: Validation used a short explicit `--state-db` path because Lima control sockets under `$HOME/Library/Application Support/Agent Box` can exceed Darwin's 104-byte Unix socket limit. See [getting started](../getting-started.md#daemon-startup-and-paths).
- **Event-bridge limits**: Lima's experimental `mountInotify` does not forward host deletion events and synthesizes `IN_ATTRIB`. Projects whose watchers require deletion events or ignore `IN_ATTRIB` must configure polling.
- **Post-removal Box state in `abox list`**: `abox rm --yes` removes all external and transient Box resources and retains `plan.json`, while SQLite `boxes.recorded_state` still renders `ready` in `abox list` afterward; transitioning the Box state row on completed removal remains a separate follow-up.
