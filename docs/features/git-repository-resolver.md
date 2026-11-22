<!-- sprout-task
{"schema_version":1,"id":"m1-repository-resolver","status":"implemented","created_at":"2026-08-29T06:55:50.389921423Z","implemented_at":"2026-08-29T20:09:15.319648668Z"}
-->

# Read-only repository intent resolution

Status: implemented.

The `internal/repository` package resolves local repository intent strictly
read-only from a validated manifest, deterministic Box identity, centralized
path provider, and the hardened `internal/machinegit` boundary. It produces
typed `repository.Facts`—canonical Git top-level path, clean porcelain-v2
status, exact 40-character SHA-1 commit ID, expanded and Git-validated branch
name, worktree path, private Git-store path, and trusted local publish
destination—with zero materialization collaborators in its dependency graph.

All Machine Git observation calls set `GIT_OPTIONAL_LOCKS=0` and keep hooks,
fsmonitor, SSH, credential helpers, and URL rewrites disabled. Remote
repository resolution remains gated by the Milestone 1 executable-subset check
until Milestone 2.

## Scope and dependencies

- Milestone 1 repository resolver. Depends on [manifest loader](manifest-loader.md), [identity and paths](identity-and-paths.md), and [hardened Machine Git profile](git-machine-profile.md). Source: `THE_BEGINNING.md` Sections 7.2, 8.1, and 9.3.

## Behavior

- Uses script-safe hardened Git plumbing (`GIT_OPTIONAL_LOCKS=0`) to discover the canonical Git top level from the manifest's local source path, inspect porcelain-v2 status (including untracked files), resolve one 40-character commit ID, and validate the task-expanded branch name (`{{ box.task }}`).
- Returns a typed dirty-source refusal listing parsed changed paths and zero partial facts whenever the local source is dirty.
- Derives the trusted local publish destination solely from the independently canonicalized source root—never from repository-writable remotes, URL rewrites, environment variables, or Git config.
- Computes future worktree and private Git-store paths through `internal/paths` purely in memory without creating files, directories, database rows, or runtime resources.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- State, Lima, Compose, and port collaborators are structurally absent from `internal/repository`.

## Verified acceptance criteria

- Resolution leaves the source repository, filesystem, state store, Lima, Compose, and ports completely unchanged.
- Dirty local sources fail cleanly before any materialization step.
- Hostile Git configuration cannot execute hooks/helpers or redirect trusted publish destinations.

## Implementation details

- `internal/repository` consumes only `internal/manifest`, `internal/identity`, `internal/paths`, and read-only operations on `internal/machinegit`.
- Branch template expansion permits only the loader-approved `{{ box.task }}` token and validates the resulting ref name through the same deny-by-default Machine Git profile.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `HERMES_CRUSHER_REAL_GIT=/usr/bin/git go test -race -count=10 ./internal/machinegit ./internal/repository` passed: nested sources under paths containing spaces and Unicode resolved to exact commit IDs and centralized Box paths with byte/mode/mtime-identical before/after filesystem snapshots.
- Dirty-source tests returned typed refusals with exact staged-rename and untracked paths, no partial facts, absent state roots, and unchanged source trees; hostile local hooks, fsmonitor, SSH commands, credential helpers, URL rewrites, and remote URLs neither executed nor altered the trusted local publish destination.
- `make check` (`0` lint issues across 13 packages), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
