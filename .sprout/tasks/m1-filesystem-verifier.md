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
- Runtime inspection still reports `limactl: command not found`. No VM was
  created and this evidence does not establish Lima, VZ, or VirtioFS behavior.

## Outcome and follow-ups

In progress. The critical Machine-side TOCTOU primitive now exists and the
Compose mutation boundary consumes it. Remaining work is to consume the same
evidence at Lima mutation, implement exact live mount-set observation plus the
bidirectional/watcher probe and managed Git exclude, and execute the complete
verifier against the available real-container lane. Real Lima execution remains
unavailable on this Machine and must not be inferred from the injected boundary.

## Original request

Implement verify mounted filesystem contracts for Agent Box according to the cited specification and dependencies.
