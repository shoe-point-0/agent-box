<!-- sprout-task
{"schema_version":1,"id":"m1-identity-paths","status":"implemented","created_at":"2026-08-29T06:48:35.984103316Z","implemented_at":"2026-08-29T15:35:02.570944634Z"}
-->

# Deterministic identity and platform paths

Status: implemented. Implementation record for `m1-identity-paths`.

- Added typed Workspace and Box IDs, strict task-key and generated-ID validation, independent golden vectors, bounded display slugs, and the normative NUL-separated Box hash input.
- Added one provider for explicit platform roots and the complete macOS Section 15 application/cache/log/runtime, Box, private-Git, and workspace layout. Dynamic state components and workspace-relative paths fail closed on traversal, malformed encoding, controls, unsafe separators, and length violations.
- Linux unit and race execution proves deterministic construction and lexical containment. Real Darwin filesystem/runtime behavior remains deferred to the existing Darwin arm64 hardware gate.

## Scope and dependencies

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 3, 8, and 15.

## Behavior

- Derive stable Workspace and Box IDs from canonical inputs using the specified hash and slug contract.
- Centralize macOS application, cache, log, runtime, Box, Git, and workspace paths behind a path provider.
- Validate task keys and path components before filesystem use.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Repeated and randomized runs produce byte-identical IDs.
- Same task keys in unrelated Workspaces cannot collide.
- No package outside the path provider constructs state paths ad hoc.

## Implementation details

- Add an `internal/identity` package that derives `ws_<8 lowercase,
  unpadded-base32 SHA-256 characters>` from an absolute, cleaned canonical
  manifest path and derives the Box display ID exactly from
  `workspace_id + NUL + task_key`. Keep the original validated task key in the
  hash while using a bounded lowercase ASCII slug only for display/path use.
- Add an `internal/paths` provider with explicit roots and a Darwin constructor
  for the Section 15 layout. Make every state, cache, log, runtime, Box,
  Box-private Git, and workspace path flow through typed provider methods.
- Reject empty, relative, unclean, NUL-bearing, control-bearing, traversal, and
  overlong identity/path inputs before returning filesystem paths. Permit
  ordinary Unicode and spaces only in validated workspace-relative path
  components, where the invariant-path contract requires them.
- Prove the contracts with independent golden vectors, randomized repeat and
  cross-Workspace collision tests, adversarial component/containment tests,
  and exact Darwin layout tests runnable on the Linux implementation lane.
  Darwin runtime behavior remains unclaimed.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run identity golden and collision-property tests

## Validation evidence

- `go test -run 'TestIdentityGolden|TestIdentityRepeatedRandomized|TestSameTaskAcrossWorkspacesDoesNotCollide' -count=100 ./internal/identity` passed 100 repeated runs, including 1,000 deterministic randomized vectors and 1,000 unrelated Workspace paths per run.
- `make check` passed formatting, pinned lint (`0 issues`), module verification, unit tests, race tests, vet, builds, spike smoke output, and automation tests across eight Go packages.
- `go test -race -count=10 ./...` passed across all eight Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation evidence only; no Darwin runtime support is claimed.
- `git diff --check` passed. A source search for Section 15 state filenames and roots found production construction only in `internal/paths`; the retained `internal/spike` Compose fixture remains disposable Milestone 0 probe input rather than application state.
