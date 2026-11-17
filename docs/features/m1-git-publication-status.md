<!-- sprout-task
{"schema_version":1,"id":"m1-git-publication-status","status":"implemented","created_at":"2026-08-29T06:55:50.449897416Z","implemented_at":"2026-08-29T21:32:48.83871901Z"}
-->

# Box commit publication status

Status: implemented. Implementation record for `m1-git-publication-status`.

Implemented a read-only publication classifier in `internal/publication` and a
single no-transport `ReachableCommits` operation in the hardened Machine Git
profile. Publication status is a set-difference over exact commit IDs: a commit
is published when reachable from at least one available caller-recorded
destination. The result preserves per-ref and per-destination evidence, and
unavailable destinations never become proof of safety.

Remote destination transport and sanitized publication remain Milestone 2 work.
The classifier accepts only canonical local destinations in this Milestone 1
slice and does not commit, push, fetch, or mutate any destination.

## Scope and dependencies

- Milestone 1. Dependencies: m1-machine-git-profile, m1-git-worktrees. Source: THE_BEGINNING.md Sections 10.3 and 10.8.

## Behavior

- Compute per-ref reachability against trusted destinations recorded outside Box-writable Git config.
- Treat missing/unavailable destinations conservatively and return exact commit evidence.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Clean worktrees with Box-only commits are classified unpublished.
- Malicious Box config cannot redirect reachability checks.
- Local, missing, rewritten, detached, and multiple-destination cases pass real-Git tests.

## Implementation details

- Add a read-only `internal/publication` classifier that accepts the private
  store identity, exact Box refs/commits, and an explicit ordered list of
  trusted destinations. The classifier never reads remotes or URLs from the
  Box-writable Git configuration.
- Extend `internal/machinegit` only with hardened, no-transport commit-graph
  enumeration. Resolve named Box refs against their recorded commits, enumerate
  each destination's reachable commits independently, and calculate stable
  per-ref set differences. A detached recorded commit uses the same path without
  inventing a branch.
- Treat a destination that cannot be inspected as unavailable evidence, not as
  proof of publication. Return every commit not reachable from any available
  trusted destination, plus destination-specific reachability evidence; fail
  only when the Box store/ref evidence itself is invalid or ambiguous.
- Cover the boundary with real-Git tests for a local destination, a clean
  Box-only commit, missing and rewritten destinations, detached commits,
  multiple destinations, ref drift, malformed output, and hostile Box config
  that attempts to redirect or execute Machine Git.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- Real Git 2.43.0 tests passed on Linux 7.0.0-28-generic x86_64. They
  classified a clean Box-only commit, a fully published local branch, a missing
  destination, rewritten destination history, a detached commit, and the union
  of two destinations while retaining exact per-destination commit evidence.
- The adversarial real-Git case installed hostile `core.fsmonitor`, hooks,
  pager, SSH, credential-helper, remote URL, and URL-rewrite configuration in
  the Box store. The configured executable did not run, the caller-recorded
  local destination remained authoritative, and the Box-only commit remained
  unpublished.
- Ref drift and malformed/non-terminated `rev-list` output were rejected rather
  than converted into optimistic publication state. Missing or unreadable
  destinations returned all Box-reachable commits as unpublished evidence.
- `make check` passed, including formatting, pinned lint (`0 issues`), module
  verification, all tests, race tests, vet, builds, smoke, and automation tests.
- `go test -race -count=10 ./...` passed; the publication package completed ten
  real-Git executions in 3.746s. `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go
  build ./...` passed as compilation evidence only, not Darwin runtime support.
