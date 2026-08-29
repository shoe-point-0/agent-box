<!-- sprout-task
{"schema_version":1,"id":"m5-darwin-arm64-hardware-gate","status":"open","created_at":"2026-08-29T07:29:33.35108351Z"}
-->

# Validate the complete Darwin arm64 Lima path on real hardware

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Validate the complete Darwin arm64 Lima path on real hardware

## Context

- Final pre-RC hardware gate. Dependencies: m0-virtiofs-probe, m0-lima-isolation-probe, m4-recovery-e2e, m5-artifact-build, m5-acceptance-matrix. Source: THE_BEGINNING.md Sections 19, 20, 24, and the Linux-first execution decision.

## Requirements

- Install the Darwin/arm64 artifact on minimum-supported and current macOS running Apple Silicon and execute the real Lima VZ, VirtioFS, Docker/Compose, routing, readiness, recovery, and cleanup paths.
- Consume Linux-developed acceptance fixtures and emit a machine-readable evidence bundle for every Darwin-specific support claim rather than redefining product behavior in this gate.
- Exercise cold install, doctor, up, prompt-capable exec, status, logs, down/up, failure diagnostics, and refusal-first removal from a clean Machine.

## Constraints

- No macOS or Apple Silicon support claim may be inferred from cross-compilation or emulation.
- Product defects found here are fixed in their owning implementation story and the complete hardware gate is rerun.

## Acceptance criteria

- [ ] The signed or test-signed Darwin/arm64 artifact installs and launches without Docker Desktop or manual Lima configuration.
- [ ] VZ virtualization, exact VirtioFS mounts, bidirectional source/Git visibility, static forwards, routes, ports, readiness, and context match observed reality.
- [ ] Crash/restart and removal scenarios preserve dirty or unpublished work and leave no owned VM, mount, listener, network, container, volume, process, socket, or temporary artifact behind.
- [ ] A tester can execute one documented agent prompt through abox exec and collect a redacted diagnostic bundle on failure.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] Run the complete Darwin/arm64 hardware acceptance manifest on minimum-supported macOS
- [ ] Run the complete Darwin/arm64 hardware acceptance manifest on current macOS
- [ ] Verify all retained evidence and post-run leak inventories

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Validate Agent Box on real Apple Silicon hardware only after the Linux implementation and release artifact are ready.
