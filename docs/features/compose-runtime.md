<!-- sprout-task
{"schema_version":1,"id":"m1-compose-adapter","status":"implemented","created_at":"2026-08-29T06:48:36.151865633Z","implemented_at":"2026-08-30T01:14:02.342658598Z"}
-->

# Compose lifecycle, exec, logs, and observation

Status: implemented.

The `compose.Runtime` adapter in `internal/compose` provides the policy-gated
guest lifecycle boundary for observing, pulling, building, applying, stopping,
and removing Compose projects as well as running noninteractive `exec` and
cancelable `logs` streams. Every mutation or streaming invocation re-normalizes
the exact generated Compose inputs, re-runs policy checks, and compares the
canonical bytes and digest against compiler evidence before acting.

Observation reads machine-readable Docker JSON directly—never human-formatted
Compose tables—covering project containers, networks, volumes, referenced
images, ownership labels, health, mounts, attachments, and published ports.
Real Lima VM execution on Apple M1 was subsequently wired in
[`abox up`](abox-up.md), which also added `--wait` to
`compose up` so cold applies wait for declared healthchecks and transported
generated Compose inputs over stdin to bounded private guest temporary files.

## Scope and dependencies

- Milestone 1 Compose runtime adapter. Depends on [process runner](process-runner.md), [Compose compiler](compose-compiler.md), [Compose security policy](compose-security-policy.md), [guest toolchain](guest-toolchain.md), and [ownership proofs](ownership-proofs.md). Source: `THE_BEGINNING.md` Sections 9.3, 12.2, and Issue 8.

## Behavior

- Binds every invocation to the immutable Plan, positive generation, exact generated root and override inputs, explicit Compose project name, pinned Docker path, and Lima guest invocation boundary—never accepting caller-selected Compose identity.
- Re-normalizes the generated inputs and validates normalized security policy and canonical digest parity before executing `pull`, `build`, `apply`, `stop`, `remove`, `exec`, or `logs`.
- Applies projects idempotently via `compose up --detach --wait --remove-orphans` (with `--wait` added in [`abox up`](abox-up.md)) followed by fresh Docker JSON observation.
- Observes project containers, networks, volumes, and referenced images deterministically via Docker JSON inspection; requires fresh `ownership` ID-and-label proofs over the complete live resource set before destructive removal, while intentionally retaining potentially shared images.
- Preserves exact process exit codes on noninteractive `exec` and propagates cancellation with bounded diagnostics on streaming `logs`.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Destructive removal refuses any resource set whose live IDs or five ownership labels do not match recorded ownership proofs.

## Verified acceptance criteria

- A second `Apply` does not duplicate containers, networks, or volumes.
- Every observed resource is attributable by opaque Docker ID and ownership labels.
- `Exec` returns exact exit status and `Logs` stops cleanly on context cancellation.

## Implementation details

- `compose.Runtime` lives beside the read-only Compose compiler in `internal/compose` and consumes only a verified compiler output.
- The compiler projects all five Box ownership labels onto services, the default network, and declared named volumes; `Observe` queries Docker JSON by explicit Compose project label, inspects each container, network, volume, and referenced image, and returns a sorted deterministic snapshot.
- Streaming `Exec` and `Logs` execute through the bounded, cancellation-aware process boundary so exit status and typed cancellation propagate cleanly.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run Compose adapter contract and real-runtime integration tests

## Validation evidence

- `TestRuntimeLifecycleAgainstRealDocker` passed against real Linux Docker Engine `29.6.1` and Compose `5.3.0` on a uniquely named two-service fixture with one named volume: two consecutive applies produced identical container, network, and volume ID sets; observation verified containers, network, volume, image, health, mounts, attachments, ownership labels, and zero Machine port bindings; `exec` returned exact exit status `37`; follow `logs` streamed a live line and canceled within the `3s` deadline; a forged recorded Box label caused removal refusal without mutating live resources; and verified removal left zero project-labeled containers, networks, volumes, or images.
- `TestRuntimePullBuildAndPolicyPreflightCommandContract` verified explicit project, profile, and generated-file arguments for `pull` and `build`, and confirmed a modified normalized privilege policy stops after read-only `config` before any pull mutation.
- `make check` (`0` lint issues), `go test -race -count=10 ./...` (ten real lifecycle fixtures in `259.754s`), `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`, and `git diff --check` passed.
- Real Apple M1 Lima-guest Compose apply (`--wait`), warm no-op identity preservation, and ownership-verified removal were subsequently exercised end to end in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
