<!-- sprout-task
{"schema_version":1,"id":"m1-guest-toolchain","status":"implemented","created_at":"2026-08-29T06:48:36.107508706Z","implemented_at":"2026-08-30T00:49:04.893818046Z"}
-->

# Guest toolchain provisioning and verification

Status: implemented. Implementation record for `m1-guest-toolchain`.

Implemented `internal/guesttoolchain` as the deterministic bridge between the
verified artifact contract and the typed Lima guest invocation boundary. It
accepts only explicit absolute preinstalled paths, reopens every opaque artifact
before guest access, verifies matching guest digests and exact tool versions,
starts only `docker.service`, and returns a typed observation only after pinned
Docker/Compose, cgroup v2, `overlay2`, bridge networking, rootful operation, and
a root-owned non-world-accessible socket are observed.

Provisioning has no acquisition, package-manager, shell interpolation, or
container socket-mount behavior. A second call deliberately repeats the same
verification and idempotent service operation rather than trusting cached
state. Production guest execution remains owned by `lima.Adapter`; the process
boundary allowlist now names `guesttoolchain` as an audited typed adapter.

## Scope and dependencies

- Milestone 1. Dependencies: m1-lima-adapter, m1-toolchain-contract. Source: THE_BEGINNING.md Sections 6.2, 12, 16.5, and 22.3.

## Behavior

- Consume an already verified preinstalled or test-fixture artifact set for the guest image, Docker Engine, Compose, and required utilities; do not download implicit defaults.
- Provision rootful Docker inside the VM without exposing its socket to workspace containers.
- Verify runtime versions, cgroup/storage/network prerequisites, daemon readiness, and tool integrity.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- A fresh runtime host reaches a verified Docker/Compose-ready state.
- Wrong versions or digest mismatches fail before project code executes.
- Provisioning is idempotent and diagnostics identify the failing layer.

## Implementation details

- Add an `internal/guesttoolchain` orchestration boundary over the typed Lima
  guest invocation contract. It consumes only opaque artifacts already verified
  by `internal/toolchain`; every artifact is reopened and digest-checked before
  the first guest command, so changed or mismatched inputs cannot reach project
  execution or guest mutation.
- Map each verified Docker, Compose, and named utility identity to one explicit
  absolute preinstalled guest path. Verify every guest file's SHA-256 before
  checking exact Docker/Compose versions or starting the daemon. Acquisition,
  download, and implicit package-manager defaults remain out of scope.
- Start rootful Docker through an explicit service-manager argv, then observe
  machine-readable `docker info` plus socket ownership/mode. Require the pinned
  server version, cgroup v2, `overlay2`, bridge networking, a root-owned socket
  with no world access, and no rootless security option. The provisioner never
  mounts or passes the Docker socket to a container.
- Classify failures by artifact, version, service, runtime, and cancellation
  layers. Re-run the same deterministic verify/start/observe sequence for warm
  provisioning, making idempotence a tested contract rather than a state flag.
- Exercise the complete fresh/warm path through an injected guest fixture on
  Linux. Real Lima VZ execution remains deferred to the Darwin hardware gate;
  this task will not infer that support from fixture execution or cross-builds.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run fresh and warm guest-toolchain provisioning on supported CI

## Validation evidence

- `go test -race -count=10 ./internal/guesttoolchain/...` passed ten
  iterations of the complete injected-Lima guest contract. Each iteration ran
  both fresh and warm provisioning over preverified fixture artifacts; both
  passes rechecked five Machine artifacts, five guest digests, exact Docker and
  Compose versions, idempotent `systemctl enable --now docker.service`, Docker
  CLI Compose-plugin loading, daemon info, and socket ownership/mode.
- The failure fixtures proved a changed Machine artifact and a guest digest
  mismatch stop before any Docker version or service command. A wrong Docker
  version stops before daemon start. Dedicated cases classified cgroup v1,
  wrong storage, missing bridge networking, rootless mode, world-accessible
  socket, malformed input, and cancellation at their artifact, version,
  runtime, or cancellation layer. Package coverage was `78.5%`.
- `make check` passed on Linux after correcting staticcheck capitalization
  findings: `golangci-lint` reported `0 issues`; dependency verification, all
  tests and race tests, vet, builds, smoke checks, shell guards, and Python
  automation tests passed.
- `go test -race -count=10 ./...` passed the full repository ten times.
  `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only.
- Fresh inspection reported `limactl: unavailable`; no VM or shared runtime was
  mutated. The Linux evidence establishes the injected guest-control contract,
  not real Lima VZ, VirtioFS, guest-image, or Darwin runtime support. Those
  unchanged production fixtures remain required at the Darwin hardware gate.
