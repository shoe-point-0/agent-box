<!-- sprout-task
{"schema_version":1,"id":"m1-reconcile-engine","status":"implemented","created_at":"2026-08-29T06:48:36.162350553Z","implemented_at":"2026-08-29T22:27:29.168605862Z"}
-->

# Operation dependency graph engine

Status: implemented.

The `internal/reconcile` package provides a deterministic operation-DAG
execution engine over the Section 9.2 `Observe` / `Apply` / `Compensate`
contract. It validates the full graph before calling any operation, always
observes live external state before mutating, executes bounded topological
batches, serializes runs on an engine instance, records durable before/after
journal events around every external side effect, and compensates only
resources created by the current failed attempt in reverse completion order
using a fresh timeout-bounded cleanup context.

Box-wide cross-process exclusion remains enforced by the caller's fenced
[operation journal](operation-journal.md) lease, as wired in
[`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 reconciliation engine. Depends on [operation journal](operation-journal.md) and [Plan diff](plan-diff.md). Source: `THE_BEGINNING.md` Sections 9.2 and 9.3.

## Behavior

- Validates stable operation keys, duplicate keys, missing dependencies, and dependency cycles before invoking `Observe` or any mutating callback.
- Executes deterministic topological batches within a configured positive fan-out bound while serializing concurrent executions on one engine instance.
- Re-observes every operation on every execution rather than inferring readiness from prior journal events, so recovery after an unknown partial `apply-started` state inspects external reality before deciding whether to apply.
- Persists typed before/after events in `internal/journal` around every `Apply` and `Compensate` call (giving each after-event its own explicit timeout so request cancellation or cleanup exhaustion does not suppress audit events); stops before mutation if persisting a before-event fails, and marks failed after-event persistence as unknown execution requiring re-observation.
- Compensates only operations whose `Result.Created` flag is true in the current `Execute` attempt, in reverse completion order under a fresh bounded cleanup context—never touching pre-existing or observed-only resources.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Callers must hold the Box's fenced journal lease before executing a mutating DAG; engine fan-out applies only within one leased operation.

## Verified acceptance criteria

- Dependency cycles and missing dependencies fail before `Observe` or `Apply`.
- Restart after an unknown partial execution re-observes external state rather than replaying mutations blindly.
- Compensation affects only resources created by the current failed attempt and preserves reverse completion order.

## Implementation details

- `internal/reconcile` tracks exact per-attempt completion order across parallel batches so rollback runs deterministically in reverse order even after concurrent fan-out.
- After-events for both `Apply` and `Compensate` use dedicated bounded timeouts independent of the caller's request or cleanup context, ensuring terminal audit records persist even on cancellation.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run race and deterministic failure-injection tests for the DAG engine

## Validation evidence

- `go test -race -count=10 ./internal/reconcile` and `go test -race -count=50 ./internal/reconcile` passed across graph validation, unknown-partial restart re-observation, bounded fan-out, serialized concurrent runs, before-event recorder failure refusal, current-attempt-only reverse-order compensation, dedicated after-event timeouts under cancellation, and bounded cleanup.
- SQLite-backed journal integration tests verified ordered `reconcile.apply-started` and `reconcile.apply-succeeded` events carrying secret-free stable metadata.
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed.
