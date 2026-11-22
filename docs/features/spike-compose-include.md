<!-- sprout-task
{"schema_version":1,"id":"m0-compose-probe","status":"implemented","created_at":"2026-08-29T06:48:35.861247609Z","implemented_at":"2026-08-29T08:00:19.561956518Z"}
-->

# Multi-repository Compose include semantics

Status: implemented.

The `compose-include` spike scenario verifies that Docker Compose `include`
keeps relative build contexts bound to their owning repository, removes
repository-owned Machine port publications before apply via a generated
Agent Box override, starts combined services with distinct image identities,
and tears down only ownership-verified project resources. Results are recorded
as typed `abox.spike.compose-include/v1` JSON evidence.

No Darwin, Lima forwarding, or macOS Compose behavior is claimed from this
Linux probe; the same fixture remains reusable at the Darwin/arm64 release gate.

## Scope and dependencies

- Milestone 0 substrate probe. Depends on [spike harness](spike-harness.md). Source: `THE_BEGINNING.md` Sections 8.4, 12, and 19.2.

## Behavior

- Generates two repository fixtures (`repo-alpha` and `repo-beta`), each with a relative `build.context: .`, distinct copied identity, and an ephemeral loopback port publication.
- Produces a root Compose `include` file and an Agent Box override using Compose `!reset []` to strip repository-owned port publications without modifying repository files.
- Normalizes both baseline and adapted models, starts only the adapted project, and verifies container identities and absence of Machine port bindings.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Compose normalization resolves each relative path from its owning repository.
- The combined project starts all expected services with repository-correct built images.
- No repository-declared port binds the Machine after policy adaptation.

## Implementation details

- The `compose-include` scenario registers in `cmd/abox-spike` and owns one marker-verified fixture tree plus one Compose project labeled with the persisted run ID.
- Normalization runs `docker compose config --format json` on both baseline and adapted models. Execution refuses to proceed unless baseline ports are present, adapted ports are absent, service names match exactly, and normalized build contexts equal each canonical repository directory.
- Services build and start from a locally cached pinned base-image digest without network pulls. Post-start inspection checks Docker `HostConfig` bindings, `docker port`, service labels, and run-ID owner labels.
- Projects are recorded as `planned` before apply and `created` afterward. Cleanup observes planned projects as well so interrupted applies converge safely: it verifies Compose and custom owner labels, runs project-scoped `down --volumes --remove-orphans --rmi local`, confirms absence of containers, networks, and built images, journals removal, and deletes the marked fixture tree.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run `docker compose config --format json` on the generated fixture
- [x] Start the fixture and inspect Machine listeners

## Validation evidence

- Linux probe passed with Docker client/server `29.6.1`, Docker Compose `5.3.0`, and cached base image `alpine@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc` (no image pull required).
- Baseline normalization retained each repository's loopback port publication and resolved `build.context` to the exact canonical `repo-alpha` and `repo-beta` directories; adapted normalization through `compose.abox.yaml` removed both `ports` fields via `!reset []` while preserving both build contexts.
- Project `aboxspike3fb88a54b763` started two services returning `alpha-repository` and `beta-repository`, with `{}` `HostConfig` port bindings, empty `docker port` output, and exact Compose and run-ID owner labels.
- Hash and mode snapshots confirmed repository Dockerfiles, Compose files, and identities were untouched by composition or policy adaptation.
- Ownership-verified cleanup removed containers, the default network, built local images, and the marked fixture tree with zero labeled residue remaining.
- `make check`, `go test -race -count=10 ./...`, and `sprout check --all` passed; `GOOS=darwin GOARCH=arm64` compilation passed as artifact evidence only.
