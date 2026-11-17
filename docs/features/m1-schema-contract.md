<!-- sprout-task
{"schema_version":1,"id":"m1-schema-contract","status":"implemented","created_at":"2026-08-29T06:55:50.368726934Z","implemented_at":"2026-08-29T20:17:00.246351204Z"}
-->

# Versioned Plan, context, output, and error contracts

Status: implemented. Implementation record for `m1-schema-contract`.

- Added independently versioned Plan, context, CLI output, daemon API, and error
  contracts. The strict Plan surface contains only the Milestone 1 executable
  subset; routes, ports, caches, setup jobs, and secret references are rejected
  until a consuming milestone adds them.
- Added checked-in public schemas and realistic non-secret golden examples.
  Unknown fields are rejected recursively, and no schema defines a secret value,
  credential location, or Docker socket path.
- Published the initial stable symbolic error registry and exhaustive exit-code
  mapping from Section 14.3. Unknown codes fail closed as internal errors.
- Follow-up: `m1-plan-compiler` should construct and canonicalize
  `contract.Plan`; `m1-context-document`, daemon API, and CLI tasks should use
  the same types rather than introducing transport-local copies.

## Scope and dependencies

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 8, 14, and 22.

## Behavior

- Define minimal Milestone 1 schemas and stable symbolic error/exit-code mapping before implementations consume them.
- Keep later milestone fields additive and reject unsupported executable features explicitly.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Schemas validate checked-in golden examples.
- Plan, daemon, CLI, and context packages consume one contract rather than redefining fields.
- No schema permits secret values.

## Implementation details

- Add one dependency-free `internal/contract` package as the canonical Go
  ownership boundary for the independently versioned Plan, daemon API, context,
  CLI output, and error surfaces. Keep the Plan limited to the Milestone 1
  executable subset; later resources remain additive rather than empty promises.
- Publish strict Draft 2020-12 JSON Schemas and checked-in golden documents for
  Plan, context, output, and error payloads. Schemas reject unknown fields and
  expose no secret-value, credential-location, or Docker-socket field.
- Centralize symbolic error definitions and their Section 14.3 exit codes in the
  contract package. Make both current product entrypoints consume that mapping
  while they continue to fail closed for unimplemented behavior.
- Test Go-model/golden parity, real JSON Schema validation, unsupported fields,
  secret-field rejection, and exhaustive error-code mapping. Use the pinned
  validator only from tests so production binaries gain no schema dependency.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- `go test -race -count=10 ./internal/contract ./cmd/abox ./cmd/aboxd`
  passed. The contract suite compiled all four Draft 2020-12 schemas,
  validated their checked-in goldens and Go round trips, injected and rejected
  `secretValue` at every object boundary, rejected deferred Plan resources, and
  checked every symbolic error against its schema enum and exit code.
- `make check` passed after `go mod tidy` classified the pinned test-only JSON
  Schema validator correctly. The first aggregate attempt stopped at
  `go mod tidy -diff`; no test or implementation failure was hidden. The rerun
  reported 0 lint issues, verified modules, passed all package and race tests,
  passed vet and builds, emitted the spike smoke JSON, and passed all three
  automation tests.
- The current `abox` and `aboxd` entrypoints and their tests import
  `internal/contract` for the usage exit status. Plan, context, output, daemon
  error, schema-version, and symbolic-code definitions now have one Go owner.
