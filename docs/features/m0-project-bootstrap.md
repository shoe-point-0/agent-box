<!-- sprout-task
{"schema_version":1,"id":"m0-project-bootstrap","status":"implemented","created_at":"2026-08-29T06:55:50.347590606Z","implemented_at":"2026-08-29T07:10:14.628492871Z"}
-->

# Disposable Go spike workspace

Status: implemented. Implementation record for `m0-project-bootstrap`.

- Delivered a dependency-free Go module, exact Apache-2.0 license, disposable empty spike registry command, internal registry tests, mode-0700 owned temporary roots, and Make quality gates.
- No stable product API, daemon, Lima/Compose adapter, or platform support claim was introduced.
- Next task: `m0-spike-harness` adds the bounded runner, resource inventory, diagnostics, and refusal-first cleanup contract used by substrate probes.

## Scope and dependencies

- Milestone 0 prerequisite with no dependencies. Source: THE_BEGINNING.md Section 25 step 1.

## Behavior

- Create the minimal Go module, disposable spike command/package, Apache-2.0 license, formatting, test, race, vet, and temporary-root support needed by Milestone 0.
- Avoid product binaries, stable APIs, production package layout, and contributor/release machinery until substrate evidence exists.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- A clean clone can build and run the empty spike registry.
- go test, race, and vet pass before any substrate probe starts.
- The bootstrap contains no claimed Agent Box product behavior.

## Implementation details

- Initialize one dependency-free Go module using the repository's canonical GitHub path.
- Add a disposable `cmd/abox-spike` executable backed by an `internal/spike` registry; the only initial behavior is deterministic discovery of an empty registry.
- Add an owned temporary-root helper that creates mode-0700 directories beneath a caller-selected parent and only removes the exact root it created.
- Keep all behavior internal and disposable: no `abox`/`aboxd` binaries, public Go packages, Lima/Compose interfaces, or product support claims.
- Provide boring Make targets for formatting, tests, race tests, vet, build, and the aggregate check.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- `make check` passed: format check, both package test suites, race detector, vet, artifact-free build, and empty-registry smoke run.
- `GOTOOLCHAIN=local GOPROXY=off go test ./...`, `go build ./...`, and `go run ./cmd/abox-spike list` passed after `go clean -cache`; smoke output was `no spikes registered`.
- `go list ./...` reported only `cmd/abox-spike` and `internal/spike`; `cmd/abox` and `cmd/aboxd` do not exist.
- `LICENSE` matched the system Apache-2.0 license byte-for-byte.
- Build verification leaves no root-level `abox-spike` artifact.
