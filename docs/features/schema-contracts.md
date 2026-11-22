<!-- sprout-task
{"schema_version":1,"id":"m1-schema-contract","status":"implemented","created_at":"2026-08-29T06:55:50.368726934Z","implemented_at":"2026-08-29T20:17:00.246351204Z"}
-->

# Versioned Plan, context, output, and error contracts

Status: implemented.

The dependency-free `internal/contract` package is the single Go ownership
boundary for Agent Box's independently versioned Plan, context, CLI output,
daemon API, and symbolic error contracts. Matching strict Draft 2020-12 JSON
Schemas and non-secret golden examples live under `schemas/v1alpha1/`.

Downstream Milestone 1 components ([Plan compiler](plan-compiler.md),
[context document](context-document.md), [daemon API](daemon-api.md),
[CLI transport](cli-transport.md), and [`abox up`](abox-up.md)) consume these
shared types directly rather than defining transport-local copies.

## Scope and dependencies

- Milestone 1 contract foundation. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Sections 8, 14, and 22.

## Behavior

- Defines strict, independently versioned schemas and Go types for Plan, context, CLI output, daemon API, and error payloads while keeping future milestone fields additive.
- Rejects unknown fields recursively at every object boundary and forbids secret values, credential locations, and Docker socket paths from appearing in any schema.
- Centralizes symbolic error codes and their Section 14.3 exit-code mappings, failing closed as internal errors for any unknown code.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Production binaries carry no runtime JSON Schema validator dependency; schema validation runs only inside tests.

## Verified acceptance criteria

- All checked-in Draft 2020-12 schemas validate their golden examples and Go round trips.
- Plan, daemon, CLI, and context packages share one canonical Go contract package.
- No schema permits secret values or unknown fields.

## Implementation details

- `internal/contract` owns schema version constants, Go data structures, the symbolic error registry, and exhaustive exit-code lookup consumed by `cmd/abox`, `cmd/aboxd`, and internal packages.
- `schemas/v1alpha1/` stores the strict Draft 2020-12 JSON Schemas and realistic non-secret golden fixtures.
- The Milestone 1 Plan surface restricts executable fields to supported Milestone 1 capabilities (later extended additively by features such as [TCP port allocator](tcp-port-allocator.md)) and rejects deferred features explicitly.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `go test -race -count=10 ./internal/contract ./cmd/abox ./cmd/aboxd` passed, compiling all four Draft 2020-12 schemas, validating checked-in goldens and Go JSON round trips, rejecting injected `secretValue` fields at every object boundary, rejecting deferred Plan resources, and checking every symbolic error code against its schema enum and exit code.
- `make check` passed with `0` lint issues, verified `go.mod`/`go.sum` (classifying the pinned test-only JSON Schema validator), all unit and race tests, `go vet`, builds, spike smoke output, and automation checks.
