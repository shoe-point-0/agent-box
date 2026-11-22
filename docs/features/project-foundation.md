<!-- sprout-task
{"schema_version":1,"id":"m1-project-foundation","status":"implemented","created_at":"2026-08-29T06:48:35.92485512Z","implemented_at":"2026-08-29T15:13:59.387316546Z"}
-->

# Go project and contributor foundation

Status: implemented.

This component establishes the production `cmd/abox` and `cmd/aboxd` binary
entrypoints, shared build-identity metadata (`internal/version`), the
clean-clone `make check` quality gate, SHA-pinned Linux and macOS CI workflows,
and baseline contributor, security, conduct, and ADR documentation while
retaining the Milestone 0 `cmd/abox-spike` harness.

Subsequent Milestone 1 work populated the CLI commands and daemon runtime
pipeline behind these entrypoints; see [CLI transport](cli-transport.md),
[daemon API](daemon-api.md), and [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 foundation. Depends on [substrate evidence report](spike-substrate-report.md). Source: `THE_BEGINNING.md` Sections 18, 22, and 25.

## Behavior

- Builds versioned `bin/abox` and `bin/aboxd` binaries reporting build version, commit SHA, and build timestamp via `--version` / `version`, failing closed on unimplemented commands.
- Runs the aggregate local and CI quality gate via `make check`: formatting, pinned `golangci-lint`, module tidiness and integrity, unit tests, race tests, `go vet`, product builds, spike smoke output, and repository automation tests.
- Provides onboarding, contribution, conduct, security disclosure, and ADR governance documents aligned with the project's pre-alpha boundaries.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- GitHub private vulnerability reporting remains a repository-administrator prerequisite noted in `SECURITY.md` before release claims.

## Verified acceptance criteria

- Both `abox` and `aboxd` build cleanly and report accurate version metadata.
- CI runs the documented local quality gates on Ubuntu 24.04 and macOS 15 alongside PR dependency review.
- A new contributor can run `make check` from a clean clone without pre-existing local build artifacts.

## Implementation details

- `cmd/abox`, `cmd/aboxd`, and `internal/version` inject version, commit, and build date via `-ldflags` during `make build` while keeping `cmd/abox-spike` and `internal/spike` intact.
- `Makefile` installs pinned `golangci-lint` (`v2.13.1`) into ignored `.tools/bin` and writes binaries into ignored `bin/`.
- GitHub Actions workflows pin every third-party action to an immutable 40-character commit SHA and run `make check` across `ubuntu-24.04` and Apple Silicon `macos-15` runners.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] `go build ./cmd/abox ./cmd/aboxd`

## Validation evidence

- `make check` passed with pinned `golangci-lint v2.13.1` (`0 issues`), module verification, unit and race tests across all five initial packages, `go vet`, versioned builds, spike smoke, and automation checks.
- A clean source-only export placed at a path containing spaces installed the pinned linter from scratch and passed `make check` end to end.
- Built binaries reported exact commit/date version metadata, `go test -race -count=10 ./...` passed, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/abox ./cmd/aboxd` passed as compilation evidence only.
- Workflow YAML validation confirmed `ubuntu-24.04`, `macos-15`, `make check`, and 40-character SHA action pinning; `SECURITY.md` explicitly records the missing GitHub private vulnerability reporting channel as a pre-release blocker.
