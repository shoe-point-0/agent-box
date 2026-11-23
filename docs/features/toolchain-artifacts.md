# Verified runtime artifact inputs

Status: implemented.

The `internal/toolchain` package is the single verification boundary for the
Lima executable, guest image, Docker, Compose, and named utility artifacts
consumed by [Machine preflight](machine-preflight.md),
[Lima runtime-host adapter](lima-runtime-host.md), and
[guest toolchain](guest-toolchain.md). Specifications require canonical
lowercase `sha256:` digests and exact versions; providers resolve local
candidates only; and verification fails closed with typed prerequisite errors on
invalid specifications, missing or unsafe files, metadata mismatches, or changed
bytes.

Consumers receive opaque verified artifacts that can only be staged from an
already-open verified descriptor or executed through a path that re-hashes the
binary immediately before invoking the owned process runner. The reusable
`internal/toolchain/toolchaintest` conformance suite governs preinstalled,
fixture, and future production providers alike, while explicit consent,
downloads, signature verification, atomic installation, and partial-download
cleanup remain [Milestone 5 bootstrap work](../../ROADMAP.md#milestone-5-releasable-v01-candidate).

## Scope and dependencies

- Milestone 1 artifact verification boundary. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Sections 6, 16.5, and 22.3.

## Behavior

- Defines immutable verified component identities (semantic component/name, non-empty version, and canonical lowercase `sha256:` digest) for Lima, the guest image, Docker, Compose, and named utilities.
- Resolves local candidates through a context-aware `Provider` with an offline preinstalled implementation and zero download or network side effects in the verification contract.
- Verifies provider metadata, canonical regular-file paths, executable permissions where required, and SHA-256 content digests before returning opaque artifacts, then re-verifies digests immediately prior to execution or streams guest/image bytes from the verified open file descriptor.
- Returns typed prerequisite errors (`ABOX_PREREQUISITE_FAILED`) for invalid specifications, missing candidates, version/identity mismatches, unsafe files, and digest mismatches.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Unverified, zero-value, non-executable, or post-verification-mutated artifacts can never reach execution.

## Verified acceptance criteria

- Missing, tampered, or mismatched artifacts fail closed with typed prerequisite errors.
- No consumer can execute an artifact lacking verified identity and fresh content digest verification.
- Preinstalled, fixture, and future download providers share one conformance suite (`internal/toolchain/toolchaintest`).

## Implementation details

- `internal/toolchain` separates artifact verification from acquisition: the `Provider` interface contains no download method. A bootstrapper must obtain explicit user consent and install files before presenting local candidates.
- `internal/toolchain/toolchaintest` provides the reusable conformance test harness run against both the preinstalled provider and injected fixture providers.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `go test -race -count=10 ./internal/toolchain/...` passed ten iterations of both the preinstalled and injected-fixture conformance suites covering complete resolution, missing artifacts, version mismatches, digest mismatches, cancellation, non-executable files, invalid specifications, and file mutation between verification and execution.
- Execution-gate tests confirmed a verified Lima fixture executed once, then failed with `digest-mismatch` (`ABOX_PREREQUISITE_FAILED`) after its bytes changed without invoking the runner a second time, and confirmed guest images and zero-value artifacts cannot reach execution.
- `make check` (`0` lint issues) and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed on Linux.
