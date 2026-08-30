<!-- sprout-task
{"schema_version":1,"id":"m1-single-repo-up","status":"open","created_at":"2026-08-29T06:48:36.203491553Z"}
-->

# Deliver the single-repository up vertical slice

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Deliver the single-repository up vertical slice

## Context

- Milestone 1. Dependencies: m1-machine-preflight, m1-git-worktrees, m1-lima-adapter, m1-filesystem-verifier, m1-compose-policy-core, m1-compose-adapter, m1-context-document, m1-reconcile-engine, m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Sections 9.3, 14.1, and Milestone 1.

## Requirements

- Wire lease, validation, Git, Plan, runtime host, mounts, Compose, basic readiness, context, and READY transition for one local repository.
- Re-observe each boundary and resume matching partial resources.
- Return success only after the declared workspace service is usable.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] A clean fixture reaches READY from zero with abox up demo.
- [ ] A second up is a measured no-op with no duplicate worktree, VM, network, volume, container, or setup effect.
- [ ] Killing the CLI does not kill or duplicate daemon-owned reconciliation.

## Technical approach

- Add a focused `internal/up` coordinator above the existing daemon journal and
  reconcile engine. The daemon operation context, rather than the attached CLI
  request, owns the complete attempt. Acquire and periodically renew one fenced
  Box lease around every DAG execution, cancel mutation immediately if renewal
  is lost, and release only the exact still-owned fence through a bounded
  cleanup context.
- Assemble the existing validation, repository/private-store/worktree, Plan,
  Lima, filesystem, Compose, readiness, context, state, and removal-snapshot
  boundaries as independently observable reconcile operations. Keep provider
  facts inside each operation and journal only stable operation keys and
  created/no-op transitions.
- Make READY publication the final operation, after live workspace-service and
  filesystem readiness. Atomically retain the Plan/current generation, cached
  component observations, context identity, and complete removal ownership;
  never infer external existence from SQLite on a later run.
- Prove the control-plane contract first with real SQLite/journal execution and
  deterministic stage fixtures: cold execution mutates each stage once, warm
  execution re-observes every stage without mutation, lease loss cancels the
  DAG, and detaching the client leaves daemon-owned execution running. Then
  assemble the unchanged concrete adapters and exercise the disposable
  single-repository smoke fixture without claiming unavailable Lima/VZ support.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run the real single-repository cold and warm up smoke test

## Validation evidence

- Control-plane slice: `go test -race -count=20 ./internal/up` passed. The new
  coordinator executed a nine-stage dependency chain under the real
  SQLite-backed journal lease, re-observed all nine stages on a second attempt,
  and recorded zero second-attempt applies. Every stage applied exactly once
  across cold and warm execution.
- The focused suite held a 60 ms lease across a 90 ms operation with at least
  three successful renewals and one exact-fence release. Injected renewal loss
  canceled the executor and performed zero releases because ownership could no
  longer be proven.
- `go vet ./internal/up`, `make lint` (`0 issues`), and `git diff --check`
  passed for the control-plane slice.
- READY-state slice: `go test -race -count=20 ./internal/state` passed after
  adding schema v4 and `Store.PublishReady`. The transaction commits the
  immutable Plan/current generation, context path and digest, freshly observed
  repository/worktree, Lima and Compose caches, and the complete removal
  ownership snapshot together. Repeating an identical publication is a no-op;
  attempts to rebind one generation are refused.
- The READY-state suite injected a unique Git-store cache collision after the
  generation insert point. SQLite rolled back the generation, repository, and
  removal-authority rows and left the Box at generation zero in `planning`.
- Post-slice `make check` passed: lint reported `0 issues`, module verification,
  all unit and race tests, vet, builds, smoke, and automation tests were green.
- Attempt-preparation slice: `go test -race ./internal/up ./internal/state`,
  `go vet ./internal/up ./internal/state`, and `git diff --check` passed. The
  daemon can now reconstruct the selected Box intent from SQLite after request
  detachment, recompile the immutable Plan through the read-only planning
  boundary, derive only centralized Box paths, and select generation 1 only
  when no current generation exists. An exact warm Plan reuses its persisted
  generation and complete removal authority; a current generation with missing
  or drifted authority fails closed with a stable ownership/identity conflict.
- Post-preparation `make check` passed: lint reported `0 issues`; module
  verification, all unit and race tests, vet, builds, smoke, shell checks, and
  Python automation tests were green. `sprout check m1-single-repo-up` passed.
- Partial Git-authority slice: schema v5 adds one generation-scoped,
  single-repository recovery record tied to an actual daemon `up` operation.
  `SaveReconciliationGitAuthority` records intent before apply and permits only
  monotonic addition of the exact private-store identity and locked-worktree
  identity. A later daemon operation can continue the same immutable claim;
  drift, proof replacement, and proof regression fail closed.
- `go test -race ./internal/state`, `go vet ./internal/state`, and
  `git diff --check` passed for the partial-authority slice. The tests exercise
  claim/store/worktree progression, cross-operation recovery, drift and
  regression refusal, schema migration from every retained fixture, and atomic
  retirement of matching partial authority during READY publication.
- Post-authority `make check` passed: lint reported `0 issues`; module
  verification, all unit and race tests, vet, builds, smoke, shell checks, and
  Python automation tests were green. `sprout check m1-single-repo-up` passed.
- This evidence does not yet satisfy the cold/warm Box acceptance criteria: the
  prepared attempt is not yet consumed by concrete Git, Lima, filesystem,
  Compose, context, and READY operations in `cmd/aboxd`. The final READY
  transaction exists but is not wired to the filesystem readiness gate, and no
  runtime-host support claim is made.

## Outcome and follow-ups

The first coherent slice adds `internal/up` as the daemon-owned fenced lifetime
around one reconciliation DAG. It validates command input, emits stable start
and ready events, continuously renews the Box lease, cancels immediately on
lease loss, and releases only a still-proven fence through cancellation-proof
bounded cleanup. Errors cross the daemon boundary only as stable redacted
contract codes.

The second coherent slice adds one atomic READY publication boundary. It
retains context identity and complete removal ownership with the current Plan,
updates only cached component observations, enforces monotonic generations,
and supports exact warm retries without allowing immutable generation evidence
to drift.

The third coherent slice adds the read-only front half of production attempt
assembly. It deliberately refuses to infer ownership from deterministic names,
paths, or cached component rows. Cold preparation is allowed only at generation
zero; warm preparation requires the exact current-generation removal snapshot.

The fourth coherent slice adds durable, monotonic partial authority for the
private Git store and linked worktree. It is intentionally retired only inside
the same transaction that publishes complete READY removal authority, so a
failed publication retains the recovery record and a successful publication
cannot leave two competing authority sources.

Next, consume `up.PreparedAttempt` and the partial-authority store in concrete
private-store and worktree reconcile operations. Persist the claim before
mutation and each freshly verified identity immediately after its apply, then
prove restart recovery against real Git without adopting a path from its name
alone. Then add Lima, filesystem, Compose, context, and READY
operations and wire the completed runner into `cmd/aboxd`. Keep the packet open
until the unchanged single-repository fixture passes through the real daemon
and CLI, including detach and measured warm no-op assertions.

## Original request

Implement deliver the single-repository up vertical slice for Agent Box according to the cited specification and dependencies.
