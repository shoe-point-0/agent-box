<!-- sprout-task
{"schema_version":1,"id":"m5-rc-acceptance","status":"open","created_at":"2026-08-29T06:48:36.700485609Z"}
-->

# Gate v0.1.0-rc.1 on clean Machines

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Gate v0.1.0-rc.1 on clean Machines

## Context

- Milestone 5 aggregate gate. Dependencies: m4-recovery-e2e, m5-secret-delivery, m5-compose-security, m5-security-model, m5-toolchain-bootstrap, m5-state-compatibility, m5-observability-performance, m5-artifact-build, m5-sbom-provenance-signing, m5-release-operations, m5-release-supply-chain, m5-installation-docs, m5-governance-readiness, m5-acceptance-matrix, m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 19, 20 Milestone 5, and 24.

## Requirements

- Execute the versioned acceptance manifest containing all 22 mandatory E2E scenarios, nine hard guarantees, migrations, destructive safety review, secret review, benchmarks, installation, and artifact verification.
- Run on minimum supported and current macOS releases using dedicated Apple Silicon hardware.
- Publish exact support matrix, known failures, and retained redacted evidence.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every hard guarantee in Section 4.2 is backed by an acceptance assertion.
- [ ] No leaked owned resource remains after the suite.
- [ ] The release candidate is blocked by every safety, security, destructive, compatibility, support-claim, or missing-evidence failure; only explicitly informational performance observations may be non-blocking.

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
- [ ] Run the complete clean-Machine v0.1.0-rc.1 acceptance suite

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement gate v0.1.0-rc.1 on clean machines for Agent Box according to the cited specification and dependencies.
