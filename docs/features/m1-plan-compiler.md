<!-- sprout-task
{"schema_version":1,"id":"m1-plan-compiler","status":"implemented","created_at":"2026-08-29T06:48:36.064104371Z","implemented_at":"2026-08-29T20:25:05.756754299Z"}
-->

# Compile canonical immutable Plans

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Compile canonical immutable Plans

## Context

- Milestone 1. Dependencies: m1-schema-contract, m1-manifest-loader, m1-identity-paths, m1-repository-resolver. Source: THE_BEGINNING.md Section 8.2 and Issue 6.

## Requirements

- Compile the Milestone 1 executable subset—one local repository, host resources, one Compose include, workspace service, and required mounts—plus platform and repository facts into the versioned Plan model.
- Add routes, ports, caches, setup keys, remote sources, and opaque secret references only in the milestone that first consumes each field; reject recognized unsupported intent before side effects.
- Serialize canonical JSON and compute a stable digest excluding secret values.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Random map order and repeated runs yield byte-identical Plans and digests.
- [x] The Plan contains every ownership and observation key required by adapters.
- [x] No secret value can enter the Plan or its hash input.

## Technical approach

- Add an `internal/plan` compiler that accepts the already validated Workspace,
  deterministic identities, read-only repository facts, a canonical manifest
  path, and explicit runtime/tool facts. It performs no materialization and
  rejects anything outside the one-local-repository Milestone 1 executable
  slice before reading Plan inputs.
- Hash the exact manifest and repository-owned Compose bytes through bounded,
  regular-file-only reads. Resolve the Compose input below the trusted source
  root while projecting its future Box worktree path into the Plan; reject
  symlinks and root escapes rather than admitting an ambiguous mount/input.
- Build every ownership and observation key from typed facts: source and
  worktree paths, private Git common directory, commit and branch, Lima
  instance, Compose project/input digest, workspace service, exact worktree and
  Git mount allowlist, normalized host resources, schema version, and tool
  versions.
- Canonically order set-like values, encode with the map-free versioned Go
  contract, and compute `planDigest` as SHA-256 over the same canonical object
  with the self-referential digest field omitted. Return canonical compact JSON
  and test it against a golden document across shuffled fact/map construction
  and repeated runs. Recognized deferred intent remains rejected by the loader's
  stable feature-not-supported gate, so secret values have no input channel.

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
- [x] Run canonical Plan golden tests repeatedly with randomized input order

## Validation evidence

- `go test -run '^TestCompileCanonicalGoldenIsStable$' -count=100 ./internal/plan`: PASS. Each test run performs 100 compilations while shuffling profile and mount-service insertion order, for 10,000 byte-for-byte comparisons against one canonical result and the normalized golden document.
- The canonical golden test validates the compiled output against `schemas/v1alpha1/plan.schema.json` and asserts repository source/commit/branch/worktree/Git-common-dir keys, manifest and Compose artifact keys, exact worktree/Git mounts, and matching Lima/Compose ownership names.
- `TestCompileHasNoSecretValueInputChannel`: PASS with a host-environment canary absent from output; recognized secret intent returns the typed `spec.secrets` feature refusal before artifact access. The Milestone 1 Plan model has no secret-bearing field.
- `go test ./...`, `go test -race ./...`, and `go vet ./...`: PASS across all 16 Go packages.
- `make check`: PASS on Linux; pinned golangci-lint reported `0 issues`, module verification passed, and unit, race, vet, build, smoke, shell, and Python automation checks passed.
- `go test -race -count=10 ./...`: PASS across all 16 Go packages, including canonical compilation and mutation detection.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as cross-compilation evidence only; no Darwin runtime behavior is claimed.
- `git diff --check`: PASS.

## Outcome and follow-ups

Implemented `internal/plan` as a read-only compiler over validated manifest intent, deterministic identities, repository resolution facts, and explicit runtime/tool facts. It rejects deferred resources and unsupported Milestone 1 cardinalities before input access, validates identity/fact consistency, hashes bounded regular manifest and Compose inputs, and projects the exact one-repository ownership and mount allowlist into the versioned contract.

Set-like profiles, services, and map-derived mounts are sorted and deduplicated before JSON encoding. `planDigest` is SHA-256 over the canonical map-free Plan document with the self-referential digest field omitted; `CanonicalJSON` refuses a Plan mutated after compilation. Compose validation and normalized-model hashing remain owned by the later Compose adapter, while Plan change classification remains in `m1-plan-diff`.

## Original request

Implement compile canonical immutable plans for Agent Box according to the cited specification and dependencies.
