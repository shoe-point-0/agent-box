<!-- sprout-task
{"schema_version":1,"id":"m0-substrate-report","status":"implemented","created_at":"2026-08-29T06:48:35.907245415Z","implemented_at":"2026-08-29T09:39:25.423012873Z"}
-->

# Publish substrate evidence and lock architectural gates

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Publish substrate evidence and lock architectural gates

## Context

- Milestone 0 Linux implementation gate. Dependencies: m0-project-bootstrap, m0-spike-harness, m0-git-path-probe, m0-compose-probe, m0-routing-probe, m0-failure-matrix. Darwin VZ and VirtioFS evidence is mandatory later in m5-darwin-arm64-hardware-gate but does not block Linux implementation. Source: THE_BEGINNING.md Sections 19.2, 20 Milestone 0, and 25.

## Requirements

- Publish measured commands, versions, results, failures, and cleanup evidence in docs/architecture/substrate-spikes.md.
- Update THE_BEGINNING.md decisions whose assumptions were disproved or narrowed.
- Record accepted decisions and unresolved blockers as ADRs or explicit follow-up tasks.
- Publish a probe-by-probe Linux development matrix with exact kernel, architecture, Git, Docker, Compose, Lima where available, guest image, and cleanup evidence; list every Darwin-only unknown as an explicit final-gate obligation.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Every Milestone 0 exit gate links to reproducible evidence.
- [x] No support claim exceeds the tested OS, architecture, Lima, Git, Docker, and Compose matrix.
- [x] Product implementation remains blocked when a critical substrate gate is red.
- [x] Every Linux-exercisable critical probe passes before implementation proceeds, and no Darwin/macOS support claim exists until m5-darwin-arm64-hardware-gate passes on real hardware.

## Technical approach

- Treat the checked-in executable probes and their implemented Sprout packets as the reproducible evidence boundary; rerun all four Linux scenarios before publishing the aggregate result.
- Publish one matrix that separates tested Linux facts from unavailable Lima/guest facts and Darwin-only obligations. Link every Milestone 0 gate to its scenario packet, command, result, and cleanup proof.
- Add a narrow substrate-gate ADR: accept the Linux Git/Compose/routing/failure mechanisms only for the executed matrix, keep product work blocked on any red Linux gate, and preserve VZ, VirtioFS, mount isolation, and Lima forwarding as explicit real-hardware obligations.
- Narrow the kickoff specification where the probes disproved portability assumptions: identical paths are proven through Linux bind mounts but remain conditional across VirtioFS; `.localhost` requires proxy-bypass preflight; and generated Compose port reset requires the validated Compose capability/version.
- Verify links and commands mechanically, run repository quality gates, complete the packet, and close it only after the evidence remains green and cleanup leaves no owned resources.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] sprout check --all
- [x] Verify every command and artifact link in docs/architecture/substrate-spikes.md

## Validation evidence

- Fresh aggregate execution on Ubuntu 24.04.4 LTS, Linux 7.0.0-28-generic x86_64, from 2026-08-29T09:31:16Z through 09:36:24Z passed `git-path`, `compose-include`, `localhost-routing`, and the complete 16-case real-`SIGKILL` `failure-matrix`.
- Every retained root was inspected and then removed through the ownership-aware `abox-spike cleanup` command. Independent Docker container, network, and volume queries for `dev.abox.spike.owner` returned empty; every run-root absence assertion passed.
- A mechanical report validator found all 11 artifact links, all four registered scenario names, and the documented `run`, `inspect`, and `cleanup` command sequence.
- Exact matrix: Go 1.26.3; Git 2.43.0; container Git 2.47.3; Docker client/server 29.6.1; Compose 5.3.0; containerd 2.2.5; runc 1.3.6; overlay2/extfs; cgroup v2. `limactl` was unavailable and no guest image or Darwin runtime was exercised, so none is claimed.
- `make check` passed unit tests, race tests, vet, build, registry smoke, shell validation, push-guard validation, and Python automation tests.
- `go test -race -count=10 ./...` passed both Go packages. `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as artifact evidence only.
- `sprout check m0-substrate-report` and `sprout check --all` passed before completion.

## Outcome and follow-ups

- Published `docs/architecture/substrate-spikes.md` as the exact Linux support boundary, reproduction guide, probe matrix, failure record, cleanup proof, and final Darwin obligation list.
- Added ADR 0001 to accept the evidence-gated Linux mechanisms while making every red critical gate blocking and retaining Darwin VZ/VirtioFS/Lima behavior as unaccepted.
- Narrowed `THE_BEGINNING.md`: path invariance is conditional across VirtioFS, Compose port reset requires a validated pinned capability, and `.localhost` requires resolver/proxy-bypass preflight.
- Explicit deferred follow-ups remain `m0-virtiofs-probe`, `m0-lima-isolation-probe`, and `m5-darwin-arm64-hardware-gate`; broader normative ADRs remain in `m1-architecture-decisions`.

## Original request

Implement publish substrate evidence and lock architectural gates for Agent Box according to the cited specification and dependencies.
