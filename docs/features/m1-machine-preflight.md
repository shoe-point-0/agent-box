<!-- sprout-task
{"schema_version":1,"id":"m1-machine-preflight","status":"implemented","created_at":"2026-08-29T06:55:50.40048004Z","implemented_at":"2026-08-30T03:03:26.448450428Z"}
-->

# Machine preflight checks

Status: implemented. Implementation record for `m1-machine-preflight`.

Implemented `internal/machine` as a read-only pre-mutation gate. It validates an
explicit OS/architecture/accelerator support tuple, canonical state/runtime
path identities and free-space budgets, fixed loopback router availability,
and the complete offline verified toolchain. Success returns immutable verified
artifacts, exact versions, and ordered diagnostic facts; failures return stable
typed prerequisite or unsupported-Machine errors with actionable facts.

The package deliberately cannot mutate Git, SQLite, Lima, generated Box files,
or persistent listeners. Platform adapters supply observations through the
context-aware `Observer`; wiring a live collector into the later `abox up`
operation remains with the operation-DAG task rather than weakening this
deterministic boundary or claiming unexecuted Darwin behavior.

## Scope and dependencies

- Milestone 1. Dependencies: m1-identity-paths, m1-toolchain-contract. Source: THE_BEGINNING.md operation 2 and Section 17.

## Behavior

- Read-only validate supported OS/architecture/accelerator, free space, state/runtime paths, router availability, verified tool versions, and artifact integrity.
- Return stable prerequisite errors and exact observed versions without downloading or provisioning.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Every failure occurs before Git, database resource rows, VM, ports, or generated Box files are created.
- Unsupported and missing prerequisites produce actionable doctor-compatible facts.
- The check is deterministic against injected platform observations.

## Implementation details

- Add an `internal/machine` preflight boundary whose input is a complete,
  value-only Machine observation. Keep collection behind a context-aware
  `Observer` so platform, filesystem, and listener facts are deterministic in
  tests and validation itself has no Git, database, VM, generated-file, or
  persistent port mutation capability.
- Validate one explicit supported `(OS, architecture, accelerator)` tuple,
  required free bytes on each canonical state/runtime path, and router address
  availability. Return stable typed reasons plus a value-only diagnostic fact
  containing the observed value, requirement, and remediation suitable for a
  future `abox doctor` renderer.
- Invoke the existing offline `toolchain.Verify` contract before accepting the
  Machine and return the exact verified component/utility versions in the
  result. Artifact lookup and hashing remain read-only; no download or
  provisioning path is introduced.
- Prove ordering with an injected observer/provider and a mutation sentinel:
  every unsupported, missing, insufficient, occupied, malformed, canceled, or
  artifact-integrity case must return before the caller's mutation phase can
  run. Exercise stable facts and byte-identical results across repeated
  injected observations.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- `go test -race -count=10 ./internal/machine/...` passed ten iterations of
  deterministic success, unsupported platform, inaccessible path, insufficient
  free space, occupied router, corrupt artifact, malformed observation, and
  cancellation cases. Every failing case left the injected mutation sentinel
  at zero calls; corrupt artifacts failed before the Machine observer ran.
- Repeated successful checks produced byte-identical JSON facts and returned
  the exact verified Lima `1.2.3`, guest image `2026.08`, Docker `29.6.1`,
  Compose `5.3.0`, and Git utility `2.43.0` fixture versions. Failure facts
  carried stable reason, observed value, requirement, remediation, and contract
  error code fields suitable for a future doctor renderer.
- `make check` passed on Linux: pinned lint reported `0 issues`, module
  verification, unit and race tests, vet, builds, spike smoke, shell guards,
  and Python automation checks all passed.
- `go test -race -count=10 ./...` passed across every package, including ten
  complete Machine preflight runs. `go vet ./internal/machine/...` and
  `git diff --check` passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only. No Darwin accelerator, filesystem, router, VZ, or VirtioFS
  runtime behavior is claimed.
