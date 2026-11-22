<!-- sprout-task
{"schema_version":1,"id":"m0-spike-harness","status":"implemented","created_at":"2026-08-29T06:44:38.361259313Z","implemented_at":"2026-08-29T07:23:33.712761465Z"}
-->

# Substrate spike harness

Status: implemented.

The spike harness in `cmd/abox-spike` and `internal/spike` provides the shared
execution, diagnostics, append-only resource inventory, and refusal-first
cleanup framework used by all Milestone 0 substrate probes. It runs scenarios
inside isolated temporary roots without exposing a stable product API or making
platform support claims from simulated fixtures.

## Scope and dependencies

- Milestone 0 foundation. Depends on [spike workspace bootstrap](spike-workspace-bootstrap.md). Source: `THE_BEGINNING.md` Sections 19.2, 20, 21 Issue 1, and 25.

## Behavior

- Executes registered scenarios via `abox-spike run --parent <dir> <scenario>` and emits machine-readable `abox.spike.result/v1` JSON output while retaining the run root and command diagnostics.
- Provides read-only `abox-spike inspect <root>` to read persisted root identity, resource inventory, and final results without mutating disk.
- Provides refusal-first `abox-spike cleanup <root>` (`abox.spike.cleanup/v1`) that preflights ownership and removes only verified `created` path resources tied to the persisted root ID.

## Constraints

- The harness stays disposable and internal; it never claims platform support from simulated results.
- Teardown never deletes resources based on name prefixes alone and always verifies recorded identity and ownership metadata before mutating.

## Verified acceptance criteria

- A single command executes a selected spike and returns a machine-readable pass/fail result plus retained diagnostics.
- Interrupted runs leave an fsynced append-only inventory that `inspect` and `cleanup` can observe and safely reconcile.
- Unit tests prove unowned resources and paths outside the temporary root are refused before mutation.

## Implementation details

- Each run initializes a cryptographically random root identity, restrictive control files, an fsynced append-only NDJSON resource inventory, a diagnostics directory, and a final JSON result file.
- Scenarios receive an internal environment offering owned-file creation, inventory recording, and an argv-only bounded process runner (no shell-string execution).
- `cleanup` runs a complete preflight before any deletion: it authorizes only resources bound to the persisted root ID, refuses planned-only, unknown, outside-root, or unsupported resource kinds, and removes recorded paths deepest-first without recursive deletion.
- Scenario-specific external cleanup (for Git, Compose, or browser profiles) and deterministic kill-point injection are layered by the probe scenarios and [failure-matrix spike](spike-failure-matrix.md). A clearly labeled `harness-smoke` scenario exercises the harness itself without making substrate claims.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the harness against a temporary-root smoke fixture and inspect its resource inventory

## Validation evidence

- `make check` and `go test -race -count=10 ./...` passed across `cmd/abox-spike` and `internal/spike`, including JSON registry smoke output.
- A live `harness-smoke` run produced `abox.spike.result/v1` with status `passed`, retained its root, and recorded eight ordered planned/created inventory entries plus `go version` diagnostics.
- `abox-spike inspect <root>` read the persisted root identity, inventory, and result without mutation; `abox-spike cleanup <root>` returned `abox.spike.cleanup/v1` and removed the owned root completely.
- Unit tests covered bounded stdout/stderr capture, literal argv handling, cancellation, failed-run retention, interrupted-run reopen, corrupt inventories, symlink roots, unsupported resource kinds, outside-root records, unknown in-root files, and exclusive-write conflicts.
