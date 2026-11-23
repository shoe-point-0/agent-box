# Destructive-operation ownership proofs

Status: implemented.

The `internal/ownership` package centralizes destructive-operation ownership
policy behind opaque, resource-specific proof types for filesystem paths, Git
worktrees, Lima instances, Compose resources, volumes, Unix sockets, and
listeners. Zero values expose no target, proof constructors fail closed with an
inspectable `ProofError` wrapping `ErrNotOwned`, and destructive leaf adapters
in [linked Git worktrees](git-worktrees.md),
[Lima runtime-host adapter](lima-runtime-host.md),
[Compose runtime adapter](compose-runtime.md), and
[`abox rm`](abox-rm.md) accept only these typed proofs rather than
raw paths, names, or IDs.

Prefix-shaped names and generic Compose labels are never accepted as ownership
evidence. `internal/ownership` performs no mutations itself; descriptor-relative
deletion and revalidation between proof and use remain
[Milestone 4 work](../../ROADMAP.md#milestone-4-recovery-and-fault-containment).

## Scope and dependencies

- Milestone 1 safety prerequisite. Depends on [identity and paths](identity-and-paths.md). Source: `THE_BEGINNING.md` Sections 9.2 and 15.3.

## Behavior

- Requires canonical strict containment under an absolute root plus matching recorded and live device/inode identity for filesystem path proofs, rejecting lexical escapes, symlinks (roots, targets, or intermediate components), stale identities, and root deletion.
- Specializes fresh path proofs for Git worktrees (requiring matching registered canonical path, common directory, and Box lock reason) and Unix sockets (requiring live socket file type).
- Requires matching recorded and independently observed live opaque IDs plus mandatory `dev.abox.box-id` live labels or embedded Lima metadata for Lima instances, Compose resources, and volumes.
- Requires matching network, address, listener identity, and active daemon lease for listener proofs.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Policy decisions stay centralized inside `internal/ownership` rather than inside leaf adapters.

## Verified acceptance criteria

- Forged names, labels, paths, symlinks, stale records, and stale leases fail proof construction and cannot authorize deletion.
- Every destructive leaf adapter API accepts typed ownership proofs rather than raw identifiers.
- Temporary-root adversarial tests verify unrelated sibling resources survive removal.

## Implementation details

- Each exported proof struct contains no exported fields and can be constructed only through fail-closed verification functions (`ProvePath`, `ProveGitWorktree`, `ProveLimaInstance`, `ProveComposeResource`, `ProveVolume`, `ProveUnixSocket`, `ProveListener`), exposing only the minimal verified target metadata adapters require.
- Refusals return typed `ProofError` values wrapping `ErrNotOwned` so callers can classify ownership failures cleanly.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `go test -race -count=100 ./internal/ownership` passed `100` repetitions covering path, Git, Lima, Compose, volume, Unix-socket, and listener proofs alongside adversarial cases for stale device/inode records, forged sibling/external paths, symlink targets and components, stale opaque IDs, non-Agent-Box and forged labels, mismatched Lima metadata, Git registration/lock drift, regular files substituted for sockets, rebound listeners, and stale leases.
- Proof-consuming temporary-root removal tests deleted only the proved target and byte-verified that unrelated sibling files survived.
- `make check` (`0` lint issues across ten packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
