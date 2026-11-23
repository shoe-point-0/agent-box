# Machine preflight checks

Status: implemented.

`internal/machine` implements a read-only pre-mutation gate that validates host prerequisites before any Box state, Git repository, SQLite row, Lima VM, generated file, or persistent listener can be created. It checks an explicit `(OS, architecture, accelerator)` support tuple, canonical state and runtime path identities with required free-space budgets, fixed loopback router address availability, and the complete offline verified toolchain. Successful checks return immutable verified artifacts, exact component and utility versions, and ordered diagnostic facts; failures return stable typed prerequisite or unsupported-Machine errors carrying actionable facts.

> **Later integration note:** At component close time, live collector wiring into `abox up` and `abox doctor` remained downstream. Subsequently, [`abox doctor`](abox-doctor.md) rendered these diagnostic facts for `abox doctor`, and [`abox up`](abox-up.md) wired live Darwin/arm64 preflight collection (`kern.hv_support`, disk free space, router loopback bindability, and strict `abox.installation/v1alpha1` verified installation metadata) into `aboxd` and verified it on Apple M1. When the Machine router is introduced in Milestone 3, its own listener must become explicit preflight evidence rather than being reported as an unrelated occupied port.

## Scope and dependencies

- **Milestone:** Milestone 1
- **Dependencies:** [identity and paths](identity-and-paths.md), [toolchain artifact contract](toolchain-artifacts.md)
- **Specification reference:** `THE_BEGINNING.md` operation 2 and Section 17

## Behavior

- `internal/machine` performs read-only validation of the supported `(OS, architecture, accelerator)` tuple, canonical state/runtime paths and free-space budgets, fixed loopback router availability, verified tool versions, and local artifact integrity.
- Preflight returns stable typed prerequisite or unsupported-Machine errors alongside exact observed versions and structured diagnostic facts, without downloading tools or provisioning resources.

## Constraints

- Platform support claims require real execution evidence on the claimed platform.
- Validation holds no capability to mutate Git repositories, SQLite databases, Lima VMs, generated Box files, or persistent listeners.
- Artifact lookup and hashing remain strictly offline and read-only; no download or provisioning path is triggered during preflight.

## Verified acceptance criteria

- Every preflight failure returns before Git stores/worktrees, database resource rows, VMs, ports, or generated Box files are created.
- Unsupported platforms and missing or invalid prerequisites emit actionable, doctor-compatible diagnostic facts.
- Preflight evaluation is deterministic against injected platform observations.

## Implementation details

### Pure read-only validation boundary

`internal/machine` separates platform observation from prerequisite evaluation through a context-aware `Observer` interface that supplies a complete, value-only Machine observation. Because validation operates strictly on value observations and read-only verification calls, tests run deterministically and the preflight layer has no mutation path into Git, SQLite, Lima, generated files, or persistent listeners.

### Prerequisite checks and structured diagnostic facts

Preflight evaluates four prerequisite domains:

1. **Offline toolchain verification:** Calls `toolchain.Verify` before accepting the Machine and returns immutable verified artifacts plus exact component and utility versions. Corrupt or invalid artifacts fail before the Machine observer runs.
2. **Supported platform tuple:** Verifies one explicit supported `(OS, architecture, accelerator)` tuple.
3. **Canonical path and free-space budgets:** Validates accessibility and required free bytes on each canonical state and runtime path.
4. **Loopback router availability:** Checks fixed loopback router address availability.

Each failure produces a typed error paired with value-only diagnostic facts carrying `reason`, `observed` value, `requirement`, `remediation`, and contract error code fields designed for `abox doctor` rendering.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

Close-time Linux validation (deterministic injected `Observer` and offline fixture provider):

- `go test -race -count=10 ./internal/machine/...`: Passed 10 iterations covering deterministic success, unsupported platform, inaccessible path, insufficient free space, occupied router, corrupt artifact, malformed observation, and cancellation. Every failing case left the injected mutation sentinel at `0` calls, and corrupt artifacts failed before the Machine observer ran.
- Repeated successful checks produced byte-identical JSON facts and returned exact verified fixture versions: Lima `1.2.3`, guest image `2026.08`, Docker `29.6.1`, Compose `5.3.0`, and Git utility `2.43.0`. Failure facts carried stable reason, observed value, requirement, remediation, and contract error code fields.
- `make check`: Passed on Linux (`0 issues` from pinned lint, module verification, unit and race tests, `go vet`, builds, spike smoke, shell guards, and Python automation checks).
- `go test -race -count=10 ./...`, `go vet ./internal/machine/...`, and `git diff --check`: Passed across every package.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: Passed as compilation evidence only at close time (no Darwin accelerator, filesystem, router, VZ, or VirtioFS runtime behavior was claimed at close time; live Apple M1 preflight execution is recorded in [`abox up`](abox-up.md)).
