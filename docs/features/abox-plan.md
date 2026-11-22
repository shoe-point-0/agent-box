<!-- sprout-task
{"schema_version":1,"id":"m1-plan-command","status":"implemented","created_at":"2026-08-29T06:55:50.479423731Z","implemented_at":"2026-08-30T00:17:33.61517398Z"}
-->

# Read-only reconciliation plans with abox plan

Status: implemented.

The `abox plan` command and `POST /v1/boxes:plan` daemon endpoint compute
deterministic, strictly read-only reconciliation plans in human and
schema-validated JSON (`abox.plan-output/v1alpha1`) formats. The planner
composes only strict manifest loading, hardened Git observation, repository
resolution, immutable Plan compilation, read-only state queries, retained-Plan
verification, and pure diff classification—with zero materializers, runtime
apply adapters, port allocators, journal writes, or state mutations in its
dependency graph.

For an absent Box, `abox plan` emits deterministic create proposals without
creating any directories, Git stores, database rows, or runtime resources. For
an existing Box, it verifies the retained `plan.json` and observes live dirty
worktree state under the Box ownership root to classify no-op, in-place,
growth, removal, restart, and conflict operations independently.

## Scope and dependencies

- Milestone 1 planning command. Depends on [Plan diff](plan-diff.md), [daemon API](daemon-api.md), [CLI transport](cli-transport.md), and [repository resolver](git-repository-resolver.md). Source: `THE_BEGINNING.md` Section 14.1.

## Behavior

- Exposes `POST /v1/boxes:plan` as a bounded read-only endpoint carrying canonical manifest and Box identity with no idempotency key, operation journal entry, lease, or mutation runner.
- Emits a strict `abox.plan-output/v1alpha1` envelope containing the compiled immutable Plan, aggregate classification, and deterministically ordered proposed operations with explicit `restart`, `destructive`, and `conflict` flags.
- Constrains retained Plan files and observed repository paths to the exact Box ownership root, refusing symlinks, mismatched retained paths, corrupt retained Plans, dirty source intent, and unsupported manifests before returning output.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Planning never materializes private stores, worktrees, SQLite rows, VMs, ports, or generated Box artifacts.

## Verified acceptance criteria

- Running `abox plan` for an absent Box creates no private store, worktree, database row, VM, port, or generated Box file.
- Repeated `abox plan` invocations for unchanged facts produce byte-identical output.
- JSON output validates against `plan-output.schema.json`.

## Implementation details

- The read-only planner derives Milestone 1's allowed local source root from the canonical workspace root containing `.abox/box.yaml`, constructs future Box paths purely in memory via `internal/paths`, and supplies exact configured tool facts and platform driver inputs to `internal/plan`.
- When a Box exists, the planner reads the retained `plan.json` within the Box root, checks live worktree dirtiness through hardened read-only Git observation, and classifies differences with `internal/plandiff`.
- Because no mutating adapters or write transactions are wired into the planner, absent-Box and conflict paths cannot create side effects by construction.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `TestCompileAbsentBoxIsDeterministicAndCreatesNothing` passed with real Git: two compilations were deeply identical, validated against `plan-output.schema.json`, left the source repository byte-for-byte unchanged, and did not create the configured application root.
- `TestCompileExistingBoxExposesRestartAndDestructiveConflicts` passed: a CPU change on a clean worktree classified as restart work, and disk shrink or dirty checkout replacement produced stable destructive conflicts without modifying the dirty file.
- Live isolated Linux daemon/CLI smoke ran two `abox plan --json TASK-31` invocations (using Git `2.43.0`, Docker `29.6.1`, Compose `5.3.0`, and a read-only `limactl --version` fixture reporting `2.0.0`) and produced byte-identical output with four absent-Box create operations; no Lima or Docker apply command ran.
- Before and after the live smoke, the source fixture hash (`e04bc0ce341e2b6dffa1454bf807db913a17d7ab3fa98903abb457217ed4cce7`) and clean `git status --porcelain` were unchanged, SQLite counts remained `0` across all eight tables (`workspaces`, `boxes`, `box_generations`, `repositories`, `worktrees`, `runtime_hosts`, `compose_projects`, `operations`), and no Box or repository directories were created.
- `make check` (`0` lint issues), `go test -race -count=10 ./...` across all 28 packages, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
