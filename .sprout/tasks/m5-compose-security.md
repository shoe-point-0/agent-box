<!-- sprout-task
{"schema_version":1,"id":"m5-compose-security","status":"open","created_at":"2026-08-29T06:48:36.581753466Z"}
-->

# Enforce unsafe Compose policy boundaries

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Enforce unsafe Compose policy boundaries

## Context

- Milestone 5 adversarial hardening. Dependencies: m1-compose-policy-core, m2-multi-repo-compose, m4-ownership-proofs. Source: THE_BEGINNING.md Sections 16.2 and 16.3.

## Requirements

- Expand adversarial coverage of the already-enforced normalized Compose deny policy, source-mount ownership diagnostics, runtime inspection, and threat-model traceability.
- Validate normalized Compose reality, not only source YAML.
- Ensure workspace services run as compatible non-root users without blindly overriding support-service identity.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Every forbidden escape primitive has a fixture and stable error.
- [ ] No service receives the guest Docker socket by default.
- [ ] Ownership-changing source-mount behavior fails diagnostics before READY.

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
- [ ] Run adversarial normalized-Compose policy tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement enforce unsafe compose policy boundaries for Agent Box according to the cited specification and dependencies.
