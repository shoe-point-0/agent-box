<!-- sprout-task
{"schema_version":1,"id":"m1-git-private-store","status":"implemented","created_at":"2026-08-29T06:48:36.031491662Z","implemented_at":"2026-08-29T21:04:38.679510154Z"}
-->

# Seed self-contained Box-private Git stores

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Seed self-contained Box-private Git stores

## Context

- Milestone 1. Dependencies: m1-process-runner, m1-machine-git-profile, m1-repository-resolver, m1-plan-compiler, m1-ownership-core. Source: THE_BEGINNING.md Sections 7.2, 10.1, and Issue 5.

## Requirements

- Consume resolved repository facts and the immutable Plan; re-observe cleanliness and full commit IDs immediately before materialization.
- Create a self-contained bare store without writable hardlinks or alternates and enable reflogs.
- Record a trusted publish destination outside Box-writable Git configuration.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Dirty local sources fail with exit semantics equivalent to ABOX_GIT_DIRTY_SOURCE before partial worktree creation.
- [x] Object writes in the Box store cannot mutate source repository inodes or refs.
- [x] Detached refs, spaces, Unicode, and branch collisions are covered by real-Git tests.

## Technical approach

- Add one `internal/gitstore` materializer that accepts both the resolver's
  trusted `repository.Facts` and the digest-checked immutable Plan. Reject any
  disagreement before filesystem mutation.
- Immediately before seeding, rerun hardened porcelain-v2 status, resolve the
  planned full commit, and inspect the intended generated branch through
  `internal/machinegit`. Return typed safety/conflict errors and retain no
  partial store on refusal or apply failure.
- Extend the typed Machine Git boundary with narrowly shaped local bare-clone,
  branch-observation, private-ref, and private-store configuration operations.
  Seed with `clone --bare --local --no-hardlinks`, pin the planned commit under
  `refs/abox/base`, enable reflogs, and remove clone-created remote config.
- Return the resolver's canonical publish destination in a Machine-owned
  materialization record; never discover publication policy from the writable
  private store.
- Exercise the real Git boundary with detached HEAD, spaces and Unicode,
  generated-branch and destination collisions, dirty-after-plan refusal,
  no-hardlink/no-alternate checks, source ref/object non-mutation, and cleanup
  after injected failures.

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
- [x] Run real-Git private-store integration tests

## Validation evidence

- `go test -race -count=10 ./...` passed all packages. The new real-Git
  `internal/gitstore` suite passed ten times on Linux 7.0.0-28-generic x86_64
  with Git 2.43.0; the full Compose package completed in 132.415s and the
  private-store package completed in 2.435s.
- The integration fixture planned a commit made from detached HEAD in a source
  path containing spaces and Unicode. Materialization copied the otherwise
  unreferenced commit, pinned it at `refs/abox/base`, enabled reflogs, removed
  clone-created `remote.origin`, and retained no alternates.
- The fixture compared every common loose object identity and found no shared
  source/store inode. A Box-only object write did not appear in the source
  object database; source refs remained byte-identical before and after. Hostile
  source hook, fsmonitor, SSH, and ext-remote configuration executed no marker.
- Real-Git refusal tests proved that dirty-after-plan input returns
  `ABOX_GIT_DIRTY_SOURCE`/exit 7 before creating the application root, an
  existing generated branch returns `ABOX_BRANCH_CONFLICT`/exit 4, and a
  pre-existing store is retained byte-for-byte under ownership refusal.
  Injected post-clone configuration failure removed the exact claimed store and
  every empty parent created by the attempt.
- `make check` passed: pinned golangci-lint reported `0 issues`; module
  tidy/verify, all Go tests, race tests, vet, builds, smoke checks, shell checks,
  and Python automation tests passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime support is claimed.

## Outcome and follow-ups

Implemented a digest-checked `internal/gitstore` materialization boundary over
the hardened Machine Git adapter. It re-observes source cleanliness, the exact
planned commit, branch validity, and generated-branch absence before mutation;
atomically claims the destination directory; copies local objects without
hardlinks or alternates; pins detached commits; enables reflogs; and performs
identity-checked cleanup on failure.

The returned Machine-owned record carries the canonical publish destination
from resolved facts. Box-writable Git configuration is deliberately not a
publication authority. Linked worktree creation, locking, observation, and
idempotent adoption remain scoped to `m1-git-worktrees`.

## Original request

Implement seed self-contained box-private git stores for Agent Box according to the cited specification and dependencies.
