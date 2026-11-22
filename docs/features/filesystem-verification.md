<!-- sprout-task
{"schema_version":1,"id":"m1-filesystem-verifier","status":"implemented","created_at":"2026-08-29T06:48:36.123097622Z","implemented_at":"2026-08-30T02:51:15.293256001Z"}
-->

# Mounted filesystem verification

Status: implemented.

The `internal/filesystem` package (paired with immutable ancestor path-set
records in `internal/ownership` and managed Git excludes in `internal/machinegit`)
gates Box `READY` publication on end-to-end filesystem isolation, path-identity
integrity, bidirectional visibility, and watcher delivery. Before a Box can be
marked `READY`, `ReadinessGate.VerifyAndPublish` revalidates every ancestor
component of every Plan path, proves exact mount-set equality across four live
layers (Lima inspection, guest `mountinfo`, Docker inspect, and per-container
`mountinfo`), confirms an undeclared Machine canary is absent inside the guest
and every container, runs a nonce-scoped bidirectional bytes/modes/rename/delete
and watcher probe beneath `.abox-probe/<nonce>`, and performs one final
path-identity revalidation before invoking the atomic `READY` publisher.

When filesystem verification was first implemented on Linux, `limactl` was
unavailable and VirtioFS execution remained hardware-deferred. Real Apple
Silicon (`darwin/arm64`) VirtioFS execution and bridged watcher delivery were
subsequently verified on Apple M1 in [`abox up`](abox-up.md)
and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md) under
the bounded visibility and `mountInotify` (`IN_ATTRIB`) contract documented in
[ADR 0011](../decisions/0011-bounded-filesystem-visibility.md).

## Scope and dependencies

- Milestone 1 filesystem readiness gate. Depends on [Lima runtime-host adapter](lima-runtime-host.md) and [guest toolchain](guest-toolchain.md). Source: `THE_BEGINNING.md` Sections 11.4, 11.5, and Issue 7.

## Behavior

- Records immutable path-set chains (`lstat` device/inode from `/` through every leaf) for Lima guest images, mounts, generated configs, and Compose inputs; revalidates the complete chain before Lima/Compose mutation and again immediately prior to `READY` publication, rejecting symlinks, renamed ancestors, or set drift.
- Collects and compares four independent live mount views (Lima inspection, guest `/proc/self/mountinfo`, Docker inspect binds, and per-container `mountinfo` addressed by immutable container ID) against the immutable Plan as an exact set (rejecting missing, extra, remapped, duplicate, nested, or mode-drifted mounts; bounding `mountinfo` to `1 MiB` / `8,192` records and decoding kernel path escapes).
- Creates an empty random undeclared canary directory outside every declared mount on the Machine and verifies its absence inside the guest and every sorted immutable container ID, removing the canary on every exit path.
- Runs fixed, argv-only bidirectional probes beneath `.abox-probe/<nonce>` (128-bit exclusive nonce) checking exact bytes, modes, atomic rename, deletion, and in-container watcher delivery (`inotifyd` with in-container handshake and `1s` delivery deadline; bounded `2s` read-only observation convergence and `ncwe` bridged-event acceptance added per [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md) in [`abox up`](abox-up.md)), removing the nonce and watcher resources on every exit path.
- Atomically materializes an owner-only managed Git exclude file containing `.abox-probe/` under the Agent Box application root and passes it via command-scoped `core.excludesFile` without editing repository `.gitignore` files or user-global Git configuration.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Probe and mount diagnostics remain structured, bounded, and free of workload content or observed host paths.
- Any verification failure or concurrent path substitution blocks `READY` publication with zero calls to the atomic publisher.

## Verified acceptance criteria

- A Box cannot become `READY` if path-chain, four-layer mount equality, undeclared-canary absence, bidirectional visibility, or watcher checks fail.
- Probe failures return bounded, redacted, non-secret diagnostics and clean up all `.abox-probe` and watcher artifacts.
- Managed `.abox-probe/` exclusion never modifies repository `.gitignore` or ambient user `.gitconfig`.
- Concurrent symlink or rename substitution of worktrees, private Git stores, Compose includes, or generated files is refused before mutation and before `READY` publication.

## Implementation details

- **Immutable path-set preflight (`internal/ownership`, `internal/lima`, `internal/compose`)**: Lima refuses create/start on any ancestor drift in guest images, mounts, or generated configs; Compose refuses any guest invocation on drift in mounts, repository Compose inputs, or generated artifacts.
- **Four-layer live collector and undeclared canary (`internal/filesystem`)**: Lima exposes a defensive copy of live inspected mounts; container commands terminate Docker option parsing (`--`) before addressing sorted immutable container IDs and pass all paths and probe payloads as positional argv to fixed `/bin/sh` scripts.
- **Watcher lifecycle and VirtioFS hardening (`internal/filesystem`)**: `inotifyd` runs inside the immutable workspace container with an in-container handshake prior to the Machine-side probe write; [`abox up`](abox-up.md) hardened `observeProbeScript` (`head -c 257` + `\nOK\n`/`\nERR\n` framing), `observeMachine` (`O_NOFOLLOW` + bounded reads), and `watchProbeScript` (`:ncwe` events, two-phase self-cleaning watchdogs, and `wait` child reaping) per [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md).
- **Managed Git exclude (`internal/machinegit`, `cmd/aboxd`)**: daemon startup atomically creates the owner-only `.abox-probe/` excludes policy file, refuses foreign pre-existing content without mutation, and passes it solely as command-scoped `core.excludesFile`.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the unchanged filesystem probe through a real Linux container; real Lima/VZ/VirtioFS execution subsequently verified on Apple Silicon in [`abox up`](abox-up.md)

## Validation evidence

- **Linux unit, race, and real-container integration**: `make check` (`0` lint issues), `go vet`, `sprout check --all`, `git diff --check`, and repeated `go test -race -count=10` runs passed across `internal/ownership`, `internal/lima`, `internal/compose`, `internal/filesystem`, `internal/machinegit`, and `cmd/aboxd`.
- **Real Linux Compose container probe (`alpine:3.20`)**: ten race repetitions exercised the unchanged bidirectional probe (exact bytes, modes, rename, deletion), BusyBox `inotifyd` handshake and native delivery within `1s`, undeclared Machine canary absence, and post-run zero-residue cleanup against an immutable container ID.
- **Substitution and managed-exclude verification**: deterministic rename-and-replacement tests substituting worktrees, private Git stores, repository Compose includes, and generated Compose files after verification produced zero calls to the `READY` publisher; `32` concurrent daemon-startup preparers materialized one clean `.abox-probe/` excludes file under `-race`, ignoring `.abox-probe/<nonce>` while keeping unrelated untracked files visible and leaving repository `.gitignore` and user `.gitconfig` byte-identical.
- **Apple Silicon VirtioFS and bridged watcher verification**: real Lima `2.0.0` VZ / VirtioFS bidirectional visibility, `mountInotify` bridged `IN_ATTRIB` delivery (`5/5` trials in `11.6–14.3 ms`), clean cancellation cleanup (`510.1 ms` remote pipe teardown, zero leaked processes/zombies/FIFOs), and end-to-end `READY` gating were subsequently verified on Apple M1 in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
