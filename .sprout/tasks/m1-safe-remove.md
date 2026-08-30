<!-- sprout-task
{"schema_version":1,"id":"m1-safe-remove","status":"open","created_at":"2026-08-29T06:48:36.249156679Z"}
-->

# Implement refusal-first Box removal

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement refusal-first Box removal

## Context

- Milestone 1. Dependencies: m1-git-publication-status, m1-ownership-core, m1-operation-journal, m1-reconcile-engine, m1-compose-adapter, m1-lima-adapter. Source: THE_BEGINNING.md Sections 10.8 and 14.1.

## Requirements

- Check canonical containment, Git registration, branch, dirtiness including ignored/untracked files, unpublished commits, and resource ownership.
- Require separate explicit discard flags plus --yes for each applicable non-interactive destructive condition.
- Remove in dependency order while retaining enough state to diagnose partial cleanup.
- Complete a read-only preflight before mutation, hold the exclusive Box lease, quiesce Box writers, re-observe every safety fact immediately before each destructive step, and bind confirmation to a digest of the exact loss inventory.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Default removal refuses dirty or unpublished work and leaves every file and resource intact.
- [ ] Explicit removal lists exact losses before confirmation and touches only the selected Box.
- [ ] Rerunning after interruption safely converges.
- [ ] Any drift in dirtiness, unpublished commits, inode identity, labels, or volume policy after preview forces a fresh refusal or confirmation before mutation.

## Technical approach

- Add a focused `internal/removal` coordinator above the existing Git, Lima,
  Compose, ownership, journal, and reconcile boundaries. Leaf steps remain
  responsible for producing freshly verified registration, branch, status,
  publication, inode, label, and volume-policy facts; the coordinator owns
  refusal policy and dependency ordering rather than duplicating adapters.
- Model removal as a read-only `Preview` followed by an authorized `Execute`.
  Canonicalize and hash the complete ordered resource/loss inventory, require
  `--yes` semantics plus separate dirty-worktree, unpublished-commit, and
  volume-discard grants, and never mutate during preview or authorization
  refusal.
- Acquire an exclusive Box lease and quiesce writers before execution. Reobserve
  the entire inventory after quiescence, then reobserve and compare each exact
  resource fact immediately before its destructive callback. Treat any change
  in identity evidence or losses as drift requiring a fresh preview.
- Execute only the explicit dependency-ordered resource steps, persist a
  before/after transition around each side effect, skip already-absent steps,
  and return completed/remaining resource keys on partial failure so a fresh
  preview and retry converges without replaying deletion blindly.
- Exercise policy, drift, partial-failure, lease/quiescence, durable-transition,
  and selected-target isolation behavior with adversarial temporary-root tests.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the refusal/confirmation/removal coordination boundary.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run destructive safety tests only inside isolated temporary roots

## Validation evidence

- `go test -race -count=10 ./internal/removal` passed ten repetitions of
  refusal, digest binding, drift, interruption/retry, dependency ordering,
  lease/quiescence, and durable before-event tests.
- The destructive isolation test operated only below `t.TempDir()`, removed the
  selected Box root, and byte-verified that an unrelated sibling file retained
  its original contents.
- `make check` passed: pinned lint reported `0 issues`; module tidiness and
  verification, all unit and race tests, vet, Linux builds, spike smoke, shell
  checks, and Python automation tests passed.
- `go test -race -count=10 ./internal/removal ./internal/worktree
  ./internal/machinegit ./internal/publication` passed. The real-Git worktree
  fixture proved a read-only removal observation reports tracked/untracked and
  ignored files (including newline-bearing names) without adding or relocking a
  worktree. The removal adapter test proved those paths and conservative
  publication commit IDs enter the digest-bound coordinator inventory and that
  default execution refuses before lease or remover calls.
- The task remains open: these results prove the coordinator slice, not a live
  `abox rm` command or concrete Git/Lima/Compose teardown integration.

## Outcome and follow-ups

- Added a fail-closed removal coordinator with canonical exact-loss previews,
  SHA-256 confirmation binding, separate discard grants, mandatory noninteractive
  confirmation, fenced lease/quiescence hooks, per-resource immediate
  reobservation, fixed teardown stages, durable before/after transitions, and
  partial-progress results for convergent retries.
- Added a hardened `git ls-files --others --ignored --exclude-standard -z`
  boundary, a mutation-free worktree observation API, and a worktree removal
  adapter that combines registration, branch, common-directory, inode, dirty,
  ignored-file, and trusted publication evidence into exact coordinator facts.
- Next slice must implement the adapter's concrete ownership-proved,
  interruption-safe Git remover, including the worktree-removed/branch-remains
  retry state. Compose, Lima, private-store, generated-state, daemon, and CLI
  integration plus disposable runtime teardown evidence remain required before
  any acceptance criterion can be checked.

## Original request

Implement implement refusal-first box removal for Agent Box according to the cited specification and dependencies.
