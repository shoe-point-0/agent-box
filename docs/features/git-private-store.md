<!-- sprout-task
{"schema_version":1,"id":"m1-git-private-store","status":"implemented","created_at":"2026-08-29T06:48:36.031491662Z","implemented_at":"2026-08-29T21:04:38.679510154Z"}
-->

# Self-contained Box-private Git stores

Status: implemented.

The `internal/gitstore` package materializes a self-contained Box-private bare
Git repository from trusted resolver facts and a digest-checked immutable Plan
through the hardened `internal/machinegit` boundary. Immediately before seeding,
it re-observes source cleanliness, exact commit resolution, branch validity, and
generated-branch absence, copies local objects without hardlinks or alternates,
pins the planned commit at `refs/abox/base`, enables reflogs, strips
clone-created remotes, and cleans up identity-verified partial state on failure.

The returned Machine-owned materialization record carries the canonical publish
destination directly from resolved facts—Box-writable Git configuration is never
trusted as a publication authority. Linked worktree creation, locking,
observation, and idempotent adoption build on this store in
[linked Git worktrees](git-worktrees.md).

## Scope and dependencies

- Milestone 1 Git isolation boundary. Depends on [process runner](process-runner.md), [hardened Machine Git profile](git-machine-profile.md), [repository resolver](git-repository-resolver.md), [Plan compiler](plan-compiler.md), and [ownership proofs](ownership-proofs.md). Source: `THE_BEGINNING.md` Sections 7.2, 10.1, and Issue 5.

## Behavior

- Consumes trusted `repository.Facts` and the digest-checked immutable Plan, rejecting any disagreement before touching disk.
- Re-observes source porcelain-v2 cleanliness, full planned commit resolution, and generated-branch absence immediately before materialization, failing with `ABOX_GIT_DIRTY_SOURCE` (exit `7`) or `ABOX_BRANCH_CONFLICT` (exit `4`) before creating partial stores.
- Claims the destination directory atomically, seeds via `git clone --bare --local --no-hardlinks`, pins the planned commit at `refs/abox/base`, enables reflogs, removes clone-created `remote.origin`, and leaves zero alternates or hardlinked objects.
- Records the canonical publish destination in a Machine-owned record outside Box-writable Git config and cleans up only the exact claimed store and empty parent directories it created if post-clone configuration fails.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Object writes or ref updates inside the Box-private store cannot mutate source repository inodes or refs.

## Verified acceptance criteria

- Dirty local sources fail with `ABOX_GIT_DIRTY_SOURCE` (exit `7`) before any store or worktree creation.
- Object writes in the Box store leave source repository inodes and refs untouched.
- Detached `HEAD` commits, paths with spaces and Unicode, and branch or destination collisions pass real-Git integration tests.

## Implementation details

- `internal/gitstore` uses narrowly shaped local bare-clone, branch-observation, private-ref, and store-configuration operations on `internal/machinegit`, keeping hooks, fsmonitor, SSH, credential helpers, and ext-remotes disabled.
- Pre-existing destination directories are never overwritten or adopted without verified ownership; failures after claiming a fresh destination remove only that verified path and empty parent directories created during the attempt.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run real-Git private-store integration tests

## Validation evidence

- `go test -race -count=10 ./...` passed across all packages on Linux (`7.0.0-28-generic` x86_64, Git `2.43.0`), including ten repetitions of the real-Git `internal/gitstore` suite (`2.435s`).
- Real-Git fixtures verified detached `HEAD` commits in paths containing spaces and Unicode, `refs/abox/base` pinning, enabled reflogs, removed `remote.origin`, zero alternates, zero shared loose-object inodes between source and store, unchanged source refs after Box-only writes, and zero execution of hostile source hooks, fsmonitor, SSH, or ext-remote config.
- Refusal tests confirmed dirty-after-plan input returns `ABOX_GIT_DIRTY_SOURCE` (exit `7`) before creating the application root, existing generated branches return `ABOX_BRANCH_CONFLICT` (exit `4`), pre-existing stores remain byte-identical under ownership refusal, and injected post-clone failures remove only the claimed store and newly created empty parents.
- `make check` (`0` lint issues) and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed (real Apple M1 private-store materialization and warm identity preservation were subsequently verified in [`abox up`](abox-up.md)).
