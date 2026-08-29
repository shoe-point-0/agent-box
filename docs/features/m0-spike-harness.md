<!-- sprout-task
{"schema_version":1,"id":"m0-spike-harness","status":"implemented","created_at":"2026-08-29T06:44:38.361259313Z","implemented_at":"2026-08-29T07:23:33.712761465Z"}
-->

# Build the disposable substrate spike harness

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Build the disposable substrate spike harness

## Context

- Milestone 0 foundation. Dependencies: m0-project-bootstrap. THE_BEGINNING.md sections 19.2, 20, 21 Issue 1, and 25 define the required evidence.

## Requirements

- Create a disposable command registry, bounded argv runner, append-only resource inventory, retained diagnostics, and temporary-root containment.
- Let each probe own scenario-specific Git, Lima, Compose, mount, and forwarding records; add the shared deterministic kill-point protocol only in m0-failure-matrix after those operations exist.
- Make the harness runnable against an isolated temporary root without establishing a stable product API.

## Constraints

- Do not create product abstractions or claim platform support from simulated results.
- Never delete a resource based only on a name prefix; teardown must verify recorded identity and ownership metadata.

## Acceptance criteria

- [x] A developer can run one command to execute a selected spike and receive a machine-readable pass/fail result plus retained diagnostics.
- [x] Interrupted runs leave an inventory that a cleanup command can inspect and safely reconcile.
- [x] Unit tests prove unowned resources and paths outside the temporary root are never removed.

## Technical approach

- Give every run a cryptographically random persisted root identity, restrictive control files, an fsynced append-only NDJSON resource inventory, a diagnostics directory, and one final JSON result.
- Pass scenarios an internal environment that exposes owned-file creation, resource recording, and a bounded argv-only runner; do not expose shell-string execution or stable product APIs.
- Retain run roots and diagnostics by default. `inspect` reads them without mutation; `cleanup` performs a complete preflight, accepts only resources tied to the persisted root identity, refuses unsupported kinds or unknown filesystem entries, and removes recorded paths deepest-first without recursive deletion.
- Compile one explicitly non-substrate `harness-smoke` scenario to exercise the execution path without making a Lima, Git, Compose, filesystem-coherency, or platform support claim.
- Keep deterministic kill-point injection out of this slice; `m0-failure-matrix` adds it after real probe operations exist.

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
- [x] Run the harness against a temporary-root smoke fixture and inspect its resource inventory

## Validation evidence

- `make check` passed package tests, race detector, vet, artifact-free build, and JSON registry smoke output.
- `go test -race -count=10 ./...` passed for both `cmd/abox-spike` and `internal/spike`.
- A real `harness-smoke` run returned `abox.spike.result/v1` with status `passed`, retained its root, and produced eight ordered planned/created inventory entries plus `go version` JSON/stdout/stderr diagnostics.
- `abox-spike inspect <root>` returned the persisted root identity, complete inventory, and final result without mutation.
- With explicit user consent, `abox-spike cleanup <root>` returned `abox.spike.cleanup/v1`; the owned run root no longer existed afterward.
- Tests cover bounded independent stdout/stderr, literal argv handling, cancellation, failed-run retention, interrupted-run reopen, corrupt inventories, symlink roots, unsupported resource kinds, outside-root records, unknown in-root files, and failed exclusive writes against pre-existing files.

## Outcome and follow-ups

- Delivered internal JSON contracts for registry listing, run results, inspection, cleanup, root identity, and append-only inventory.
- Delivered an argv-only bounded runner, retained command diagnostics, fsynced inventory/control files, explicit root ownership, and preflight-before-mutation cleanup using non-recursive removal.
- Cleanup authorizes only `created` path resources bound to the persisted root ID; planned-only, unknown, outside-root, and unsupported resources cause refusal before mutation.
- Added only the clearly labeled `harness-smoke` fixture. No Git, Lima, Compose, VirtioFS, routing, or platform capability is claimed.
- Next stories add real substrate probes; scenario-specific external resource cleanup remains intentionally owned by those stories.

## Original request

Build the disposable substrate spike harness described by the implementation kickoff specification.
