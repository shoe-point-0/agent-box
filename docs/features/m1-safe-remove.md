<!-- sprout-task
{"schema_version":1,"id":"m1-safe-remove","status":"implemented","created_at":"2026-08-29T06:48:36.249156679Z","implemented_at":"2026-08-30T05:45:13.364027059Z"}
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

- [x] Default removal refuses dirty or unpublished work and leaves every file and resource intact.
- [x] Explicit removal lists exact losses before confirmation and touches only the selected Box.
- [x] Rerunning after interruption safely converges.
- [x] Any drift in dirtiness, unpublished commits, inode identity, labels, or volume policy after preview forces a fresh refusal or confirmation before mutation.

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
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

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
- `go test -race -count=10 ./internal/worktree ./internal/removal
  ./internal/machinegit ./internal/publication` passed after adding the concrete
  Git remover. Disposable real-Git fixtures proved that an exactly observed,
  locked worktree containing untracked and ignored files is removed with its
  expected generated branch while its private store remains; an injected
  branch-delete interruption leaves an observable branch-only state that a
  fresh retry removes; and replacing the registered path after observation
  refuses removal without changing the replacement or generated branch.
- `make check` passed after the concrete Git removal slice: pinned lint reported
  `0 issues`; all unit and race tests, vet, Linux builds, spike smoke, shell
  checks, and Python automation tests passed. `sprout check m1-safe-remove`,
  `sprout check --all`, and `git diff --check` also passed.
- `go test -race -count=10 ./internal/removal ./internal/compose` passed after
  adding the concrete Compose removal step. The digest-bound preview includes
  exact container, network, volume, ownership-label, and explicit volume-delete
  policy evidence; default execution refused the exact volume loss before any
  lease or runtime removal call; changed ownership labels, volume inventory,
  and unrecorded replacement IDs refused before mutation.
- `go test -run '^TestRuntimeLifecycleAgainstRealDocker$' -v
  ./internal/compose` passed against the live Linux Docker boundary in 12.40s.
  The fixture proved an ownership-label forgery leaves the project intact, then
  injected a container-only partial teardown and proved a fresh removal deletes
  only the remaining recorded container, network, and volume set.
- `make check` passed after the Compose slice: pinned lint reported `0 issues`;
  module verification, all unit and race tests, vet, Linux builds, spike smoke,
  shell checks, and Python automation tests passed. `sprout check
  m1-safe-remove`, `sprout check --all`, and `git diff --check` also passed.
- `go test -race -count=10 ./internal/lima ./internal/removal` passed after
  adding the concrete Lima removal step. The typed adapter fixture proved the
  read-only preview accepts only the immutable Plan config plus exact persisted
  and live ownership metadata, refuses config/metadata drift before deletion,
  and reobserves terminal absence after an interrupted delete without replaying
  `limactl delete`. The coordinator fixture proved state/metadata drift refusal,
  default non-mutation, and fresh-preview convergence after delete-side failure.
- `make check` passed after the Lima slice: pinned lint reported `0 issues`;
  module verification, all unit and race tests, vet, Linux builds, spike smoke,
  shell checks, and Python automation tests passed. This Linux lane does not
  have `limactl`, so the slice makes no live Lima or Darwin runtime claim.
- `go test -race -count=10 ./internal/gitstore ./internal/removal` passed after
  adding the private-store removal step. Disposable real-Git fixtures proved
  that only the exact recorded store inode below the canonical selected Box
  root is removed, an unrelated Box and source repository remain intact, a
  replacement inode after preview is refused unchanged, and a fresh absent
  observation converges after deletion returned an injected interruption.
- `make check` passed after the private-store slice: pinned lint reported `0
  issues`; module verification, all unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests passed. `go vet
  ./internal/gitstore ./internal/removal` and `git diff --check` also passed.
- `go test -race -count=10 ./internal/generatedstate ./internal/removal` passed
  after adding transient generated-state teardown. Disposable path-provider
  fixtures proved exact inode-bound removal of context, Lima, Compose, empty
  workspace/Git roots, and interrupted atomic-write remnants while retaining
  the immutable Plan and diagnostics. Replacement inodes, nonempty resource
  roots, and unrecognized Box-root entries were refused without touching their
  contents; an injected post-delete interruption converged from a fresh
  observation. `go vet ./internal/generatedstate ./internal/removal` and `git
  diff --check` also passed.
- `make check` passed after the generated-state slice: pinned lint reported `0
  issues`; module verification, all unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests passed.
- `go test -race -count=10 ./internal/removal ./internal/journal
  ./internal/state` passed after binding removal to the durable operation
  journal. The real SQLite fixture held and renewed the exact operation-owned
  fenced lease beyond its original TTL, rejected a competing daemon owner,
  persisted ordered `removal.remove-started` and
  `removal.remove-succeeded` events, released at the current fence, and allowed
  a successor only at fence 2. An injected renewal loss canceled the removal
  execution context and proved that no later destructive step began.
- `go vet ./internal/removal ./internal/journal ./internal/state`, `make lint`,
  and `git diff --check` passed for the durable lifecycle slice; pinned lint
  reported `0 issues`.
- `make check` passed after the durable lifecycle slice: pinned lint reported
  `0 issues`; module verification, all unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests passed.
- `go test -race -count=10 ./internal/daemon ./internal/removal ./cmd/aboxd`
  passed after adding daemon writer quiescence. Every ordinary daemon mutation
  now enters a per-Box writer gate; removal can use its journal lease to close
  admission, drain already-admitted writers, and resume only with the same
  lease token. Race fixtures proved drain-before-success, refusal of new writers
  while fenced, cancellation rollback, wrong-token refusal, and unrelated-Box
  isolation.
- `make check`, `sprout check m1-safe-remove`, `sprout check --all`, and
  `git diff --check` passed after the writer-gate slice. Pinned lint reported
  `0 issues`; module verification, unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests all passed.
- `go test -race -count=10 ./internal/state` passed after adding schema v3 and
  the typed removal-ownership projection. Migration fixtures exercised upgrades
  from every prior schema; real SQLite tests proved exact current-generation
  and Plan-digest binding, deterministic round trips, idempotent identical
  writes, refusal of changed ownership authority, and fail-closed reads when
  authority is absent or its row identity is forged. `go vet ./internal/state`,
  `make lint`, and `git diff --check` also passed; pinned lint reported `0
  issues`.
- `make check`, `sprout check m1-safe-remove`, and `sprout check --all` passed
  after the ownership-snapshot slice. The full gate included module
  verification, unit and race tests, vet, Linux builds, spike smoke, shell
  checks, and Python automation tests.
- `go test -race -count=10 ./internal/state` passed after closing a recovery
  gap in the ownership snapshot. The schema-v1alpha2 payload now retains the
  exact immutable Lima and Compose runtime specifications, including verified
  guest-image identity, generated Compose model bytes and digest, executable
  paths, generation, and both path-evidence sets. Save and load reject any
  mismatch among those specifications, the selected Box generation, core Lima
  metadata, Compose labels, and the retained Plan digest. Five adversarial
  mutations proved that generation, path evidence, Plan identity, runtime-host
  identity, and resolved-model drift all fail before persistence.
- `make lint` passed with `0 issues`; `make check`, `sprout check
  m1-safe-remove`, `sprout check --all`, and `git diff --check` passed for the
  reconstruction-complete snapshot slice.
- `go test -race -count=10 ./internal/removal` passed after adding the
  current-generation removal assembly. The assembly loads only the selected
  Box's validated snapshot, rejects missing authority plus operation, Plan,
  generation, repository-order, and configured-root drift, and reconstructs
  the fixed Compose, Lima, worktree, private-store, and generated-state order
  with the daemon incarnation's journal lease, writer gate, and event recorder.
  Assembly tests use inert adapters and prove that construction itself performs
  no external observation, lease acquisition, or mutation.
- `make lint` passed with `0 issues`; `make check` passed after the assembly
  slice, including module verification, all unit and race tests, vet, Linux
  builds, spike smoke, shell checks, and Python automation tests. `go vet
  ./internal/removal` and `git diff --check` also passed.
- `go test -race -count=10 ./internal/removal` passed after adding the
  operation-runner boundary between daemon dispatch and the assembled removal
  coordinator. The runner strictly decodes authorization, emits the complete
  digest-bound preview before considering consent, refuses missing or stale
  consent before lease acquisition, maps failures to stable redacted contract
  codes, and emits explicit completed/remaining progress only after convergence.
  Adversarial tests proved unknown/trailing input, missing ownership authority,
  stale digests, and preview-journal failure cannot reach a remover.
- `make check` passed for the operation-runner slice: pinned lint reported `0
  issues`; module verification, all unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests passed. `sprout check
  m1-safe-remove`, `sprout check --all`, and `git diff --check` also passed.
- `go test -race -count=10 ./internal/daemon ./internal/daemonclient` passed
  after adding the typed `POST /v1/boxes:remove` route and client transport.
  The real Unix-socket fixture ran `removal.OperationRunner` end to end: the
  first request streamed the exact digest-bound preview and refused without a
  lease or remover call, while a second request returning that digest and
  explicit `yes` streamed preview, completion, and terminal events before
  removing once. A broken removal stream reattached by operation ID and resumed
  strictly after the last durable sequence. `go vet ./internal/daemon
  ./internal/daemonclient` and `git diff --check` also passed.
- `make check` passed for the removal-transport slice: pinned lint reported `0
  issues`; module verification, all unit and race tests, vet, Linux builds,
  spike smoke, shell checks, and Python automation tests passed. `sprout check
  m1-safe-remove` and `sprout check --all` also passed.
- `go test -race -count=10 ./cmd/abox` passed after adding the user-facing
  `abox rm` boundary. CLI fixtures proved preview-only removal makes exactly one
  authorization-free request, renders every exact loss without terminal-control
  ambiguity (including a newline-bearing path), and exits with the safety code.
  The authorized path makes a distinct second operation carrying the exact
  preview digest, `--yes`, and each independent discard grant. Malformed daemon
  previews are rejected as protocol failures before authorization.
- `go vet ./cmd/abox`, `git diff --check`, and `make check` passed for the CLI
  slice. The full gate reported `0 issues` and passed module verification, all
  unit and race tests, vet, Linux builds, spike smoke, shell checks, and Python
  automation tests.
- `go test -race -count=10 ./cmd/aboxd ./internal/state ./internal/removal
  ./internal/toolchain` passed after wiring production daemon dispatch. The
  daemon now routes `remove` into `removal.OperationRunner`, reconstructs the
  concrete Lima and Compose adapters lazily, and verifies the exact persisted
  Lima executable identity and SHA-256 digest before constructing either
  adapter. A real Unix-socket daemon test proved missing ownership authority
  returns `ABOX_OWNERSHIP_CONFLICT` rather than the former feature-not-supported
  response; focused tests also proved invalid artifact identity is rejected
  before provider resolution and snapshot/artifact drift is rejected during
  assembly.
- `make check`, `sprout check m1-safe-remove`, `sprout check --all`, and `git
  diff --check` passed for the production-dispatch slice. Pinned lint reported
  `0 issues`; module verification, unit and race tests, vet, Linux builds, spike
  smoke, shell checks, and Python automation tests all passed. `limactl` remains
  unavailable on this Linux lane, so no live Lima teardown is claimed.
- `go test -race -count=10 ./internal/lima ./internal/state
  ./internal/generatedstate ./internal/removal` passed after adding the final
  assembled full-Box fixture. Each run materialized a compiled Plan, persisted
  current-generation removal authority in real SQLite, created a real locked
  private-store Git worktree with dirty, ignored, and unpublished data, and
  exercised the operation runner with durable journal leases/events. Default
  execution preserved every selected resource; explicit digest-bound execution
  removed the exact Compose, Lima, worktree, private-store, and generated-state
  inventory while byte-verifying an unrelated sibling Box and source checkout.
  Post-preview dirtiness, unpublished-commit, worktree-inode, Compose-label, and
  volume-driver-policy changes all refused before the first remover. An injected
  post-delete Lima interruption changed the preview digest, and a fresh retry
  converged without replaying already-absent Compose or Lima deletion.
- The fixture exposed two previously masked integration defects and now guards
  both. Lima removal ownership distinguishes the compiled `abox-<box-id>`
  instance name from the Box ID stored in ownership metadata. Generated-state
  preflight can inventory nonempty dependency-owned Git/workspace directories,
  while final teardown accepts only an exact monotonic artifact shrink caused
  by earlier leased stages; replacement artifacts and root-identity changes
  still refuse.
- `make check` passed after the full-Box slice: pinned lint reported `0 issues`;
  module verification, all unit and race tests, vet, Linux builds, spike smoke,
  shell checks, and Python automation tests passed. `go vet ./internal/lima
  ./internal/state ./internal/generatedstate ./internal/removal` and `git diff
  --check` also passed. The runtime-host and Compose boundary remains a
  deterministic ownership-verifying fixture because `limactl` is unavailable;
  no live Lima or Darwin claim is made.

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
- Added an ownership-proved concrete Git remover. It uses Git's locked-worktree
  force primitive only after exact typed reobservation, compare-and-deletes the
  generated branch at its expected commit, verifies terminal absence, and
  models the worktree-removed/branch-remains interruption state for fresh
  preview and retry.
- Added an ownership-proved concrete Compose step over the existing guest
  runtime. It inventories exact containers, networks, volumes, labels, and
  volume-loss policy; deep-copies persisted ownership records; independently
  reobserves before the leaf call; and allows only absent members of the
  recorded set so interrupted `compose down` retries converge without adopting
  replacement resources. A live Docker fixture exercises refusal, partial
  teardown, retry, and selected-project terminal absence.
- Added an ownership-proved Lima step over the pinned typed adapter. Preview
  binds exact instance name, lifecycle state, Lima status/version, and embedded
  metadata; deletion independently revalidates the immutable config and
  ownership record, verifies terminal absence, and permits a fresh retry to
  converge after an interrupted successful delete.
- Added an ownership-proved private-store step. Preview binds the canonical
  store path, base commit, and recorded device/inode; the leaf independently
  revalidates the immutable Plan, persisted record, canonical Box containment,
  and live inode before consuming a typed path proof. Terminal absence is
  observable without replaying recursive deletion after an interruption.
- Added an ownership-proved generated-state step. Preview binds the persisted
  Box-root inode, immutable retained Plan, and exact transient artifact set;
  the leaf reobserves and consumes typed path proofs for only path-provider
  artifacts and interrupted atomic-write remnants. It refuses nonempty or
  unrecognized state and deliberately retains `plan.json` plus diagnostics so
  daemon recovery can diagnose and finalize partial cleanup.
- Added an operation-scoped journal lease guard that acquires, continuously
  renews, and exactly releases the persisted fence. Lease loss cancels the
  coordinator's execution context and is checked before every later side
  effect. Removal transitions now have a concrete redacted operation-event
  recorder.
- Added daemon-local per-Box writer admission. Journaled non-removal mutations
  enter the gate, while the removal operation bypasses self-admission and uses
  its fenced journal lease to quiesce and drain that same gate before teardown.
- Added an immutable, schema-versioned removal-ownership snapshot atomically
  bound to the current Box generation and retained Plan digest. It preserves
  Box-root and private-store inode identities, trusted publication destinations,
  Lima metadata, exact Compose IDs/labels, and Lima/Compose path-chain evidence.
  A generation accepts only byte-identical retry writes; loading strictly
  validates the payload and independently compares its Box/generation/digest to
  the selected database row. The snapshot also retains the full immutable Lima
  and Compose specifications required to reconstruct destructive adapters after
  daemon restart rather than depending on process-local reconciliation values.
- Added the restart-safe assembly boundary that consumes the current persisted
  authority and reconstructs every concrete removal step in dependency order.
  The boundary fails closed before preview when the snapshot is missing or its
  Box, Plan, generation, repository, or configured path identity differs, and
  injects only daemon-incarnation capabilities that must never be persisted.
- Added an operation runner that preserves the preview/authorization boundary
  at daemon dispatch: callers must return the exact emitted digest with every
  applicable grant, and malformed, stale, or unjournaled previews cannot enter
  the removal lease lifecycle. Provider errors remain behind stable redacted
  control-plane failures.
- Added a typed daemon removal route and resumable client method over the same
  durable operation stream as reconciliation. The route records operations as
  `remove`, so daemon execution bypasses ordinary writer admission and leaves
  lease/quiescence ownership with the removal coordinator.
- Wired production `aboxd` dispatch to the operation runner and concrete
  adapters. Removal ownership schema v1alpha3 now persists the immutable Lima
  executable identity that authorized creation; daemon restart resolves the
  local candidate, verifies its exact digest through `internal/toolchain`, and
  binds that same identity to the assembly snapshot before any preview command.
  Compose executes only through that verified Lima adapter. Missing authority
  fails before executable lookup, preserving refusal behavior on clean daemon
  installations without Lima.
- There is not yet a production successful-reconciliation path to attach this
  save to: `cmd/aboxd` still returns `ABOX_FEATURE_NOT_SUPPORTED` for non-removal
  mutations. `m1-single-repo-up` must add the real reconciliation assembly and
  save this snapshot only after all owned resources reconcile successfully.
  `abox rm` now performs a mandatory preview operation, renders exact losses,
  then starts a distinct digest-authorized operation only when `--yes` is
  present; dirty-worktree, unpublished-commit, and volume loss each retain a
  separate flag. The disposable assembled-operation fixture now proves refusal,
  exact selected-Box loss, drift, and interrupted retry through persisted
  production ownership rather than inferring those guarantees from leaf tests.

## Original request

Implement implement refusal-first box removal for Agent Box according to the cited specification and dependencies.
