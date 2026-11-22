<!-- sprout-task
{"schema_version":1,"id":"m1-lima-adapter","status":"implemented","created_at":"2026-08-29T06:48:36.094129071Z","implemented_at":"2026-08-30T00:39:19.680714239Z"}
-->

# Lima runtime-host adapter

Status: implemented.

The `internal/lima` package implements the version-pinned Machine-to-VM
runtime-host boundary over Lima `2.0.0`. Every public call checks the verified
`limactl` executable version, generates or validates deterministic owner-only
VZ/VirtioFS YAML from an immutable Plan, decodes `limactl list --format=json
--all-fields` JSON Lines directly, independently verifies immutable config
digests and embedded Box ownership metadata, and re-observes after every
lifecycle side effect—including failed or canceled commands.

Real Apple Silicon (`darwin/arm64`) VZ/VirtioFS execution of `internal/lima`
was subsequently wired and verified on Apple M1 in
[`abox up`](abox-up.md), which also added explicit incidental
TCP/UDP port-forwarding suppression, `mountInotify: true` for writable mounts,
noninteractive `/usr/bin/sudo -n --` administrative guest execution, stdin
transport for bounded guest inputs, and graceful stop-plus-ownership-reproof
before `limactl delete` on running VMs. The full multi-version Darwin release
matrix remains tracked in `m5-darwin-arm64-hardware-gate`.

## Scope and dependencies

- Milestone 1 Lima runtime-host adapter. Depends on [process runner](process-runner.md), [Plan compiler](plan-compiler.md), [ownership proofs](ownership-proofs.md), and [toolchain artifact contract](toolchain-artifacts.md). Source: `THE_BEGINNING.md` Sections 5.1, 6.2, 9, and Issue 7.

## Behavior

- Generates minimal deterministic VZ/VirtioFS Lima configuration with exact same-path mounts, explicit CPU/memory/disk limits, disabled ambient SSH/proxy/containerd features, explicit incidental port-forward suppression, `mountInotify: true` for writable mounts, and Box/Plan/generation ownership metadata in `config.env`.
- Performs idempotent `Create`, `Start`, `Stop`, `Inspect`, `Invoke`, and `Delete` operations through version-checked argv-only `limactl` commands and JSON Lines inventory decoding, never parsing human lifecycle prose.
- Normalizes and SHA-256-compares the immutable observed config subset before adoption or mutation, explicitly classifying absent, stopped, running, partial/broken, immutable-drift, unsupported-version, and canceled states.
- Requires `ownership.ProveLimaInstance` over the recorded identity and independently observed embedded metadata immediately before deletion; when deleting a running VM, gracefully stops and re-proves ownership before `limactl delete` (added in [`abox up`](abox-up.md)).

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Existing VMs with mismatched immutable configuration are refused rather than edited in place.

## Verified acceptance criteria

- Lifecycle methods are idempotent against independently observed Lima state.
- Deletion fails closed unless persisted records, Plan-derived identity, and live embedded Box metadata all agree.
- Contract tests cover Lima `2.0.0` JSON Lines outputs, drift, partial creation, cancellation, and error classification.

## Implementation details

- `internal/lima` consumes only a verified Lima `2.0.0` artifact from `internal/toolchain` and runs `limactl` exclusively via `internal/process` with bounded independent `stdout`/`stderr` capture.
- Every side-effecting call re-observes via `limactl list --format=json --all-fields` after completion, failure, or cancellation so callers never treat command exit codes alone as postcondition proof.
- Guest invocation (`Invoke`) runs only against verified running instances via explicit argv (and noninteractive `/usr/bin/sudo -n --` / bounded stdin staging when required by [`abox up`](abox-up.md)).

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run Lima adapter contract tests
- [x] Run the adapter smoke path on Linux with a real supported Lima configuration where available; retain Darwin/arm64 execution for `m5-darwin-arm64-hardware-gate`

## Validation evidence

- `go test -race -count=10 ./internal/lima/...` passed (`75.3%` statement coverage) against a stateful fake `limactl` backed by a verified executable fixture, covering deterministic VZ/VirtioFS YAML, exact mount/image identities, idempotent create/start/stop/delete, guest argv, ownership refusal, config drift, tampered generated files, partial creation, cancellation with re-observation, malformed/duplicate JSON Lines, unsupported executable/instance versions, and failed postconditions.
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`, and `git diff --check` passed on Linux (where `limactl` was absent; contract fixtures matched upstream Lima `v2.0.0`).
- Live Apple Silicon (`darwin/arm64`, Apple M1) Lima `2.0.0` VZ creation, warm identity preservation, `mountInotify: true`, disabled incidental forwarding, and stop-before-delete cleanup were subsequently verified on real hardware in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
