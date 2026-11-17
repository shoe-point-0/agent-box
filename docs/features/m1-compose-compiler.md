<!-- sprout-task
{"schema_version":1,"id":"m1-compose-compiler","status":"implemented","created_at":"2026-08-29T06:48:36.138968013Z","implemented_at":"2026-08-29T20:36:42.900677707Z"}
-->

# Generated Compose inputs

Status: implemented. Implementation record for `m1-compose-compiler`.

- Added `internal/compose` as a read-only compiler boundary. It verifies the exact Plan-pinned Compose version, uses an explicit project name and complete child environment on every normalization, discovers the effective service set before rendering policy, and returns the canonical redacted model digest without applying runtime resources.
- Extended the versioned Plan contract with validated immutable policy so runtime compilation does not consult mutable manifest intent. Updated schema, example, digest input, and canonical Plan golden together.
- Retained security-capability validation and runtime apply/inspect lifecycle for the dependency-ready `m1-compose-policy-core` and `m1-compose-adapter` tasks. Cache volumes remain intentionally deferred to `m2-cache-volumes`; multi-repository include expansion remains deferred to `m2-multi-repo-compose`.

## Scope and dependencies

- Milestone 1. Dependencies: m1-plan-compiler, m1-schema-contract, m1-atomic-writer, m0-compose-probe. Source: THE_BEGINNING.md Sections 8.4 and 12.1.

## Behavior

- Generate compose.root.yaml and compose.abox.yaml atomically from the Plan.
- Set explicit project identity, Box labels, Milestone 1 declared mounts, network policy, and Machine-port suppression; cache-volume projection lands with m2-cache-volumes.
- Normalize through the pinned Compose CLI and retain a redacted model digest.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Generated files are deterministic for one Plan.
- Relative repository paths preserve owning project directories.
- Forbidden publications and secret values are absent from the retained model.

## Implementation details

- Add the missing immutable policy projection to the Plan contract so Compose compilation does not reach back into mutable manifest intent for network policy.
- Compile through a narrow, context-aware command runner: verify the exact Plan-pinned Compose version, normalize the generated include root to discover the effective Milestone 1 service set, then generate policy overrides for that complete set.
- Render deterministic YAML with an explicit project name on every Compose invocation, owning repository `project_directory`, all five ownership labels, exact Plan-declared bind mounts, an internal default network when egress is denied, and `ports: !reset []` for every service.
- Normalize the root plus override before any runtime apply, require all publications to be absent, canonicalize the JSON model, redact registered apply-time secret values through the safe process runner, hash it, and atomically retain all three generated artifacts with owner-only permissions.
- Cover rendering and command contracts with a fake runner, golden files, mutation/version/publication/secret tests, and one real Linux Docker Compose 5.3.0 normalization test guarded by tool availability.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run Compose compiler golden tests
- [x] Normalize generated fixtures with the pinned Compose CLI

## Validation evidence

- `go test ./internal/compose -run '^TestCompileGeneratedInputsAreDeterministic$' -count=100` passed 2,000 compiler iterations with byte-identical `compose.root.yaml`, `compose.abox.yaml`, retained canonical JSON, and model digest.
- The golden root fixes the absolute include and owning worktree `project_directory`; the golden override fixes deterministic all-service port resets, five ownership labels, exact worktree/Git bind projections, Box-network attachment, and denied-egress `internal: true` policy.
- `TestCompileNormalizesWithRealPinnedCompose` passed against the locally executed Linux matrix: Docker client/server 29.6.1 and Docker Compose 5.3.0. Real `docker compose config --format json` resolved relative `build.context: .` to the exact owning worktree, removed every repository publication through `!reset []`, enforced the internal Box network, and retained no registered canary secret.
- Refusal tests proved an unpinned Compose version stops after version preflight, a post-compilation Plan mutation runs no command, a surviving publication writes no retained model, and unredacted registered secret material is rejected.
- Generated artifacts are first normalized in an owned staging directory, then each is crash-consistently replaced with mode `0600` through `atomicfile`; failed normalization leaves previously committed destination artifacts untouched.
- `go test ./...`, `go test -race ./...`, and `go vet ./...` passed across all 16 packages. `make check` passed with 0 lint issues, verified modules, tests/race/vet/build/smoke/automation. `go test -race -count=10 ./...` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` also passed; the latter is compilation evidence only.
