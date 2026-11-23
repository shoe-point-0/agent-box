# Generated Compose inputs

Status: implemented.

The read-only Compose compiler in `internal/compose` compiles an immutable Plan
into deterministic `compose.root.yaml`, `compose.abox.yaml`, and a canonical
redacted JSON model digest without applying any runtime resources. It verifies
the exact Plan-pinned Compose CLI version, discovers the effective service set
from the generated include root, renders policy overrides across every service
and named volume, normalizes through `docker compose config --format json`, and
commits generated artifacts atomically at mode `0600`.

This slice also added immutable policy projection to the versioned Plan
contract so compilation never reaches back into mutable manifest files.
Pre-apply security policy enforcement and runtime lifecycle execution build
directly on this compiler in [Compose security policy](compose-security-policy.md)
and [Compose runtime adapter](compose-runtime.md). Cache volumes and multi-repository
include expansion remain [Milestone 2 work](../../ROADMAP.md#milestones-2-and-3-multi-repository-workspaces-and-routing).

## Scope and dependencies

- Milestone 1 Compose compiler. Depends on [Plan compiler](plan-compiler.md), [schema contracts](schema-contracts.md), [atomic file writer](atomic-file-writer.md), and [Compose include spike](spike-compose-include.md). Source: `THE_BEGINNING.md` Sections 8.4 and 12.1.

## Behavior

- Verifies the exact Plan-pinned Compose version and passes an explicit project name plus complete child environment on every normalization.
- Generates deterministic `compose.root.yaml` (preserving each owning repository's `project_directory`) and `compose.abox.yaml` projecting explicit project identity, all five ownership labels, exact Plan-declared bind mounts, internal Box network policy when egress is denied, and `ports: !reset []` on every service.
- Normalizes root plus override through the pinned Compose CLI, verifies all Machine port publications are absent, canonicalizes the JSON model, redacts registered apply-time secrets through the safe process runner, computes the model digest, and atomically writes all three artifacts with mode `0600`.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Failed normalization, unpinned versions, post-compilation Plan mutation, surviving port publications, or unredacted secrets fail closed without replacing previously committed artifacts.

## Verified acceptance criteria

- Generated files and canonical model digests are byte-identical across repeated compilations of one Plan.
- Relative repository paths (`build.context`, etc.) resolve against their owning worktree `project_directory`.
- Forbidden port publications and registered secret values are absent from the retained normalized model.

## Implementation details

- The versioned Plan schema, example, digest input, and canonical golden include validated immutable network policy so Compose compilation consumes only immutable Plan state.
- Compilation runs through a narrow context-aware command runner in two read-only steps: normalizing the include root to discover the full Milestone 1 service set, then generating and normalizing root plus override.
- Artifacts are staged inside an owned temporary directory and committed via `internal/atomicfile` at mode `0600` only after normalization, publication checks, and secret redaction pass.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run Compose compiler golden tests
- [x] Normalize generated fixtures with the pinned Compose CLI

## Validation evidence

- `go test ./internal/compose -run '^TestCompileGeneratedInputsAreDeterministic$' -count=100` passed `2,000` iterations producing byte-identical `compose.root.yaml`, `compose.abox.yaml`, canonical JSON, and model digest.
- Golden fixtures locked absolute include and owning worktree `project_directory` projection in `compose.root.yaml`, and all-service `!reset []` port resets, five ownership labels, worktree/Git bind mounts, Box-network attachment, and `internal: true` denied-egress policy in `compose.abox.yaml`.
- `TestCompileNormalizesWithRealPinnedCompose` passed against real Linux Docker `29.6.1` and Docker Compose `5.3.0`, confirming relative `build.context: .` resolution, removal of repository port publications, internal network enforcement, and absence of a registered canary secret.
- Negative tests verified refusal on unpinned Compose versions, post-compilation Plan mutation, surviving port publications, and unredacted secret material, leaving prior destination files unchanged.
- `make check` (`0` lint issues across 16 packages), `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed (live Apple M1 compilation inside a Lima guest was subsequently verified in [`abox up`](abox-up.md)).
