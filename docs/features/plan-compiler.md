# Canonical immutable Plans

Status: implemented.

The `internal/plan` package is a read-only compiler that turns validated
manifest intent, deterministic identities, repository resolution facts, and
explicit runtime/tool facts into a canonical, immutable `v1alpha1` Plan
document and SHA-256 `planDigest`. It enforces the Milestone 1 executable
subset before accessing inputs, hashes bounded regular manifest and Compose
files, projects the exact one-repository ownership and mount allowlist, and
refuses post-compilation mutation via `CanonicalJSON`.

Compose normalization and security policy validation are performed downstream
by [Compose compiler](compose-compiler.md) and
[Compose security policy](compose-security-policy.md); Plan change classification
is handled by [Plan diff](plan-diff.md).

## Scope and dependencies

- Milestone 1 Plan compiler. Depends on [schema contracts](schema-contracts.md), [manifest loader](manifest-loader.md), [identity and paths](identity-and-paths.md), and [repository resolver](git-repository-resolver.md). Source: `THE_BEGINNING.md` Section 8.2 and Issue 6.

## Behavior

- Compiles the Milestone 1 executable subset—one local repository, host resources, one Compose include, workspace service, and required mounts—plus platform and repository facts into the versioned Plan contract without materializing resources.
- Rejects deferred intent (routes, TCP ports, caches, setup keys, remote sources, and secrets) and unsupported Milestone 1 cardinalities via the loader's feature gate before reading input files, leaving no input channel for secret values.
- Hashes manifest and repository-owned Compose files via bounded regular-file reads under the trusted source root, rejecting symlinks and root escapes while projecting future Box worktree paths into the Plan.
- Canonicalizes and deduplicates set-like profiles, services, and mounts before map-free JSON encoding, and computes `planDigest` as SHA-256 over the canonical document with the self-referential digest field omitted.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- `CanonicalJSON` refuses any Plan mutated after compilation, and no secret value can enter the Plan or its digest input.

## Verified acceptance criteria

- Shuffled map/collection order and repeated runs produce byte-identical Plans and `planDigest` values.
- Compiled Plans carry every ownership and observation key required by Git, Lima, and Compose adapters.
- Recognized secret intent fails before artifact access, and host environment canaries cannot enter compiled output.

## Implementation details

- `internal/plan` projects every adapter ownership and observation key from typed facts: source and worktree paths, private Git common directory, base commit and generated branch, Lima instance identity, Compose project and input digest, workspace service, exact worktree and Git mount allowlist, normalized host resources, schema version, and exact tool versions.
- Set-like slices are sorted and deduplicated prior to serialization through the map-free `internal/contract` Go structs, guaranteeing stable byte layout across Go runtime runs.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run canonical Plan golden tests repeatedly with randomized input order

## Validation evidence

- `go test -run '^TestCompileCanonicalGoldenIsStable$' -count=100 ./internal/plan` passed `10,000` compilations (`100` shuffled profile and mount-service insertion orders per run) producing byte-identical canonical Plans validated against `schemas/v1alpha1/plan.schema.json`.
- `TestCompileHasNoSecretValueInputChannel` verified host-environment canaries cannot enter output and recognized `spec.secrets` intent fails before artifact access.
- `make check` (`0` lint issues across 16 packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
