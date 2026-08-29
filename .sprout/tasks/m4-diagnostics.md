<!-- sprout-task
{"schema_version":1,"id":"m4-diagnostics","status":"open","created_at":"2026-08-29T06:48:36.517682116Z"}
-->

# Produce redacted Box-scoped diagnostic bundles

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Produce redacted Box-scoped diagnostic bundles

## Context

- Milestone 4. Dependencies: m4-recovery-reconstruction, m1-context-document. Source: THE_BEGINNING.md Section 17.3.

## Requirements

- Build the bundle from typed allowlisted records for redacted Plan/model, versions, events, Lima config, container health, probes, scoped Git porcelain, and time-bounded daemon logs; never copy raw tool output wholesale.
- Exclude source content, diffs, secrets, credentials, volume contents, and unrelated Boxes.
- Create archives atomically with exclusive no-follow semantics and mode 0600, scanning archive headers, names, link targets, and decompressed content.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A failing fixture produces enough evidence to locate the failing boundary.
- [ ] Secret and unrelated-path canaries are absent in decompressed bundle bytes.
- [ ] Bundle generation is read-only with respect to Box resources.

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
- [ ] Run diagnostic content allowlist and canary-redaction tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement produce redacted box-scoped diagnostic bundles for Agent Box according to the cited specification and dependencies.
