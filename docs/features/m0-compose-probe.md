<!-- sprout-task
{"schema_version":1,"id":"m0-compose-probe","status":"implemented","created_at":"2026-08-29T06:48:35.861247609Z","implemented_at":"2026-08-29T08:00:19.561956518Z"}
-->

# Prove multi-repository Compose include semantics

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Prove multi-repository Compose include semantics

## Context

- Milestone 0. Dependencies: m0-spike-harness. Source: THE_BEGINNING.md Sections 8.4, 12, and 19.2.

## Requirements

- Create two repository fixtures with relative build contexts and Compose include files.
- Generate a root include model and Agent Box override using the pinned Compose version.
- Test removal of repository-owned published ports without changing repository files.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Compose normalization resolves each relative path from its owning repository.
- [x] The combined project starts all expected services.
- [x] No repository-declared port binds the Machine after policy adaptation.

## Technical approach

- Register a Linux `compose-include` scenario that owns one marked fixture tree and one explicit Compose project labeled with the persisted run ID.
- Generate two repository directories, each with a relative `build.context: .`, distinct copied identity, and repository-owned ephemeral loopback port publication. Generate a relative root `include` model plus an Agent Box override using Compose `!reset []` for ports.
- Normalize the baseline and adapted models with Docker Compose 5.3.0. Refuse to start unless baseline ports are present, adapted ports are absent, service names are exact, and normalized build contexts equal their canonical owning repository paths.
- Build and start only the adapted project from a locally cached pinned base-image ID, verify both containers are running with the expected copied identities, and inspect Docker HostConfig plus `docker port` to prove no Machine publication exists.
- Record the project as planned before apply and created after successful apply. Scenario cleanup observes even planned projects, verifies exact Compose/custom owner labels, runs project-scoped `down --volumes --remove-orphans`, proves resources are absent, journals removal, then removes the marker-verified fixture tree.
- Emit a typed retained report and make no Compose-version or platform claim beyond the executed Linux matrix.

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
- [x] Run docker compose config --format json on the generated fixture
- [x] Start the fixture and inspect Machine listeners

## Validation evidence

- Real probe passed on Linux with Docker client/server 29.6.1 and Docker Compose 5.3.0 using cached base image `alpine@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc`; no image pull was required.
- Baseline `docker compose config --format json` retained each repository's loopback ephemeral publication and resolved `build.context` to the exact canonical `repo-alpha` and `repo-beta` directories.
- Adapted normalization through the generated `compose.abox.yaml` removed both `ports` fields via `!reset []` before apply while retaining the exact repository-local build contexts.
- Project `aboxspike3fb88a54b763` built and started exactly two services. `alpha` returned `alpha-repository` from its built image and `beta` returned `beta-repository`, proving each relative Dockerfile/COPY context belonged to the correct repository.
- Docker inspection reported `{}` HostConfig port bindings, empty `docker port` output, empty rendered `Ports`, running state, exact Compose service labels, and exact persisted run-ID owner labels for both containers.
- Hash/mode snapshots proved the generated repository Dockerfiles, Compose files, and identities were unchanged by root composition or policy adaptation.
- With explicit approval, cleanup verified project and custom owner labels on containers and the default network, ran project-scoped `down --volumes --remove-orphans --rmi local`, verified both explicit built images were absent, removed the marker-verified fixture tree, and left no labeled container, network, run root, or parent directory.
- `make check`, `go test -race -count=10 ./...`, and `sprout check --all` passed. Artifact-free Darwin/arm64 compilation passed but remains unvalidated hardware evidence.

## Outcome and follow-ups

- Delivered the `compose-include` Linux scenario and typed `abox.spike.compose-include/v1` report.
- Proved Compose include path ownership, repository-relative build contexts, deterministic generated root/override files, pre-apply port suppression, combined service startup, identity-correct images, zero Machine publications, and ownership-verified teardown.
- Extended cleanup to pass planned scenario resources, allowing partial Compose applies to be observed and reconciled without trusting only a successful-created journal entry.
- No Darwin, Lima forwarding, or macOS Compose behavior is claimed; the same fixture remains available to the final Darwin/arm64 gate.

## Original request

Implement prove multi-repository compose include semantics for Agent Box according to the cited specification and dependencies.
