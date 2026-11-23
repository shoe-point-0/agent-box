# Strict v1alpha1 Workspace loader

Status: implemented.

The `internal/manifest` package implements a bounded strict YAML loader, the
complete typed `v1alpha1` Workspace syntax model, and a separate Milestone 1
executable-subset gate. It rejects unknown or duplicate fields, YAML aliases,
non-string keys, excessive parser dimensions, invalid quantities, shell-style
setup commands, traversal or symlink escapes, unsupported templates, and secret
literals with stable field-path errors that never echo scalar values.

The loader validates manifest-level structure and references only; Compose
service existence and Compose-based readiness remain planner responsibilities
because service definitions belong to Compose inputs rather than
`.abox/box.yaml`.

## Scope and dependencies

- Milestone 1 manifest contract. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Section 7 and Issue 2.

## Behavior

- Decodes `v1alpha1` Workspace manifests strictly through `go.yaml.in/yaml/v3`, rejecting aliases, duplicate keys, non-string mapping keys, unknown fields, malformed shapes, and inputs exceeding size, nesting, collection-count, path-length, or template-output bounds.
- Validates required fields, namespace uniqueness, repository references, argv-only setup commands, secret-reference syntax, documented template tokens, normalized CPU/byte/duration quantities, and safe relative paths.
- Resolves local repository sources with `EvalSymlinks` against explicit canonical source roots, preserving authored paths alongside canonical paths while rejecting lexical traversal, missing targets, root escapes, and symlink escapes.
- Separates the full `v1alpha1` syntax model from the Milestone 1 executable subset so recognized remote repositories, caches, setup jobs, routes, TCP ports, and secrets fail planning with stable field-aware `ABOX_FEATURE_NOT_SUPPORTED` errors until their owning milestones land.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Diagnostics identify stable manifest field paths and never interpolate scalar or secret values.

## Verified acceptance criteria

- All documented `THE_BEGINNING.md` Section 7 manifest examples parse to the expected typed model.
- Every validation rule and hostile path fixture is covered by table-driven tests.
- Parser bounds for input size, nesting, collection counts, path length, and template output are enforced and fuzzed.

## Implementation details

- `internal/manifest.Loader` runs two passes: bounded structural YAML validation followed by semantic model validation and symlink-aware local root canonicalization.
- Executable-subset gating runs as an explicit separate check over the parsed model so future milestones can enable features without changing syntax parsing.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run manifest fuzz tests for parser and path boundaries

## Validation evidence

- `make check` (`0` lint issues across nine packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
- `go test -run=^$ -fuzz=FuzzLoadBytes -fuzztime=2s ./internal/manifest` passed after `309,619` executions (`190` new corpus inputs beyond baseline).
- `go test -run=^$ -fuzz=FuzzPathBoundaries -fuzztime=2s ./internal/manifest` passed after `137,421` executions (`7` new corpus inputs beyond baseline).
