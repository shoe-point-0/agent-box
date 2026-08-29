<!-- sprout-task
{"schema_version":1,"id":"m1-guest-toolchain","status":"open","created_at":"2026-08-29T06:48:36.107508706Z"}
-->

# Provision and verify the guest toolchain

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Provision and verify the guest toolchain

## Context

- Milestone 1. Dependencies: m1-lima-adapter, m1-toolchain-contract. Source: THE_BEGINNING.md Sections 6.2, 12, 16.5, and 22.3.

## Requirements

- Consume an already verified preinstalled or test-fixture artifact set for the guest image, Docker Engine, Compose, and required utilities; do not download implicit defaults.
- Provision rootful Docker inside the VM without exposing its socket to workspace containers.
- Verify runtime versions, cgroup/storage/network prerequisites, daemon readiness, and tool integrity.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A fresh runtime host reaches a verified Docker/Compose-ready state.
- [ ] Wrong versions or digest mismatches fail before project code executes.
- [ ] Provisioning is idempotent and diagnostics identify the failing layer.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run fresh and warm guest-toolchain provisioning on supported CI

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement provision and verify the guest toolchain for Agent Box according to the cited specification and dependencies.
