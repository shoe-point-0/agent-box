<!-- sprout-task
{"schema_version":1,"id":"m1-ownership-core","status":"implemented","created_at":"2026-08-29T06:55:50.42129706Z","implemented_at":"2026-08-29T19:32:25.880876316Z"}
-->

# Authorize destructive operations with typed ownership evidence

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Authorize destructive operations with typed ownership evidence

## Context

- Milestone 1 safety prerequisite. Dependencies: m1-identity-paths. Source: THE_BEGINNING.md Sections 9.2 and 15.3.

## Requirements

- Define consuming-package proof types for paths, Git worktrees, Lima instances, Compose resources, volumes, sockets, and listeners.
- Require canonical containment and independent recorded/live identity facts before mutation or deletion.
- Keep policy decisions outside leaf adapters.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Forged names, labels, paths, symlinks, and stale records cannot authorize deletion.
- [x] Every destructive adapter API requires proof rather than raw identifiers.
- [x] Temporary-root adversarial tests show unrelated resources survive.

## Technical approach

- Add an `internal/ownership` policy package whose exported proof values have
  no exported fields and can only be produced by fail-closed verification.
  Future leaf adapters consume the resource-specific proof type; they do not
  accept a raw path, name, opaque runtime ID, or listener address for a
  destructive call.
- Bind filesystem proofs to an absolute canonical root and target plus the
  target's recorded/live device and inode identity. Reject lexical escape,
  symlink roots/targets or components, stale identities, and root deletion.
- Build typed Git worktree and Unix-socket proofs on a fresh path proof. Require
  Git's registered canonical path, common directory, and lock reason to match
  recorded facts; require the live filesystem object to be a socket.
- For Lima, Compose resources, and volumes, compare the recorded identity to a
  separately observed live identity and require non-empty embedded metadata or
  ownership labels. For listeners, require matching network/address/listener
  identity plus a matching active daemon lease. Name prefixes are never an
  accepted factor.
- Return typed, inspectable refusal errors while exposing only the minimum
  verified target data adapters need. Keep deletion policy and filesystem
  mutation out of this package. Exercise proof-consuming test adapters only
  under temporary roots, including forged labels/names/paths, stale records,
  symlink escapes, and unrelated-resource survival.

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

- `go test -race -count=100 ./internal/ownership` passed 100 repetitions of
  path, Git, Lima, Compose, volume, Unix-socket, and listener proof tests.
  Adversarial cases covered stale device/inode records, forged sibling and
  external paths, symlink targets/components, stale opaque IDs, non-Agent-Box
  and forged labels, mismatched Lima metadata, Git registration/lock drift,
  regular files substituted for sockets, rebound listeners, and stale leases.
- The proof-consuming temporary-root removal test deleted only the proved
  target and byte-verified that an unrelated sibling file survived.
- `make check` passed formatting, pinned lint (`0 issues`), module verification,
  unit tests, race tests, vet, builds, spike smoke output, and automation tests
  across all ten Go packages.
- `go test -race -count=10 ./...` passed across all ten Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime behavior is claimed.
- `git diff --check` passed.

## Outcome and follow-ups

- Added opaque, resource-specific proof types for paths, Git worktrees, Lima
  instances, Compose resources, volumes, Unix sockets, and listeners. Zero
  values expose no destructive target; proof constructors fail with an
  inspectable `ProofError` wrapping `ErrNotOwned`.
- Filesystem proof requires canonical strict containment and matching recorded
  and live device/inode identity. Git and socket proofs specialize that fresh
  path proof with live Git registration/common-directory/lock facts or socket
  file type respectively.
- Runtime proofs require exact recorded/live opaque identity plus the mandatory
  `dev.abox.box-id` live label or embedded metadata. Listener proof requires a
  matching bound identity and active daemon lease. Prefix-shaped names and
  generic Compose labels are never accepted as ownership evidence.
- No production destructive leaf adapter exists yet; upcoming Git, Lima,
  Compose, filesystem, and routing adapters can only receive the corresponding
  opaque proof target contract. Policy remains centralized in `ownership`, and
  this package performs no mutation.
- Descriptor-relative deletion and revalidation against changes after proof
  construction remain explicitly assigned to `m4-ownership-proofs`; this
  Milestone 1 contract does not claim TOCTOU resistance.

## Original request

Implement authorize destructive operations with typed ownership evidence for Agent Box.
