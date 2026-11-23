# Plan change and conflict classification

Status: implemented.

The `internal/plandiff` package provides a pure, mutation-free policy engine
and executable-Plan comparator consumed by [`abox plan`](abox-plan.md)
and reconciliation planning. Equal immutable Plans follow a zero-allocation
digest fast path (while failing closed on mismatched content for matching
digests or unexplained digest divergence), and changed Plans yield
deterministically ordered no-op, in-place, restart, growth, removal, or
conflict records with stable reason codes and human descriptions.

The full `THE_BEGINNING.md` Section 9.4 policy matrix is modeled as typed,
value-only change facts so future route, setup, port, cache, and secret
adapters reuse the same tested classification rules without weakening the
strict `v1alpha1` executable Plan schema.

## Scope and dependencies

- Milestone 1 Plan diff engine. Depends on [Plan compiler](plan-compiler.md). Source: `THE_BEGINNING.md` Section 9.4 and Issue 6.

## Behavior

- Classifies current versus desired immutable Plans plus read-only dirty-worktree observations into ordered `no-op`, `in-place`, `restart`, `growth`, `removal`, or `conflict` results with deterministic safety/impact aggregate precedence.
- Fast-paths equal Plan digests to an allocation-free no-op while failing closed on digest/content inconsistency.
- Classifies VM CPU/memory and secret-consumer changes as `restart`, disk increases as `growth`, clean repository removals as `removal`, and disk shrink or immutable Box/runtime/repository path drift as `conflict`.
- Elevates base-ref/branch replacement and repository removal to `conflict` whenever the observed worktree is dirty, while ensuring worktree dirt alone does not trigger runtime work.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Classification is strictly read-only and never mutates input Plans, observations, or external resources.

## Verified acceptance criteria

- Every Section 9.4 reconciliation-change row is covered by table-driven tests.
- No-op digest comparison performs zero heap allocations on the warm path.
- Conflict classification leaves all input Plans and observations byte-for-byte unchanged.

## Implementation details

- `internal/plandiff` separates executable-Plan extraction (handling only fields emitted by `internal/plan`, preserving `TestPlanRejectsDeferredExecutableFields`) from the typed Section 9.4 policy classifier so later milestones plug into the same classifier without schema compromises.
- Change reasons are sorted deterministically by resource identity and stable reason code before JSON serialization and aggregate precedence selection.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run Plan diff golden and property tests

## Validation evidence

- `TestClassifyCoversReconcileBehaviorTable` passed for all nine Section 9.4 rows, including both clean and dirty branches for checkout replacement and repository removal.
- `TestCompareExecutablePlanChanges` passed across digest no-op, Compose in-place, CPU/memory restart, disk growth/shrink, clean/dirty checkout replacement, and clean/dirty repository removal; `TestCompareConflictDoesNotMutateInputs` confirmed byte-identical JSON input snapshots before and after conflicts.
- `TestCompareNoOpAllocations` passed across `1,000` runs; `BenchmarkCompareNoOp-16` measured `127.5 ns/op`, `0 B/op`, and `0 allocs/op` on Linux amd64 (Intel i5-12600K).
- `TestClassifyGoldenAndOrderingProperty` passed `100` randomized fact orderings matching `internal/plandiff/testdata/diff.golden.json`, and the focused diff suite passed `100` repeated runs.
- `make check` (`0` lint issues across 20 packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
