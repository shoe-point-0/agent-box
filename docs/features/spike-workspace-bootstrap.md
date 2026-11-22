<!-- sprout-task
{"schema_version":1,"id":"m0-project-bootstrap","status":"implemented","created_at":"2026-08-29T06:55:50.347590606Z","implemented_at":"2026-08-29T07:10:14.628492871Z"}
-->

# Disposable Go spike workspace

Status: implemented.

This bootstrap established the minimal, dependency-free Go workspace, Apache-2.0
license, disposable `cmd/abox-spike` registry, mode-`0700` owned temporary-root
helper, and Makefile quality gates required to execute Milestone 0 substrate
probes before product code existed.

Later Milestone 1 components added the production `abox` and `aboxd` binaries
on top of this module; see [project foundation](project-foundation.md)
and [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 0 prerequisite with no dependencies. Source: `THE_BEGINNING.md` Section 25 step 1.

## Behavior

- Provides a Go module at the repository's canonical module path with Apache-2.0 licensing and standard `make` targets (`fmt-check`, `test`, `test-race`, `vet`, `build`, `check`).
- Exposes a disposable `cmd/abox-spike` CLI backed by `internal/spike` for deterministic scenario discovery.
- Creates mode-`0700` temporary roots under a caller-selected parent and restricts deletion to the exact created root.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Milestone 0 bootstrap stays internal and disposable, avoiding premature product APIs or platform support claims.

## Verified acceptance criteria

- A clean clone builds and runs the empty spike registry.
- `go test`, `-race`, and `go vet` pass before any substrate probe starts.
- The bootstrap contains no claimed Agent Box product behavior.

## Implementation details

- `cmd/abox-spike` delegates to an internal registry in `internal/spike`, initially reporting `no spikes registered` until probe scenarios register in subsequent Milestone 0 work.
- Temporary-root creation enforces mode `0700` and identity-scoped removal beneath the caller-specified parent directory.
- `Makefile` defines deterministic local quality checks covering formatting, unit tests, race detection, `go vet`, builds, and registry smoke checks without leaving stray build artifacts at the repository root.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- At bootstrap closure, `make check` passed formatting, unit tests, race detection, `go vet`, artifact-free builds, and the empty-registry smoke run (`no spikes registered`).
- `GOTOOLCHAIN=local GOPROXY=off go test ./...`, `go build ./...`, and `go run ./cmd/abox-spike list` passed cleanly after `go clean -cache`.
- `go list ./...` reported only `cmd/abox-spike` and `internal/spike` before Milestone 1 added product binaries, and `LICENSE` matched the standard Apache-2.0 text byte-for-byte.
