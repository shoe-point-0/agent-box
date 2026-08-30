<!-- sprout-task
{"schema_version":1,"id":"m1-plan-command","status":"implemented","created_at":"2026-08-29T06:55:50.479423731Z","implemented_at":"2026-08-30T00:17:33.61517398Z"}
-->

# Render a read-only reconciliation plan

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Render a read-only reconciliation plan

## Context

- Milestone 1 command. Dependencies: m1-plan-diff, m1-daemon-api, m1-cli-foundation, m1-repository-resolver. Source: THE_BEGINNING.md Section 14.1.

## Requirements

- Implement abox plan with human and JSON proposed operations based on read-only resolution and observed state.
- Expose stable conflicts and restart/destructive implications without materializing desired resources.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Running plan from an absent Box creates no private store, worktree, resource row, VM, port, or generated Box file.
- [x] Repeated plan output is deterministic for unchanged facts.
- [x] JSON validates against the output schema.

## Technical approach

- Add a versioned, bounded `POST /v1/boxes:plan` read endpoint. The request
  carries only canonical manifest and Box identity; unlike `up`, it has no
  idempotency key, operation journal, lease, or mutation runner.
- Compose a read-only planner from the strict manifest loader, hardened Machine
  Git observer, repository resolver, immutable Plan compiler, state read
  transaction, canonical current-Plan reader, and pure Plan diff classifier.
  Keep every materializer, runtime apply adapter, port allocator, and state
  update capability out of this dependency graph.
- Return a strict `abox.plan-output/v1alpha1` envelope containing the desired
  immutable Plan, aggregate classification, and stable ordered proposed
  operations. Operations explicitly flag restart, destructive, and conflict
  implications. An absent Box emits deterministic create operations; an
  existing Box compares its retained current Plan and live dirty-worktree
  observation before classifying changes.
- Derive Milestone 1's allowed local source root from the canonical workspace
  root containing `.abox/box.yaml`, construct all future Box paths without
  creating them, and use exact configured tool facts and platform driver as
  Plan inputs. Reject malformed identity, missing/corrupt retained Plans, dirty
  source intent, and unsupported manifests before returning output.
- Cover absent/no-op/restart/destructive/conflict paths, strict transport,
  deterministic human and JSON rendering, schema validation, and before/after
  snapshots proving no Box rows, private store, worktree, runtime, port, or
  generated Box artifacts are created.

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

## Validation evidence

- `make check`: PASS on Linux. The pinned linter reported `0 issues`; module
  verification, unit tests, race tests, vet, builds, spike smoke, shell checks,
  and Python automation checks all passed.
- `go test -race -count=10 ./...`: PASS across all 28 Go packages, including
  repeated real-Git planner tests and daemon/client transport tests.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation
  evidence only; no Darwin runtime behavior is claimed.
- `TestCompileAbsentBoxIsDeterministicAndCreatesNothing`: PASS. Two real-Git
  compilations were deeply identical, validated against
  `plan-output.schema.json`, left the source tree byte-for-byte unchanged, and
  did not create the configured application root.
- `TestCompileExistingBoxExposesRestartAndDestructiveConflicts`: PASS. A retained
  Plan plus live clean worktree exposed CPU change as restart work; subsequent
  disk shrink and dirty checkout replacement produced stable destructive
  conflicts without changing the dirty file.
- Live isolated Linux daemon/CLI smoke: PASS. Two `abox plan --json TASK-31`
  invocations were byte-identical and returned four absent-Box create
  operations. Git `2.43.0`, Docker `29.6.1`, Compose `5.3.0`, and a disposable
  read-only `limactl --version` fixture (`2.0.0`) supplied tool facts; no Lima or
  Docker apply command ran.
- Before and after the live smoke, the source fixture hash was
  `e04bc0ce341e2b6dffa1454bf807db913a17d7ab3fa98903abb457217ed4cce7`
  and `git status --porcelain` was empty. SQL counts remained zero for
  `workspaces`, `boxes`, `box_generations`, `repositories`, `worktrees`,
  `runtime_hosts`, `compose_projects`, and `operations`; Box and repository
  state directories were absent. The owned daemon, socket, state, fixture, and
  fake-tool directory were removed afterward.
- `git diff --check`: PASS.

## Outcome and follow-ups

Implemented `abox plan` as a bounded read-only daemon request with deterministic
human and schema-valid JSON output. The planner composes only strict loading,
hardened Git observation, path construction, immutable Plan compilation,
read-only state lookup, retained-Plan verification, and pure diff
classification. It has no materializer, reconciliation runner, Lima mutation,
Compose apply, port allocator, journal write, or state-update capability.

Absent Boxes produce explicit create proposals without materialization.
Existing Boxes expose no-op/in-place/growth/removal/restart/conflict classes and
mark restart, destructive, and conflict implications independently. Retained
Plan files and repository paths are constrained to the exact Box ownership root
before observation; symlinked or mismatched retained paths are refused.

## Original request

Implement render a read-only reconciliation plan for Agent Box.
