<!-- sprout-task
{"schema_version":1,"id":"m1-project-foundation","status":"implemented","created_at":"2026-08-29T06:48:35.92485512Z","implemented_at":"2026-08-29T15:13:59.387316546Z"}
-->

# Go project and contributor foundation

Status: implemented. Implementation record for `m1-project-foundation`.

- Added minimal `abox` and `aboxd` entrypoints plus one shared internal build-identity package. Both binaries expose only version reporting and fail closed for unfinished commands.
- Replaced the spike-only Make contract with a clean-clone product quality gate and remediated pre-existing spike findings exposed by the new standard linter set.
- Added SHA-pinned Linux/macOS quality CI, pull-request dependency review, contributor onboarding, conduct and security policies, and an indexed ADR convention without claiming an implemented runtime.
- Follow-up: a repository administrator must enable GitHub private vulnerability reporting (or publish another private channel) before release or broader governance-readiness claims.
- Follow-up: remote Apple Silicon CI and the dedicated real-hardware VZ/VirtioFS gate remain separate evidence obligations; cross-compilation does not satisfy them.

## Scope and dependencies

- Milestone 1. Dependencies: m0-substrate-report. Source: THE_BEGINNING.md Sections 18, 22, and 25.

## Behavior

- Evolve the Milestone 0 module into cmd/abox and cmd/aboxd entrypoints, production Makefile targets, and only the package layout earned by the first slice.
- Configure formatting, vet, unit tests, race tests, linting, dependency review, and supported macOS CI.
- Add README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, and ADR conventions without claiming unfinished capabilities.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Both binaries build and report version information.
- CI runs the documented local quality gates.
- A new contributor can run the checks from a clean clone.

## Implementation details

- Keep the disposable `abox-spike` command and `internal/spike` evidence harness intact, while adding only the production `cmd/abox`, `cmd/aboxd`, and shared `internal/version` package earned by this slice.
- Give both product binaries an explicit, tested `--version`/`version` path backed by build-time version, commit, and date fields; reject every unimplemented command rather than presenting a mock product.
- Make `make check` the clean-clone contract: format verification, pinned `golangci-lint`, module tidiness/integrity, unit and race tests, vet, product builds, spike smoke, and automation checks. Keep generated binaries and the pinned tool under ignored repository-local directories.
- Run the same aggregate gate on Linux and a GitHub-hosted Apple Silicon macOS 15 runner. Add a separate pull-request dependency-review job; pin all third-party actions to immutable commits.
- Publish contributor, conduct, security, and ADR-process documents that describe the pre-alpha implementation boundary and do not claim Lima/VirtioFS runtime support.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] go build ./cmd/abox ./cmd/aboxd

## Validation evidence

- `make check` passed from the repository root. The pinned `golangci-lint v2.13.1` reported `0 issues`; module tidiness/integrity, all five Go package test suites, race tests, vet, both versioned product builds, spike smoke, and automation checks passed.
- A source-only export with no `.git`, `.tools`, `bin`, or `nohup.out` was placed at a path containing spaces. Its first `make check` installed the pinned linter under that export and passed every gate, proving the documented clean-clone workflow without relying on repository-local generated artifacts.
- Built binaries reported `abox 19d6f8c-dirty (commit 19d6f8c5c78801f1c4ef53d14e1c1f7cbbd2fe94, built 2026-08-29T02:39:43-07:00)` and the corresponding `aboxd` identity. Unit tests also cover development metadata and refusal of unimplemented commands.
- `go test -race -count=10 ./...` passed for `cmd/abox`, `cmd/abox-spike`, `cmd/aboxd`, `internal/spike`, and `internal/version`.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/abox ./cmd/aboxd` passed. This is compilation evidence only, not Darwin runtime evidence.
- Ruby parsed both workflow files and `.golangci.yml`; a repository check verified that CI includes `ubuntu-24.04`, Apple Silicon `macos-15`, and `make check`, and that every action reference is a 40-character commit SHA. Remote CI has not run for this unpushed commit.
- GitHub reported the repository is public and private vulnerability reporting is disabled. `SECURITY.md` records that missing channel as an explicit pre-release governance blocker instead of publishing a dead disclosure path as operational.
