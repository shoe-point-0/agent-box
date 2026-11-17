<!-- sprout-task
{"schema_version":1,"id":"m1-filesystem-verifier","status":"implemented","created_at":"2026-08-29T06:48:36.123097622Z","implemented_at":"2026-08-30T02:51:15.293256001Z"}
-->

# Mounted filesystem verification

Status: implemented. Implementation record for `m1-filesystem-verifier`.

Completed for the Linux engineering lane. Machine path identity protects Lima
and Compose mutation; fresh live inspection enforces exact mount sets and an
undeclared canary; and the nonce-scoped bidirectional bytes, modes, rename,
delete, and native watcher lifecycle runs behind a fixed-command immutable-
container adapter. The READY gate composes those checks, performs one final
complete path-identity revalidation, and cannot call its atomic READY publisher
after any checked failure or substitution.

The unchanged probe and cleanup path passed through a real Linux Compose
container. `limactl` remains unavailable on this Machine, so no Lima, VZ, or
VirtioFS runtime behavior is claimed. That platform-specific execution remains
with the explicitly deferred real Darwin/arm64 hardware gate.

## Scope and dependencies

- Milestone 1. Dependencies: m1-lima-adapter, m1-guest-toolchain. Source: THE_BEGINNING.md Sections 11.4, 11.5, and Issue 7.

## Behavior

- Probe Machine-to-workspace and workspace-to-Machine bytes, modes, atomic rename, deletion, and watcher delivery under .abox-probe.
- Verify every declared canonical path and prove undeclared canaries are absent.
- Record stable allowed-root identity, revalidate every ancestor immediately before Lima and Compose apply, reject symlink or inode drift, and compare the Plan with live Lima inspection, guest mountinfo, Docker inspect, and container mountinfo as an exact set.
- Clean probe artifacts and configure an Agent Box-managed global Git exclude.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- A Box cannot become ready when required visibility or ownership checks fail.
- Probe failures retain bounded, non-secret diagnostics.
- Cleanup never changes repository .gitignore or unrelated global Git configuration.
- Concurrent symlink and rename substitution of worktrees, Git stores, generated files, and bind sources is refused before READY.

## Implementation details

- Extend the existing filesystem ownership boundary with an immutable path-set
  record. Capture every component from the filesystem root through each exact
  Plan path with `lstat`, then revalidate the complete device/inode chain and
  reject symlinks, replacement ancestors, missing components, duplicates, or
  any difference between the expected and recorded path sets. Lima and Compose
  mutation preflights will consume this evidence rather than recanonicalizing a
  mutable path and trusting its new target.
- Add an `internal/filesystem` verifier over the typed Lima invocation boundary.
  Compare the Plan's complete mount set with fresh Lima configuration, guest
  mountinfo, Docker inspect bind mounts, and workspace-container mountinfo.
  Treat presence and absence as a set equality contract, including explicit
  undeclared canaries, rather than checking only that required mounts exist.
- Run fixed, argv-only probe operations beneath `.abox-probe/<nonce>` for exact
  bytes and modes, rename and delete visibility in both directions, and native
  workspace-container watcher delivery. Keep returned diagnostics structured,
  bounded, and free of workload content; remove the nonce on every exit path.
- Materialize an owner-only Agent Box excludes file containing `.abox-probe/`
  and pass it explicitly through the controlled Git profile. Do not edit a
  repository `.gitignore` or inherit/mutate the user's global Git configuration.
- Exercise orchestration and substitution races with deterministic boundary
  fixtures, then run the unchanged probe against a real container. A real Lima
  VM result is required only where a supported `limactl` is available; Linux
  fixture/container evidence must not be described as VZ or VirtioFS evidence.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run the unchanged filesystem probe through a real Linux container; retain
  real Lima/VZ/VirtioFS execution as the explicit Darwin hardware gate

## Validation evidence

- Path-identity slice: `go test -race -count=10 ./internal/ownership/...`
  passed. The suite records exact sorted sets and every ancestor from `/` to
  each leaf, then proves exact-set revalidation and refusal after duplicate/set
  drift, parent rename-and-replacement, and symlink substitution.
- Compose preflight integration: `go test -race -count=3
  ./internal/ownership/... ./internal/compose/...` passed, including the real
  Docker lifecycle test. Every Compose operation now revalidates the captured
  Plan mounts, repository Compose inputs, and all generated Compose evidence
  before the first guest invocation; a replaced generated file was refused
  with zero invoker calls.
- `go vet ./internal/ownership/... ./internal/compose/...` and `git diff
  --check` passed for this slice.
- Lima mutation preflight: `go test -race -count=10 ./internal/lima/...`
  passed. The adapter now requires exact ancestor identity evidence for the
  guest image and every declared mount before create/start reconciliation, and
  separate evidence for the generated Lima config before create. Deterministic
  substitution tests replace a generated config and rename/replace a mount
  ancestor, then prove zero create/start invocations.
- Cross-boundary regression: `go test -race -count=3
  ./internal/ownership/... ./internal/lima/... ./internal/compose/...` passed,
  including the real Docker lifecycle. `make check` passed, including full
  tests, race tests, vet, lint with `0 issues`, and builds. `sprout check --all`
  and `git diff --check` passed.
- Runtime inspection still reports `limactl: command not found`. No VM was
  created and this evidence does not establish Lima, VZ, or VirtioFS behavior.
- Exact live mount-set contract: `go test -race -count=10
  ./internal/filesystem/... ./internal/lima/... ./internal/compose/...` passed,
  including the real Docker lifecycle suite. The typed verifier compares the
  Plan independently with Lima's inspected config, guest mountinfo, Docker
  inspect binds, and mountinfo keyed by immutable container ID. It rejects
  missing, extra, remapped, duplicate, nested, or mode-drifted mounts at each
  layer; bounds mountinfo to 1 MiB/8192 records; decodes kernel path escapes;
  and returns stable diagnostics without observed paths or content. Lima now
  exposes a defensive mount copy from its live inspection rather than its
  generated input.
- `go vet ./internal/filesystem/... ./internal/lima/... ./internal/compose/...`,
  `make lint` (`0 issues`), and `git diff --check` passed for this slice.
- Live collection boundary: `go test -race -count=10
  ./internal/filesystem/... ./internal/lima/... ./internal/compose/...` passed,
  including the existing real-Docker Compose lifecycle. The collector now
  obtains a defensive mount set from fresh Lima inspection, re-observes Docker
  through the Compose runtime, reads guest mountinfo through the typed Lima
  invocation, and reads each container's mountinfo with fixed argv addressed
  by its immutable Docker ID. Deterministic tests prove sorted immutable-ID
  calls, duplicate-ID refusal before container exec, bounded capture, stable
  cancellation, and no command-output or underlying-error disclosure.
- `go vet ./internal/filesystem/... ./internal/lima/... ./internal/compose/...`,
  `make lint` (`0 issues`), and `git diff --check` passed for the live collector
  slice. This does not yet claim a complete filesystem probe or real Lima
  execution.
- Bidirectional probe orchestration: `go test -race -count=10
  ./internal/filesystem/...` passed. For every sorted Plan worktree, the verifier
  now creates an exclusive 128-bit nonce beneath `.abox-probe`, requires the
  immutable workspace-container ID and a writable workspace-service mount, and
  checks exact bytes and modes plus rename and deletion visibility in both
  directions. Tests exercise all workspace-operation failures, mismatched
  bytes, cancellation, duplicate/ambiguous projections, symlink refusal,
  preservation of a pre-existing concurrent-probe parent, bounded diagnostics,
  and nonce cleanup on every exercised exit path.
- `make check` passed after the probe-orchestration slice, including full unit
  and race tests, vet, lint with `0 issues`, builds, and repository script tests.
  The current workspace boundary fixture shares the real test filesystem; it is
  not container or VirtioFS evidence. The fixed-command workspace adapter and
  unchanged real-container execution remain required before completion.
- Fixed-command workspace-container boundary: `go test -race -count=10
  ./internal/filesystem/...` passed, including ten executions of the unchanged
  bidirectional probe lifecycle through a real Compose-managed `alpine:3.20`
  workspace container. The fixture runs as the Machine UID/GID and bind-mounts
  the worktree at its identical absolute path; exact bytes, modes, rename, and
  deletion visibility passed in both directions and teardown left no container
  or `.abox-probe` residue.
- The adapter addresses only the immutable Docker ID, terminates Docker option
  parsing explicitly, accepts only nonce-owned probe paths and generated probe
  payloads/modes, and invokes constant `/bin/sh` scripts with all paths and data
  passed as positional argv. Unit tests prove fixed argv, malformed/truncated
  output refusal, cancellation classification, bounded redacted failures, and
  zero command calls for invalid or cross-nonce requests. The mountinfo
  collector now also terminates Docker option parsing before immutable IDs.
- `make check` passed for this slice: full tests and race tests, `go vet`,
  dependency checks, lint with `0 issues`, builds, smoke, and automation tests.
  `sprout check --all` and `git diff --check` passed. This is real Linux
  Docker/Compose bind-mount evidence, not Lima, VZ, or VirtioFS evidence.
- Native workspace watcher delivery: `go test -race -count=10
  ./internal/filesystem/...` passed ten unchanged real Compose-container runs.
  Each probe starts BusyBox `inotifyd` inside the immutable workspace-container
  ID, proves watch installation with an in-container handshake, then requires a
  native create/modify/close-write event for the exact Machine-created probe
  file within one second. The fixed script receives all paths through positional
  argv, owns its FIFO and child processes, and removes readiness/handshake state
  on every return path. The integration test verifies no probe artifacts,
  watcher processes, or Compose containers remain.
- Watcher setup, delivery, malformed request, and command failures retain the
  existing bounded redacted error contract. `make check` passed after
  this slice, including full tests and race tests, `go vet`, dependency checks,
  lint with `0 issues`, builds, smoke, and automation tests. This establishes
  native Linux inotify delivery through the tested Docker bind mount only; it
  does not establish VirtioFS or Darwin watcher behavior.
- Undeclared mount canary: `go test -race -count=10
  ./internal/filesystem/...` passed ten real Compose-container runs. The live
  verifier now creates an empty Machine-owned random directory outside every
  declared mount before collecting the four live mount views. Only after exact
  Plan equality passes does it run fixed argv through the guest and every
  sorted immutable container ID to require that exact path to be absent.
- Unit tests prove guest and container visibility failures, output rejection,
  cancellation classification, redacted diagnostics, deterministic ordering,
  refusal when the canary root is inside a declared mount, and cleanup after
  each exercised outcome. The real `alpine:3.20` Compose fixture proved the
  undeclared Machine sibling is absent from the workspace container and left
  no canary, probe, process, or container residue.
- `make check`, `sprout check --all`, and `git diff --check` passed after this
  slice. `make check` included full tests and race tests, vet, dependency
  checks, lint with `0 issues`, builds, smoke, and automation tests. Guest
  absence remains fixture evidence because `limactl` is unavailable; this is
  not Lima, VZ, or VirtioFS evidence.
- Managed Git exclude: `go test -race -count=10 ./internal/machinegit/...
  ./internal/paths/... ./internal/repository/... ./internal/gitstore/...
  ./internal/worktree/... ./internal/publication/... ./internal/boxplan/...
  ./cmd/aboxd/...` passed. Daemon startup now atomically materializes the exact
  `.abox-probe/` policy at the Agent Box application root, requires an
  owner-only real parent and owner-only regular policy file, and passes its
  canonical path explicitly as command-scoped `core.excludesFile` while
  continuing to disable user-global and system Git configuration. Thirty-two
  concurrent preparers produced one complete policy file and no temporary
  residue under the race detector.
- The real-Git test proved `.abox-probe/<nonce>` is ignored while an unrelated
  untracked file remains visible, and byte-compared the repository `.gitignore`
  and an ambient user-global `.gitconfig` before and after the operation.
  Foreign pre-existing managed-exclude content is refused without mutation.
- `make check` passed after correcting two initially reported unchecked close
  errors; the final run included lint with `0 issues`, full unit and race tests,
  vet, dependency checks, builds, smoke, and automation tests. `go vet` and
  `git diff --check` also passed for the focused package set.
- READY publication gate: `go test -race -count=10
  ./internal/filesystem/...` passed. `ReadinessGate.VerifyAndPublish` now joins
  fresh four-layer mount inspection and undeclared-canary checks with the
  unchanged bidirectional workspace-container probe. It invokes the caller's
  atomic READY publisher only after both complete successfully.
- The gate revalidates the complete Lima image/mount and Compose
  mount/include/generated-file path sets immediately before READY publication.
  Deterministic rename-and-replacement tests substituted a worktree, private
  Git store, repository Compose include, and generated Compose input after both
  verifier stages; every substitution was refused and the READY publisher had
  zero calls. Live-verifier, probe, cancellation, and nil-publisher failures
  likewise produced zero READY calls.
- Final `make check` passed: full tests and race tests, vet, dependency checks,
  lint with `0 issues`, builds, smoke, and automation tests. `go vet
  ./internal/filesystem/...` and `git diff --check` passed. The existing real
  `alpine:3.20` Compose integration continued to exercise exact bytes, modes,
  rename, deletion, native inotify delivery, undeclared-canary absence, and
  cleanup through an immutable container ID.
