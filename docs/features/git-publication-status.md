<!-- sprout-task
{"schema_version":1,"id":"m1-git-publication-status","status":"implemented","created_at":"2026-08-29T06:55:50.449897416Z","implemented_at":"2026-08-29T21:32:48.83871901Z"}
-->

# Box commit publication status

Status: implemented.

The `internal/publication` package provides a read-only commit publication
classifier used by [`abox rm`](abox-rm.md) to determine whether Box
commits are safely preserved in trusted destinations before removal.
Publication is computed strictly as a set difference over exact commit IDs: a
commit is published if and only if it is reachable from at least one available
caller-recorded destination, preserving exact per-ref and per-destination
evidence.

Unavailable, missing, or unreadable destinations are treated conservatively as
unavailable evidence rather than proof of publication. In Milestone 1 the
classifier inspects canonical local destinations only without committing,
pushing, fetching, or mutating any repository; remote destination transport and
sanitized publication remain Milestone 2 scope.

## Scope and dependencies

- Milestone 1 publication safety classifier. Depends on [hardened Machine Git profile](git-machine-profile.md) and [linked Git worktrees](git-worktrees.md). Source: `THE_BEGINNING.md` Sections 10.3 and 10.8.

## Behavior

- Computes per-ref and detached-commit reachability against an explicit ordered list of caller-recorded trusted local destinations without ever consulting remotes or URLs in Box-writable Git configuration.
- Enumerates commit graphs via a hardened, no-transport `ReachableCommits` operation on `internal/machinegit`.
- Reports every commit not reachable from any available trusted destination alongside per-destination reachability evidence, treating missing or unreadable destinations as unavailable rather than safe, and failing closed when Box store or ref evidence itself is drifted or malformed.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Classification is strictly read-only and cannot be redirected or executed by hostile Box Git configuration.

## Verified acceptance criteria

- Clean worktrees containing Box-only commits are classified as unpublished.
- Hostile Box Git configuration cannot redirect reachability checks or execute hooks/helpers.
- Local, missing, rewritten, detached, and multi-destination scenarios pass real-Git integration tests.

## Implementation details

- `internal/publication` resolves named Box refs against recorded commit IDs (and handles detached commits without synthesizing a branch), independently enumerates reachable commits in each trusted local destination, and returns deterministic set-difference evidence.
- Ref drift or malformed/unterminated `rev-list` output causes immediate failure rather than an optimistic publication classification.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- Real Git `2.43.0` tests on Linux (`7.0.0-28-generic` x86_64) verified clean Box-only commits, fully published local branches, missing destinations (reporting all Box-reachable commits as unpublished), rewritten destination history, detached commits, and multi-destination unions with exact per-destination evidence.
- Adversarial real-Git tests installed hostile `core.fsmonitor`, hooks, pager, SSH, credential-helper, remote URL, and URL-rewrite settings inside the Box store; no configured executable ran, the caller-recorded local destination remained authoritative, and the Box-only commit remained unpublished.
- Negative tests confirmed ref drift and malformed `rev-list` output fail closed.
- `make check` (`0` lint issues), `go test -race -count=10 ./...` (`internal/publication` in `3.746s`), and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed.
