<!-- sprout-task
{"schema_version":1,"id":"m1-git-worktrees","status":"implemented","created_at":"2026-08-29T06:48:36.047741734Z","implemented_at":"2026-08-29T21:22:59.63032857Z"}
-->

# Linked worktree lifecycle and observation

Status: implemented.

The `internal/worktree` package manages deterministic, digest-checked linked
Git worktrees on top of Box-private bare stores through the hardened
`internal/machinegit` boundary. It validates private-store filesystem identity,
rejects unsupported gitlinks, relative-worktree configurations, alternates, and
checkout filters before mutation, creates and locks the planned Box branch and
worktree with a deterministic Box-ID lock reason, and returns a fresh complete
observation covering registration, branch, commit, dirty paths, canonical
common directory, lock reason, and device/inode identities.

Matching worktrees are observed idempotently without mutation even after
Box-local commits, while registration, branch, lock, common-directory, path, or
store drift surfaces typed conflicts without resetting or adopting ambiguous
state. Safe removal of established dirty or unpublished worktrees is handled by
[`abox rm`](abox-rm.md), and Apple Silicon VirtioFS worktree
mounting and warm identity preservation were subsequently verified end to end
in [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 linked-worktree lifecycle. Depends on [private Git stores](git-private-store.md). Source: `THE_BEGINNING.md` Sections 10.1, 10.4, 10.6, and Issue 5.

## Behavior

- Consumes trusted resolver facts, the immutable single-repository Plan, and the private-store record, rejecting any disagreement before observing or mutating Git state.
- Rejects active submodules (gitlinks in the planned commit), relative-worktree store configurations, linked-worktree alternates, and hostile checkout filters prior to creation.
- Creates the planned branch and linked worktree, locks it with a deterministic Box-ID reason, parses `git worktree list --porcelain -z` as NUL-delimited records, and combines plumbing observations for common directory, porcelain-v2 dirty paths (including newlines), and live device/inode identity.
- Treats matching existing worktrees as no-ops and classifies duplicate paths, branch reuse, path/store drift, wrong branch, or missing/wrong lock as typed ownership conflicts without resetting state.
- On post-create failure, removes only the exact identity-verified worktree registration, unchanged generated branch ref, and newly created empty parent directories; otherwise preserves state and returns a cleanup refusal.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Ambiguous, unregistered, or drifted worktrees are never silently reset or adopted.

## Verified acceptance criteria

- A second `Ensure` call on a matching worktree is a no-op with no add, lock, reset, or source-ref mutation.
- Mismatched registration, branch, lock, common directory, or store identity returns a typed conflict without mutating the worktree.
- Git operates cleanly across Machine and canonical mounted paths with spaces and Unicode in supported fixtures.

## Implementation details

- `internal/worktree` executes exclusively through narrowly shaped operations on `internal/machinegit`, parsing NUL-delimited `worktree list --porcelain -z` and porcelain-v2 status so filenames containing spaces, Unicode, or newlines decode losslessly.
- Live filesystem device/inode identities are recorded for both the private bare store and the linked worktree so downstream ownership proofs ([ownership proofs](ownership-proofs.md)) and removal ([`abox rm`](abox-rm.md)) verify exact filesystem objects.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run real-Git worktree lifecycle integration tests

## Validation evidence

- `go test -race -count=10 ./...` passed on Linux (`7.0.0-28-generic` x86_64, Git `2.43.0`), with the real-Git `internal/worktree` suite completing ten repetitions in `11.285s`.
- Real-Git lifecycle tests verified creation and locking in paths containing spaces and Unicode, `worktree list --porcelain -z` parsing, absolute `.git` pointers and common directories, distinct worktree/store device-inode identities, clean/advanced-commit/newline-dirty observations, and no-op repeated `Ensure`.
- Refusal tests preserved wrong-lock, wrong-branch, malformed-common-pointer, unregistered-path, existing-branch, and stale-store-identity fixtures without reset; active gitlinks, relative-worktree settings, alternates, and hostile checkout filters were rejected before creation without executing filter binaries; injected post-create failures cleaned up only freshly identity-checked resources.
- `make check` (`0` lint issues) and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed.
- Mounted-path Git interoperability was verified on Linux in [Git path invariance spike](spike-git-path.md) (Git `2.43.0` / container Git `2.47.3`) and on Apple Silicon VirtioFS in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
