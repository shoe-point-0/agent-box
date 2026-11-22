<!-- sprout-task
{"schema_version":1,"id":"m1-safe-remove","status":"implemented","created_at":"2026-08-29T06:48:36.249156679Z","implemented_at":"2026-08-30T05:45:13.364027059Z"}
-->

# Refusal-first Box removal

Status: implemented.

The `internal/removal` coordinator, `internal/generatedstate` cleaner,
`POST /v1/boxes:remove` daemon route, and `abox rm` CLI command implement
refusal-first, ownership-proved, digest-bound Box removal. Every `abox rm`
invocation first runs a read-only preview operation that computes the exact
loss inventory and binds it to a SHA-256 preview digest; non-interactive
deletion executes only when a second authorized operation supplies that exact
digest, `--yes`, and separate explicit discard grants for dirty worktrees,
unpublished commits, or Compose volumes.

Removal executes under the Box's fenced journal lease after quiescing and
draining daemon writers, re-observes every safety and ownership fact
immediately before each destructive callback, and removes resources in strict
dependency order (Compose project, Lima VM, linked worktree and branch,
private Git store, and transient generated state) while retaining `plan.json`
and diagnostics so interrupted cleanups converge safely on retry.

When safe removal first landed, production `abox up` reconciliation was
not yet wired to persist removal ownership snapshots. End-to-end snapshot
persistence on `READY` publication, live Apple M1 refusal (`ABOX_CLEANUP_REFUSED`
/ exit `7`), and verified full teardown (`abox rm --yes`) were subsequently
wired and validated in [`abox up`](abox-up.md) and
[apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
Transitioning `boxes.recorded_state` in SQLite upon removal completion (so
`abox list` reflects removed state rather than `ready`) remains a tracked
follow-up documented in [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 safe removal workflow. Depends on [Git publication status](git-publication-status.md), [ownership proofs](ownership-proofs.md), [operation journal](operation-journal.md), [reconciliation engine](reconciliation-engine.md), [Compose runtime adapter](compose-runtime.md), and [Lima runtime-host adapter](lima-runtime-host.md). Source: `THE_BEGINNING.md` Sections 10.8 and 14.1.

## Behavior

- Runs a mandatory read-only preview operation checking canonical containment, Git registration, branch, tracked/untracked/ignored dirty files (`git ls-files --others --ignored --exclude-standard -z`), conservative unpublished commit sets, Lima metadata, Compose resource inventory/volume policy, private-store inodes, and generated-state artifacts.
- Canonicalizes and hashes the complete loss inventory into a SHA-256 confirmation digest; refuses execution unless `--yes`, the exact matching preview digest, and each applicable separate discard grant (dirty worktree, unpublished commits, volume loss) are explicitly supplied.
- Acquires and continuously renews the Box's fenced journal lease (canceling execution immediately on renewal loss), closes and drains daemon-local per-Box writer admission, re-observes the complete inventory after quiescence, and re-verifies each resource immediately before its destructive step—aborting on any post-preview drift.
- Deletes only ownership-proved resources in fixed dependency order (Compose, Lima, worktree/branch, private Git store, transient generated files), recording redacted `removal.remove-started` and `removal.remove-succeeded` journal events around each step, skipping already-absent steps on retry, and retaining `plan.json` plus diagnostics.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Missing, malformed, stale, or mismatched removal-ownership snapshots fail closed (`ABOX_OWNERSHIP_CONFLICT`) before tool lookup or mutation.
- Non-empty unrecognized directories, replacement inodes, or forged labels refuse deletion and leave unrelated Boxes and source checkouts untouched.

## Verified acceptance criteria

- Default `abox rm` without `--yes` (or without required discard grants) refuses deletion, renders exact losses, and leaves every file and runtime resource intact.
- Authorized `abox rm --yes` removes only the selected Box's proved resources and leaves unrelated Boxes and source checkouts untouched.
- Interrupted removals converge safely on a fresh preview and retry without replaying already-completed deletions blindly.
- Any post-preview drift in dirtiness, unpublished commits, inode identity, ownership labels, or volume policy triggers refusal before the first destructive step.

## Implementation details

- **Persisted restart-safe authority (`internal/state`, schema `v3` / payload `v1alpha3`)**: each Box generation persists an immutable removal-ownership snapshot bound to the Box ID, generation, and retained Plan digest—recording Box-root and private-store device/inode identities, trusted publication destinations, Lima executable SHA-256 identity and immutable spec/metadata, exact Compose IDs/labels/spec/model digest, and Lima/Compose ancestor path-chain evidence. Reloading strictly validates identity parity and re-verifies the pinned Lima executable via `internal/toolchain` before reconstructing adapters.
- **Per-Box writer quiescence and operation dispatch (`internal/daemon`, `internal/daemonclient`, `cmd/aboxd`, `cmd/abox`)**: ordinary Box mutations enter a per-Box writer gate; `POST /v1/boxes:remove` records `remove` operations that bypass self-admission, drain active writers using the fenced lease, stream resumable NDJSON preview/progress events, and map failures to stable redacted contract codes.
- **Concrete ownership-proved teardown stages (`internal/removal`, `internal/generatedstate`)**:
  - *Compose*: deep-copies persisted ownership records, independently re-observes containers/networks/volumes/labels, permits only absent members of the recorded set on retry, and refuses unrecorded replacement IDs or forged labels.
  - *Lima*: binds exact instance name (`abox-<box-id>`), status/version, immutable config digest, and embedded Box metadata, stopping running VMs and re-proving ownership before `limactl delete` (added in [`abox up`](abox-up.md)).
  - *Git worktree and branch*: invokes locked-worktree force removal only after exact re-observation, compare-and-deletes the generated branch at its expected commit, and models the interrupted worktree-removed/branch-remains state for clean retry.
  - *Private store*: validates canonical Box containment and recorded device/inode via typed `ownership` path proofs before deleting the bare store.
  - *Generated state*: inventories path-provider transient files (`context.json`, Lima/Compose generated files, empty workspace/Git roots, and interrupted atomic-write remnants), accepts only exact monotonic shrink from earlier leased stages, refuses unrecognized entries or root-inode drift, and preserves `plan.json` and diagnostics.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run destructive safety tests only inside isolated temporary roots

## Validation evidence

- **Unit, race, and real-Git/real-Docker integration**: `make check` (`0` lint issues), `go vet`, `sprout check --all`, `git diff --check`, and `go test -race -count=10` passed across `internal/removal`, `internal/generatedstate`, `internal/state`, `internal/journal`, `internal/daemon`, `internal/daemonclient`, `internal/worktree`, `internal/gitstore`, `internal/lima`, `internal/compose`, `cmd/abox`, and `cmd/aboxd`.
- **Real Linux Docker & Git lifecycle checks**: `TestRuntimeLifecycleAgainstRealDocker` (`12.40s` on Linux Docker) verified label-forgery refusal and partial container teardown retry convergence; real-Git fixtures verified tracked/untracked/ignored (including newline-bearing) loss detection, locked-worktree and compare-and-delete branch teardown, interrupted branch-delete retry, replacement-inode refusal, and byte-identical preservation of unrelated sibling Boxes and source repositories.
- **End-to-end daemon, lease, writer-gate, and full-Box fixtures**: real SQLite tests verified schema `v3` migrations, `v1alpha3` snapshot drift refusal, fenced lease renewal beyond TTL and cancellation on renewal loss, per-Box writer-gate drain and wrong-token refusal, resumable Unix-socket `POST /v1/boxes:remove` streaming, CLI two-step preview-then-authorize flows, post-preview drift refusal across dirtiness/commits/inodes/labels/volumes, and interrupted Lima-delete retry convergence.
- **Real Apple Silicon CLI validation**: live refusal-first `abox rm --json` (exit `7`, `ABOX_CLEANUP_REFUSED`, zero mutations) and authorized `abox rm --json --yes` (exit `0` in `24.52 s`, empty `limactl list`, only `plan.json` retained) were subsequently verified on Apple M1 hardware in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
