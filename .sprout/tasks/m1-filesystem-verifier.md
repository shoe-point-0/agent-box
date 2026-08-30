<!-- sprout-task
{"schema_version":1,"id":"m1-filesystem-verifier","status":"open","created_at":"2026-08-29T06:48:36.123097622Z"}
-->

# Verify mounted filesystem contracts

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Verify mounted filesystem contracts

## Context

- Milestone 1. Dependencies: m1-lima-adapter, m1-guest-toolchain. Source: THE_BEGINNING.md Sections 11.4, 11.5, and Issue 7.

## Requirements

- Probe Machine-to-workspace and workspace-to-Machine bytes, modes, atomic rename, deletion, and watcher delivery under .abox-probe.
- Verify every declared canonical path and prove undeclared canaries are absent.
- Record stable allowed-root identity, revalidate every ancestor immediately before Lima and Compose apply, reject symlink or inode drift, and compare the Plan with live Lima inspection, guest mountinfo, Docker inspect, and container mountinfo as an exact set.
- Clean probe artifacts and configure an Agent Box-managed global Git exclude.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A Box cannot become ready when required visibility or ownership checks fail.
- [ ] Probe failures retain bounded, non-secret diagnostics.
- [ ] Cleanup never changes repository .gitignore or unrelated global Git configuration.
- [ ] Concurrent symlink and rename substitution of worktrees, Git stores, generated files, and bind sources is refused before READY.

## Technical approach

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

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run filesystem verifier integration tests through a real VM and container

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

## Outcome and follow-ups

In progress. Machine path identity protects Lima and Compose mutation, the
four-layer exact comparison collects through fresh live inspection, and the
nonce-scoped bidirectional byte/mode/rename/delete lifecycle is implemented
behind a fixed-command immutable-container adapter and has passed unchanged
through a real Linux Compose container. Remaining work is watcher and
undeclared-canary probes, the managed Git exclude, and readiness integration.
Real Lima execution remains unavailable on this Machine and must not be
inferred from fixtures or Docker evidence.

## Original request

Implement verify mounted filesystem contracts for Agent Box according to the cited specification and dependencies.
