<!-- sprout-task
{"schema_version":1,"id":"m1-identity-paths","status":"implemented","created_at":"2026-08-29T06:48:35.984103316Z","implemented_at":"2026-08-29T15:35:02.570944634Z"}
-->

# Deterministic identity and platform paths

Status: implemented.

The `internal/identity` and `internal/paths` packages provide deterministic
Workspace and Box identifiers alongside one centralized path provider for all
application, cache, log, runtime, Box, private-Git, and workspace directories.
All dynamic identity and path segments fail closed on traversal, NUL/control
characters, unsafe separators, or length violations before touching disk.

Darwin path-layout behavior was later exercised end to end on Apple M1 in
[`abox up`](abox-up.md); note that Darwin's 104-byte Unix
socket path limit still affects Lima control sockets under default macOS
Application Support paths (documented in [getting started](../getting-started.md#daemon-startup-and-paths)).

## Scope and dependencies

- Milestone 1 identity and storage layout foundation. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Sections 3, 8, and 15.

## Behavior

- Derives deterministic Workspace IDs (`ws_<8 lowercase unpadded-base32 SHA-256 chars>`) from canonical absolute manifest paths and Box IDs from `workspace_id + NUL + task_key` paired with a bounded lowercase ASCII display slug.
- Centralizes explicit-root and macOS Section 15 directory layouts (`state.db`, cache, logs, runtime, `boxes/<box-id>/`, private Git stores, and linked worktrees) behind typed `paths.Provider` methods.
- Validates task keys, generated IDs, and workspace-relative paths before returning any filesystem path, permitting ordinary Unicode and spaces only inside validated workspace-relative components where invariant paths require them.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- No production package outside `internal/paths` constructs Box or application state paths ad hoc.

## Verified acceptance criteria

- Repeated and randomized runs produce byte-identical Workspace and Box IDs.
- Identical task keys across unrelated Workspaces never collide.
- All production state paths originate from `internal/paths`.

## Implementation details

- `internal/identity` hashes the exact validated task key (not just the display slug) with the NUL-separated Workspace ID so visually similar task keys remain collision-free while display paths stay safe and bounded.
- `internal/paths` exposes constructors for explicit roots and the macOS Section 15 layout, rejecting empty, relative, unclean, NUL-bearing, control-bearing, traversal, or overlong components before returning paths.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run identity golden and collision-property tests

## Validation evidence

- `go test -run 'TestIdentityGolden|TestIdentityRepeatedRandomized|TestSameTaskAcrossWorkspacesDoesNotCollide' -count=100 ./internal/identity` passed 100 repetitions, covering 1,000 deterministic randomized vectors and 1,000 unrelated Workspace paths per run.
- `make check` (`0` lint issues across eight packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
- Source inspection confirmed all production Section 15 state filenames and roots are constructed exclusively in `internal/paths`.
