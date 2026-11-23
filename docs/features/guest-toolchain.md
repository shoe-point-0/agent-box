# Guest toolchain provisioning and verification

Status: implemented.

The `internal/guesttoolchain` package verifies preinstalled guest artifacts and
ensures a rootful Docker Engine and Compose runtime is ready inside a Lima
runtime host before any project code executes. It consumes only opaque
artifacts already verified by `internal/toolchain`, reopens and re-hashes each
Machine artifact, validates matching SHA-256 digests and exact versions inside
the guest at explicit absolute paths, starts only `docker.service`, and
confirms cgroup v2, `overlay2`, bridge networking, rootful operation, and a
root-owned non-world-accessible Docker socket.

Provisioning performs no network downloads, package-manager installs, shell
interpolation, or container socket mounts, and every warm call repeats the full
verification and idempotent service sequence rather than trusting cached state.
The package is registered in the audited process-boundary allowlist as a typed
adapter over `lima.Adapter`. Real Apple Silicon Lima VZ execution of this
provisioner was subsequently verified end to end in
[`abox up`](abox-up.md); the full multi-version Darwin
release matrix remains [open](../../ROADMAP.md#milestone-5-releasable-v01-candidate).

## Scope and dependencies

- Milestone 1 guest toolchain boundary. Depends on [Lima runtime-host adapter](lima-runtime-host.md) and [toolchain artifact contract](toolchain-artifacts.md). Source: `THE_BEGINNING.md` Sections 6.2, 12, 16.5, and 22.3.

## Behavior

- Reopens and digest-checks every preverified Machine artifact from `internal/toolchain` before issuing the first guest command; never downloads implicit defaults or runs package managers.
- Maps Docker, Compose, and required guest utility identities to explicit absolute preinstalled guest paths, verifying SHA-256 digests and exact pinned versions prior to starting `docker.service`.
- Starts rootful Docker via explicit argv (`systemctl enable --now docker.service`) without exposing the Docker socket to workspace containers.
- Observes machine-readable `docker info` and Docker socket ownership/mode, requiring exact server version, cgroup v2, `overlay2`, bridge networking, rootful operation, and a root-owned non-world-accessible socket.
- Classifies failures explicitly by layer (`artifact`, `version`, `service`, `runtime`, or `cancellation`) and repeats the full verify/start/observe sequence identically on warm runs.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Any Machine artifact tamper, guest digest mismatch, or version mismatch halts provisioning before daemon start or project execution.

## Verified acceptance criteria

- Fresh and warm runtime hosts converge to a verified Docker/Compose-ready state.
- Wrong tool versions or digest mismatches fail before any project code runs.
- Provisioning is idempotent and errors identify the exact failing layer.

## Implementation details

- `internal/guesttoolchain` orchestrates guest verification strictly through the typed Lima guest invocation interface (`lima.Adapter`), using fixed argv invocations with zero shell interpolation.
- Each invocation re-runs artifact verification, guest SHA-256 verification, version checks, idempotent `docker.service` activation, CLI Compose-plugin loading, `docker info` inspection, and socket ownership/mode checks, making idempotency a verified runtime property rather than a cached flag.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run fresh and warm guest-toolchain provisioning on supported CI

## Validation evidence

- `go test -race -count=10 ./internal/guesttoolchain/...` passed ten iterations of the complete injected-Lima fresh and warm provisioning contract (`78.5%` statement coverage), re-verifying five Machine artifacts, five guest SHA-256 digests, exact Docker and Compose versions, idempotent `systemctl enable --now docker.service`, Compose plugin loading, `docker info`, and socket ownership/mode on every pass.
- Negative fixtures verified that tampered Machine artifacts and guest digest mismatches stop before Docker version or service commands, wrong Docker versions stop before daemon start, and cgroup v1, wrong storage driver, missing bridge networking, rootless mode, world-accessible sockets, malformed output, and cancellation classify at their exact layer.
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed on Linux (where `limactl` was unavailable, so no local VM was mutated).
- Real Apple M1 Lima `2.0.0` VZ guest-toolchain verification (`Docker 29.1.3`, `Compose 2.40.3+ds1-0ubuntu1~24.04.1`) was subsequently verified end to end in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
