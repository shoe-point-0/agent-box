<!-- sprout-task
{"schema_version":1,"id":"m1-git-worktrees","status":"implemented","created_at":"2026-08-29T06:48:36.047741734Z","implemented_at":"2026-08-29T21:22:59.63032857Z"}
-->

# Linked worktree lifecycle and observation

Status: implemented. Implementation record for `m1-git-worktrees`.

Implemented a digest-checked, idempotent linked-worktree manager over the
hardened Machine Git boundary. It validates the private-store filesystem
identity, rejects unsupported metadata before mutation, creates the exact
planned branch and worktree, applies the Box-derived lock reason, and returns a
fresh complete observation including registration, branch, commit, dirty paths,
common directory, lock reason, and both filesystem identities.

Matching worktrees are observed without mutation even after Box-local commits.
Registration, branch, lock, common-directory, path, store, and branch-ref drift
produce typed conflicts and are never reset. Failed creation cleanup is limited
to the exact identity and unchanged Git registration created by that call.
Safe removal of an established dirty or unpublished worktree remains scoped to
`m1-safe-remove`; guest-side invariant-path transport remains governed by the
Darwin hardware gate.

## Scope and dependencies

- Milestone 1. Dependencies: m1-git-private-store. Source: THE_BEGINNING.md Sections 10.1, 10.4, 10.6, and Issue 5.

## Behavior

- Create the generated branch and worktree, lock it with the Box identity, and parse NUL-delimited Git porcelain.
- Observe registration, branch, commit, dirty state, common directory, lock reason, and device/inode identity.
- Reject active submodules and unsupported relative-worktree clients.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- A second ensure call is a no-op for a matching worktree.
- Mismatched registration, branch, or ownership produces a conflict without reset.
- Git operates from Machine and canonical mounted paths in supported fixtures.

## Implementation details

- Add a focused `internal/worktree` manager over narrowly shaped operations in
  the hardened `internal/machinegit` boundary. Consume the resolver facts,
  immutable single-repository Plan, and private-store record; reject any
  disagreement before observing or mutating Git state.
- Parse `git worktree list --porcelain -z` as NUL-delimited records and combine
  the selected registration with plumbing observations for the canonical
  common directory, porcelain-v2 dirty state, and live filesystem identities.
  Treat duplicate paths, branch reuse, path/store identity drift, wrong branch,
  wrong commit during creation, and wrong or missing Box lock as typed ownership
  conflicts. Never reset or adopt ambiguous state.
- Before creation, reject gitlinks in the planned commit and Box stores that
  enable relative-worktree metadata. Create exactly one generated branch at the
  planned commit, lock it with a deterministic reason containing the Box ID,
  and re-observe the complete result. A matching existing registration returns
  its fresh observation without mutation.
- On a post-create failure, remove only a worktree whose path identity,
  registration, branch, commit, common directory, and lock still match the
  resource created by this call, then delete only the unchanged generated ref.
  Otherwise preserve the resource and surface the cleanup refusal.
- Exercise the production adapter against real Git with spaces and Unicode,
  repeated ensure, dirty observation, branch/path/lock conflicts, active
  submodules, relative-mode configuration, hostile configuration, NUL parser
  edge cases, and injected post-create failure cleanup.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run real-Git worktree lifecycle integration tests

## Validation evidence

- `make check` passed. The pinned linter reported `0 issues`; module
  tidy/verify, all Go tests, race tests, vet, builds, spike smoke checks, shell
  checks, and Python automation tests passed.
- `go test -race -count=10 ./...` passed all packages on Linux
  7.0.0-28-generic x86_64 with Git 2.43.0. The real-Git `internal/worktree`
  suite completed ten repetitions in 11.285s; the full Compose package
  completed in 132.615s.
- Real-Git lifecycle coverage created and locked a generated branch/worktree in
  paths containing spaces and Unicode, parsed `worktree list --porcelain -z`,
  verified the absolute `.git` pointer and common directory, recorded distinct
  worktree/store device-inode identities, and observed clean, advanced-commit,
  and newline-containing dirty-path states. A second ensure performed no add,
  lock, reset, or source-ref mutation.
- Refusal tests preserved wrong-lock, wrong-branch, malformed-common-pointer,
  unregistered-path, existing-branch, and stale-store-identity fixtures without
  reset or adoption. Active gitlinks, relative-worktree configuration, and
  configured checkout filters were rejected before creation; the hostile
  filter executable did not run.
- Injected lock and post-lock verification failures removed only the freshly
  identity-checked registration, exact generated ref, worktree, and empty
  parents. Linked-worktree alternate detection refused before starting Git.
- The existing Milestone 0 `git-path` fixture remains the mounted-path evidence:
  stock Machine Git 2.43.0 and container Git 2.47.3 passed status, add, commit,
  log, lock, and inode-isolation checks at identical absolute Linux bind-mount
  paths for plain, spaces, Unicode, long, and symlinked-source cases. VirtioFS
  transport remains explicitly deferred to real Darwin/arm64 hardware.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime claim is made.
