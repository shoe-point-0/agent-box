<!-- sprout-task
{"schema_version":1,"id":"m1-lima-adapter","status":"implemented","created_at":"2026-08-29T06:48:36.094129071Z","implemented_at":"2026-08-30T00:39:19.680714239Z"}
-->

# Implement the Lima runtime-host adapter

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the Lima runtime-host adapter

## Context

- Milestone 1 Linux implementation. Dependencies: m1-process-runner, m1-plan-compiler, m1-ownership-core, m1-toolchain-contract. Exercise machine-readable Lima contracts and Linux-supported runtime paths now; real VZ behavior is a final Darwin hardware gate. Source: THE_BEGINNING.md Sections 5.1, 6.2, 9, and Issue 7.

## Requirements

- Generate minimal pinned VZ Lima configuration with explicit resources, metadata, and exact mount allowlist.
- Create, start, stop, inspect, invoke, and delete through version-checked machine-readable limactl contracts.
- Classify unsupported versions, partial creation, mismatched immutable config, and cancellation.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Lifecycle methods are idempotent against observed Lima state.
- [x] Deletion requires recorded instance identity plus embedded Box metadata.
- [x] Adapter contract tests cover expected supported Lima outputs and errors.

## Technical approach

- Add an `internal/lima` boundary pinned to the verified Lima 2.0.0 artifact.
  Every public operation first checks the executable version, invokes only
  argv-based `limactl` commands through the bounded process runner, and observes
  all instances through `list --format=json --all-fields` JSON Lines rather
  than parsing lifecycle prose or treating command success as postcondition
  evidence.
- Generate deterministic minimal VZ/virtiofs YAML from the immutable Plan and
  verified guest-image identity. Include exact same-path mounts, explicit CPU,
  memory, disk, disabled ambient SSH/proxy/containerd features, and Box/Plan/
  generation ownership facts in Lima's machine-readable `config.env`.
- Normalize the immutable observed config subset and compare its SHA-256
  identity before adoption or mutation. Model absent, stopped, running, and
  partial/broken states explicitly; re-observe after every side-effect,
  including failed or cancelled commands.
- Make create/start/stop/delete idempotent from observation. Require
  `ownership.ProveLimaInstance` over the recorded identity and independently
  observed embedded metadata immediately before deletion. Keep guest invocation
  running-only and argv-based with bounded, independently captured output.
- Prove the external contract with a stateful fake `limactl` that consumes a
  real verified executable fixture. Cover supported JSON Lines, malformed
  output, unsupported versions, immutable drift, partial create, cancellation,
  postcondition failures, idempotency, ownership refusal, and exact argv.
  Attempt a real Linux smoke path only if a supported `limactl` is installed;
  do not infer VZ behavior or install a runtime merely to satisfy this task.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run Lima adapter contract tests
- [x] Run the adapter smoke path on Linux with a real supported Lima configuration where available; retain Darwin/arm64 execution for m5-darwin-arm64-hardware-gate

## Validation evidence

- `go test -race -count=10 ./internal/lima/...`: PASS. Nine contract
  test groups cover deterministic VZ/virtiofs YAML, exact mount and image
  identities, invalid inputs, idempotent create/start/stop/delete, guest argv,
  ownership refusal, config drift, tampered generated files, partial creation,
  cancellation with re-observation, malformed/duplicate JSON Lines,
  unsupported executable and instance versions, and failed postconditions.
- `make check`: PASS after the final implementation. `golangci-lint` reported
  `0 issues`; module verification, all package tests, `go test -race ./...`,
  `go vet ./...`, builds, spike-list smoke, shell checks, and Python tests
  passed. The first run correctly exposed three local lint findings and the
  production process-boundary allowlist; those were fixed rather than waived.
- `go test -race -count=10 ./...`: PASS across all packages after the final
  change, including the adapter and existing process/reconciliation contracts.
- `go test -cover ./internal/lima/...`: PASS with 75.3% statement coverage.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation
  evidence only. It is not VZ, VirtioFS, forwarding, or Darwin runtime evidence.
- Linux runtime preflight: `command -v limactl` found no installed executable,
  so no VM or shared runtime resource was created and no real Linux smoke result
  is claimed. Contract fixtures are pinned to Lima 2.0.0 behavior verified
  against upstream tag `v2.0.0`; real VZ execution remains assigned to
  `m5-darwin-arm64-hardware-gate`.
- `git diff --check`: PASS.

## Outcome and follow-ups

Implemented `internal/lima` as the version-pinned Machine-to-runtime-host
boundary. It consumes only a verified Lima 2.0.0 artifact, generates and
revalidates owner-only deterministic configuration, decodes the upstream JSON
Lines inventory, independently verifies immutable config and embedded ownership
facts, and re-observes after every lifecycle side effect even when the client
fails or is cancelled. Delete fails closed unless the exact persisted record,
Plan-derived record, and live instance metadata agree.

The adapter intentionally does not claim Linux VZ execution or Darwin support.
The next Linux tasks can consume this contract for guest-toolchain provisioning,
mount verification, and refusal-first removal. The final Apple Silicon gate must
still run the unchanged real VZ/VirtioFS lifecycle, cancellation, forwarding,
and leak fixtures before the project claims macOS runtime support.

## Original request

Implement implement the lima runtime-host adapter for Agent Box according to the cited specification and dependencies.
