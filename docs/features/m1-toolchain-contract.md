<!-- sprout-task
{"schema_version":1,"id":"m1-toolchain-contract","status":"implemented","created_at":"2026-08-29T06:55:50.411239326Z","implemented_at":"2026-08-30T00:24:53.573766531Z"}
-->

# Define verified runtime artifact inputs

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Define verified runtime artifact inputs

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 6, 16.5, and 22.3.

## Requirements

- Define the immutable verified Lima, guest image, Docker, Compose, and utility identities consumed by preflight and runtime provisioning.
- Support preinstalled/test-fixture providers now and a production download provider later without implicit network access.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Missing or mismatched artifacts fail with typed prerequisite errors.
- [x] No consumer can execute an artifact lacking verified identity.
- [x] Fixture and future production providers share contract tests.

## Technical approach

- Add an `internal/toolchain` boundary with immutable component identities for
  Lima, the guest image, Docker, Compose, and named utilities. An identity is
  valid only with a semantic component/name, non-empty version, and canonical
  lowercase `sha256:` digest.
- Resolve candidates through a context-aware `Provider`; implement an offline
  preinstalled provider now and leave acquisition/consent/network policy out of
  the interface so a later download provider cannot make network access an
  implicit side effect of verification.
- Verify provider identity, canonical regular-file location, executable mode
  where required, and file content before returning opaque verified artifacts.
  Recheck content before each package-owned execution and expose guest/image
  bytes through an already-open verified file descriptor.
- Return typed prerequisite errors for invalid specifications, missing
  candidates, identity mismatch, unsafe files, and digest mismatch. Add a
  reusable provider contract suite and run it against both the preinstalled
  provider and an injected fixture provider so future production providers use
  the same behavioral gate.

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

## Validation evidence

- `go test -race -count=10 ./internal/toolchain/...` passed ten iterations
  of both the preinstalled and injected-fixture provider contract suites. The
  suites exercised complete resolution, missing artifacts, provider version
  mismatch, digest mismatch, cancellation, non-executable files, invalid
  specifications, and mutation between verification and attempted execution.
- The execution-gate test proved a verified Lima fixture reached the injected
  runner once, then changed its bytes and received a typed
  `digest-mismatch`/`ABOX_PREREQUISITE_FAILED` without a second runner call.
  The guest image and a zero/unverified artifact cannot reach execution.
- `make check` passed on Linux: `golangci-lint` reported `0 issues`, module
  verification passed, all Go tests and race tests passed, `go vet ./...`
  passed, both binaries and all packages built, and repository smoke/shell/
  Python checks passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime behavior is claimed.
- The preinstalled provider only resolves caller-supplied local entries. The
  provider contract contains no download method; future bootstrap must perform
  explicit consent and acquisition before presenting a local candidate.

## Outcome and follow-ups

Implemented `internal/toolchain` as the single verification boundary for the
Lima executable, guest image, Docker, Compose, and named utility artifacts.
Specifications require canonical SHA-256 identities and exact versions;
providers return local candidates; verification rejects invalid identities,
missing/unsafe files, mismatched provider metadata, and changed content with
typed prerequisite errors. Consumers receive opaque artifacts, can stage bytes
only from an already verified open descriptor, and execute only through a path
that rechecks content immediately before invoking the owned process runner.

`internal/toolchain/toolchaintest` is the reusable provider conformance suite
for the preinstalled implementation, fixture providers, and the later explicit
download/install implementation. `m1-machine-preflight`, `m1-lima-adapter`, and
`m1-guest-toolchain` own wiring this contract into platform checks and runtime
provisioning; `m5-toolchain-bootstrap` owns consent, download, signatures,
atomic installation, and corrupt-partial cleanup.

## Original request

Implement define verified runtime artifact inputs for Agent Box.
