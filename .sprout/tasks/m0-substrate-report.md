<!-- sprout-task
{"schema_version":1,"id":"m0-substrate-report","status":"open","created_at":"2026-08-29T06:48:35.907245415Z"}
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

- [ ] Every Milestone 0 exit gate links to reproducible evidence.
- [ ] No support claim exceeds the tested OS, architecture, Lima, Git, Docker, and Compose matrix.
- [ ] Product implementation remains blocked when a critical substrate gate is red.
- [ ] Every Linux-exercisable critical probe passes before implementation proceeds, and no Darwin/macOS support claim exists until m5-darwin-arm64-hardware-gate passes on real hardware.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] sprout check --all
- [ ] Verify every command and artifact link in docs/architecture/substrate-spikes.md

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement publish substrate evidence and lock architectural gates for Agent Box according to the cited specification and dependencies.
