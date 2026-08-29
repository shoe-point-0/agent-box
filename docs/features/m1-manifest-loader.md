<!-- sprout-task
{"schema_version":1,"id":"m1-manifest-loader","status":"implemented","created_at":"2026-08-29T06:48:35.969030304Z","implemented_at":"2026-08-29T19:23:44.030893289Z"}
-->

# Implement the strict v1alpha1 Workspace loader

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the strict v1alpha1 Workspace loader

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 7 and Issue 2.

## Requirements

- Define the Workspace schema and strict YAML decoding with unknown-field rejection.
- Validate uniqueness, references, argv-only setup commands, secret references, templates, and normalized quantities.
- Canonicalize paths against explicit roots and reject traversal and symlink escapes.
- Separate the complete v1alpha1 syntax model from the smaller Milestone 1 executable subset; recognized remote sources, caches, setup jobs, routes, TCP ports, and secrets must fail planning with a stable field-aware feature-not-supported error until their owning milestone lands.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] All documented manifest examples parse to the expected model.
- [x] Every validation rule and malicious path fixture has a table-driven test.
- [x] Errors identify a stable field path and never expose secret values.
- [x] Parser limits for input size, nesting, collection counts, path length, and template output are enforced and fuzzed.

## Technical approach

- Add an `internal/manifest` package whose public boundary is a bounded `Loader`, a complete typed `v1alpha1` syntax model, and a separate Milestone 1 executable-subset gate.
- Parse YAML through `go.yaml.in/yaml/v3`, reject aliases, duplicate keys, non-string mapping keys, unknown fields, excessive input/nesting/collection sizes, and malformed document shape before typed decoding. All package-produced errors carry stable manifest field paths and do not interpolate scalar values.
- Validate the complete syntax model in a second pass: required values, named-namespace uniqueness, repository references, argv-only commands, secret-reference shape, documented template tokens, normalized CPU/byte/duration quantities, and safe relative paths.
- Resolve local repository sources with `EvalSymlinks` against explicit canonical source roots. Preserve the authored path while recording the canonical path, and reject lexical traversal, missing targets, root escapes, and symlink escapes.
- Exercise the documented Section 7 example, every validation class, malicious filesystem fixtures, executable-subset errors, redaction, limits, and fuzz entry points. No runtime, Git, Compose, or Darwin behavior is claimed by this loader task.

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
- [x] Run manifest fuzz tests for parser and path boundaries

## Validation evidence

- `make check`: PASS on Linux; formatter, pinned golangci-lint (0 issues), module verification, unit tests, race tests, vet, builds, spike smoke test, shell checks, and Python automation tests all passed.
- `go test -race -count=10 ./...`: PASS across all nine Go packages, including `internal/manifest`.
- `go test -run=^$ -fuzz=FuzzLoadBytes -fuzztime=2s ./internal/manifest`: PASS after 309,619 executions; 190 new interesting inputs beyond the cached baseline.
- `go test -run=^$ -fuzz=FuzzPathBoundaries -fuzztime=2s ./internal/manifest`: PASS after 137,421 executions; 7 new interesting inputs beyond the cached baseline.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation evidence only; no Darwin runtime support is claimed.
- `git diff --check`: PASS.

## Outcome and follow-ups

Implemented a bounded strict loader and complete typed `v1alpha1` syntax model in `internal/manifest`. It rejects unknown/duplicate fields, aliases, malformed shapes, excessive parser dimensions, invalid quantities, invalid references, shell-style setup commands, unsafe paths, unsupported templates, and secret literals with stable value-free field diagnostics. Local sources are canonicalized through symlinks against explicit roots. A separate typed Milestone 1 gate rejects recognized remote repositories, caches, setup jobs, routes, TCP ports, and secrets until their owning milestones land.

The loader validates manifest-level references. Compose service existence and readiness-via-Compose-health remain planner responsibilities because service definitions are intentionally owned by Compose inputs rather than the Workspace syntax model.

## Original request

Implement implement the strict v1alpha1 workspace loader for Agent Box according to the cited specification and dependencies.
