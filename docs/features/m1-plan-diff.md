<!-- sprout-task
{"schema_version":1,"id":"m1-plan-diff","status":"implemented","created_at":"2026-08-29T06:48:36.078727449Z","implemented_at":"2026-08-29T21:49:13.112703731Z"}
-->

# Plan change and conflict classification

Status: implemented. Implementation record for `m1-plan-diff`.

Implemented a pure `internal/plandiff` policy and executable-Plan comparator.
Equal immutable Plan content takes a zero-allocation digest fast path; matching
digests with divergent content and unexplained divergent digests fail closed.
Changed Plans produce deterministically ordered no-op, in-place, restart,
growth, removal, or conflict results with stable reason codes and human text.

Dirty worktree observations gate base/branch replacement and repository removal
without exposing any mutation capability. Immutable Box/runtime/repository path
drift and disk shrink fail closed. The complete Section 9.4 policy is represented
as typed change facts so later route, setup, and secret Plan adapters reuse the
same tested behavior without prematurely weakening the v1alpha1 Plan schema.

## Scope and dependencies

- Milestone 1. Dependencies: m1-plan-compiler. Source: THE_BEGINNING.md Sections 9.4 and Issue 6.

## Behavior

- Compare current and desired Plans into no-op, in-place, restart, growth, removal, and conflict changes.
- Reject disk shrink and base-ref or branch replacement for dirty existing worktrees.
- Expose deterministic human and machine-readable change reasons.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Every reconcile-change row in Section 9.4 has a test.
- No-op classification is stable and allocation-free enough for the warm path target.
- Conflicts never mutate observed resources.

## Implementation details

- Preserve the executable Plan schema's explicit rejection of deferred routes,
  setup, ports, caches, and secret references. The first attempted approach of
  adding optional diff-only Plan fields failed the existing
  `TestPlanRejectsDeferredExecutableFields` contract and was discarded. Express
  the complete Section 9.4 policy as typed, value-only change facts so later
  Plan-field adapters can reuse it when those fields become executable, while
  current Plan comparison handles only fields the compiler actually emits.
- Implement a pure `internal/plandiff` boundary over immutable current/desired
  Plans plus a minimal read-only dirty-worktree observation. Return ordered,
  versionable class/reason/resource records; never invoke an adapter or expose a
  mutation callback from classification.
- Fast-path equal Plan digests to an explicit no-op result with no heap
  allocation. For changed Plans, compare resources by stable identity, sort the
  resulting reasons deterministically, and select the aggregate class by fixed
  safety/impact precedence.
- Treat VM CPU/memory and secret-consumer changes as restart work, disk increase
  as growth, disk shrink and immutable identity drift as conflicts, and clean
  repository removal as removal. Base/branch replacement and repository removal
  become conflicts when the corresponding observed worktree is dirty. Worktree
  dirt alone does not create runtime work.
- Cover every Section 9.4 row with table-driven tests, add a stable JSON golden,
  randomized ordering/property checks, a zero-allocation no-op assertion and
  benchmark, and assertions that conflict classification leaves all inputs
  byte-for-byte unchanged.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run Plan diff golden and property tests

## Validation evidence

- `TestClassifyCoversReconcileBehaviorTable`: PASS for all nine Section 9.4
  rows, including both clean/dirty checkout replacement and repository-removal
  branches. The table exercises stable class, reason, resource, and human
  description output without requiring deferred resources in the executable
  Plan schema.
- `TestCompareExecutablePlanChanges`: PASS for the current Plan projection:
  digest no-op, Compose in-place, CPU/memory restart, disk growth/shrink,
  clean/dirty checkout replacement, and clean/dirty repository removal.
- `TestCompareConflictDoesNotMutateInputs`: PASS after byte-for-byte JSON
  snapshots of current Plan, desired Plan, and dirty-worktree observations.
- `TestCompareNoOpAllocations`: PASS across 1,000 runs comparing distinct equal
  Plans. `BenchmarkCompareNoOp-16` measured `127.5 ns/op`, `0 B/op`, and
  `0 allocs/op` on Linux amd64 (Intel i5-12600K).
- `TestClassifyGoldenAndOrderingProperty`: PASS across 100 randomized fact
  orders with byte-identical JSON matching `internal/plandiff/testdata/diff.golden.json`.
  The focused Plan diff suite passed 100 repeated runs.
- The initial optional-field design was rejected by the existing
  `TestPlanRejectsDeferredExecutableFields` test. Those edits were removed; the
  final `make check` passed all packages and reported `0 issues` from the pinned
  golangci-lint, plus module verification, unit, race, vet, build, smoke, shell,
  and Python automation checks.
- `go test -race -count=10 ./...`: PASS across all 20 Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation
  evidence only; no Darwin runtime behavior is claimed.
- `git diff --check`: PASS.
