<!-- sprout-task
{"schema_version":1,"id":"m1-single-repo-up","status":"implemented","created_at":"2026-08-29T06:48:36.203491553Z","implemented_at":"2026-11-22T05:29:34.140202Z"}
-->

# Single-repository up vertical slice

Status: implemented.

The single-repository `abox up` vertical slice connects the entire Milestone 1
daemon-owned reconciliation pipeline end to end: Machine preflight, fenced Box
leases, immutable Plan compilation, monotonic partial recovery ledgers, private
Git stores and locked linked worktrees, Lima VZ runtime hosts, guest toolchain
verification, bounded stdin-transported Compose compilation and health-aware
apply, bounded bidirectional VirtioFS and bridged-watcher readiness
verification, context document materialization, atomic READY publication, and
ownership-aware removal.

This vertical slice is verified on real Apple Silicon hardware (Apple M1,
`darwin/arm64`, macOS `27.0.1` build `26A434`, Lima `2.0.0` VZ, Docker `29.1.3`,
Compose `2.40.3+ds1-0ubuntu1~24.04.1`) using the production `abox` and `aboxd`
binaries. See the [Apple Silicon M1 gate report](../architecture/apple-silicon-m1-gate.md),
sanitized CLI evidence in [apple-silicon-m1-evidence.json](../architecture/apple-silicon-m1-evidence.json),
and the bounded filesystem visibility contract in
[ADR 0011](../decisions/0011-bounded-filesystem-visibility.md).

Tracked follow-ups outside this single-repository vertical slice:
- The full multi-version Darwin/arm64 release matrix (minimum and current macOS,
  two-VM forwarding collision, and full kill/recovery matrix) remains tracked in
  `m5-darwin-arm64-hardware-gate`.
- The explicit short `--state-db` path workaround for Darwin's 104-byte Unix
  socket path limit on Lima control sockets remains open for default macOS
  Application Support paths.
- Transitioning `boxes.recorded_state` in SQLite when `abox rm --yes` completes
  (so `abox list` reports removed state rather than `ready`) remains a separate
  removal state-lifecycle follow-up.
- When the Machine router is introduced in Milestone 3, its already-owned
  listener must become explicit preflight evidence rather than being mistaken
  for an unrelated occupied port.

## Scope and dependencies

- Milestone 1 end-to-end vertical slice. Depends on [Machine preflight](machine-preflight.md), [linked Git worktrees](git-worktrees.md), [Lima runtime-host adapter](lima-runtime-host.md), [filesystem verification](filesystem-verification.md), [Compose security policy](compose-security-policy.md), [Compose runtime adapter](compose-runtime.md), [context document](context-document.md), [reconciliation engine](reconciliation-engine.md), [daemon API](daemon-api.md), and [CLI transport](cli-transport.md). Source: `THE_BEGINNING.md` Sections 9.3, 14.1, and Milestone 1.

## Behavior

- Runs Machine preflight, fenced Box lease acquisition, strict validation, private Git store and locked linked worktree materialization, immutable Plan generation, Lima VZ runtime provisioning, guest toolchain verification, Compose compilation and health-aware apply, bounded filesystem and bridged-watcher readiness, `context.json` generation, and atomic `READY` publication for one local repository.
- Re-observes every external boundary independently and resumes matching partial or `READY` resources without inferring ownership from deterministic paths or SQLite caches.
- Executes reconciliation under the daemon operation context so detaching or killing the attached CLI does not interrupt or duplicate reconciliation, returning success only after the declared workspace service and filesystem readiness gates pass.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Existing immutable VM configurations are refused rather than edited in place.
- Generated Compose inputs travel as bounded private guest temporary files over stdin rather than broadening the VM mount allowlist.

## Verified acceptance criteria

- A clean fixture reaches `READY` from zero with `abox up demo`.
- A second `abox up demo` is a measured no-op with no duplicate worktree, VM, network, volume, container, or setup effect.
- Killing the attached CLI with `SIGKILL` during reconciliation does not kill or duplicate daemon-owned reconciliation.

## Implementation details

- **Daemon-owned coordination (`internal/up`)**: `Coordinator` acquires and periodically renews one fenced Box lease around `Pipeline`, cancels mutation immediately on renewal loss, and releases only a still-proven fence through a bounded cleanup context. `Pipeline` executes preparation, generated Plan, Git, Lima, guest toolchain, Compose compilation, Compose apply/observation, filesystem readiness, context materialization, and `Store.PublishReady` in dependency order.
- **Monotonic recovery authority and atomic READY (`internal/state`)**: schema migrations v4 through v8 add atomic `Store.PublishReady` and monotonic generation-scoped partial recovery tables for generated roots, Git stores and worktrees, Lima specs/instances, and Compose specs/resources. `READY` publication atomically commits the generation, context identity, cached observations, and complete removal ownership snapshot while retiring only exact matching partial authority (`cloneRemovalSnapshot` preserves `nil` slices such as zero-volume `Compose.Volumes`).
- **Verified installation and preflight (`internal/installation`, `internal/machine`, `cmd/aboxd`)**: `aboxd` loads strict bounded `abox.installation/v1alpha1` metadata, checks Darwin/arm64 VZ support (`kern.hv_support`), free disk space, and router loopback bindability before any Box mutation, and assembles per-operation adapters with zero mutable Box identity shared across operations.
- **Bounded guest transport and runtime lifecycle (`internal/lima`, `internal/compose`)**: administrative guest commands run via noninteractive `/usr/bin/sudo -n --`; environment values and generated Compose root/override files travel over stdin to mode-`0700` temporary guest directories cleaned on exit and signal cancellation. Lima configurations explicitly disable incidental TCP/UDP forwarding and enable `mountInotify: true` for writable mounts. `compose.Runtime.Apply` passes `up --detach --wait --remove-orphans` so cold starts wait for declared Compose healthchecks. `lima.Adapter.Delete` gracefully stops and re-proves ownership of a running VM before invoking `limactl delete`.
- **Bounded filesystem visibility and bridged watcher (`internal/filesystem`)**: per [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md), `ProbeVerifier` retries read observations only within two seconds without repeating mutations. `observeProbeScript` uses `head -c 257` with `\nOK\n` / `\nERR\n` framing and guarded `stat` so VirtioFS stale-stat/read races yield invalid existing observations (`ProbeFile{Exists: true, Regular: false}`) that satisfy neither exact-file nor absent postconditions while preserving trailing newlines, NUL corruption detection, and strict output/memory bounds. `observeMachine` enforces `O_NOFOLLOW` and `maxProbeDataBytes+1` bounds. `watchProbeScript` subscribes to `ncwe` (accepting Lima `mountInotify`'s bridged `IN_ATTRIB` event alongside native create/modify/close-write) and enforces two-phase self-cleaning watchdogs (`3s` setup, `1.5s` delivery) with `wait` child reaping so canceled watches exit cleanly with zero leaked processes, zombies, or FIFOs.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the real single-repository cold, warm, detached-CLI, and removal smoke tests on Apple Silicon

## Validation evidence

- **Control-plane and adapter test suite**: `make check` passed with `golangci-lint` reporting `0 issues`, `go mod tidy -diff` and `go mod verify` clean, all unit and `-race` suites passing across every Go package, `go vet ./...` clean, `bin/abox` and `bin/aboxd` builds verified, spike smoke passing, and all 15 Python automation checks passing. Focused stress suites verified lease renewal/loss, atomic READY rollback and zero-volume partial Compose retirement, monotonic recovery across all partial authority tables, stale-stat/read races, trailing newlines, NUL corruption preservation, oversized and `chmod 0000` probe files, watchdog cleanup, guest-input cancellation cleanup, `--wait` Compose apply, and stop-before-delete Lima removal.
- **Apple Silicon bridged watcher and cancellation diagnostics (Apple M1, macOS 27.0.1 build 26A434, Lima 2.0.0 VZ, Docker 29.1.3, Compose `2.40.3+ds1-0ubuntu1~24.04.1`)**: 5/5 trials delivered Lima `mountInotify`'s bridged `IN_ATTRIB` (`e`) event after the in-container handshake in `11.6–14.3 ms` (deadline `1000 ms`) with mode `0600` and exact payload bytes intact. Live watcher cancellation after a 1-second timeout exited the local process group in `9.1 ms` and closed ControlMaster pipes via remote watchdog cleanup in `510.1 ms` (within the `2000 ms` `shutdownTimeout`) with zero leaked processes, zombies, or `/tmp/abox-watch-*` FIFOs.
- **Production CLI cold READY and warm no-op (`state7`, [apple-silicon-m1-evidence.json](../architecture/apple-silicon-m1-evidence.json))**:
  - Cold `bin/abox up --json --manifest ... demo` from zero exited `0` in `40.79 s`, publishing generation `1` (`READY`), three healthy services (`alpha`, `beta`, `gamma`) with verified sibling HTTP reachability (`/` -> 200, `?peer=beta` -> 200, `?peer=gamma` -> 200), exact mount modes (`services.git` RO, `workspace/services` RW), disabled incidental port forwarding, `mountInotify: true`, zero remaining partial authority rows, and zero `.abox-probe`, `/tmp/abox-watch-*`, `/tmp/abox-inputs.*`, or leaked watcher processes/zombies.
  - Second warm `bin/abox up --json --manifest ... demo` exited `0` in `15.36 s` with 100% identical worktree dev/inode/HEAD, Git store dev/inode, all six generated file inodes, VM name and `hostAgentPID`, all three container IDs and `Created` timestamps, network ID, zero volumes, and unchanged generation `1`.
  - Refusal-first `bin/abox rm --json` without `--yes` exited `7` (`ABOX_CLEANUP_REFUSED`) and left every resource identity untouched; `bin/abox rm --json --yes` exited `0` in `24.52 s`, leaving empty `limactl list` and only retained `plan.json` in the Box root.
- **Production CLI detached reconciliation (`state8`, [apple-silicon-m1-evidence.json](../architecture/apple-silicon-m1-evidence.json))**:
  - Launched fresh cold `bin/abox up --json --manifest ... demo`, observed `up.started` (`op_7E4A86B8A2E3F072ABFE0C06571A2FE1`), and sent `SIGKILL` (`-9`) to the attached CLI process mid-reconciliation while `aboxd` stayed alive with that exact operation still `running`.
  - The daemon completed the same single operation to `succeeded` / `READY` with zero duplicate resources; a follow-up warm `abox up` preserved 100% of worktree, Git, generated-file, VM, container, and network identities, and `bin/abox rm --json --yes` cleanly removed all external/transient resources.
- `sprout check --all` passed.
