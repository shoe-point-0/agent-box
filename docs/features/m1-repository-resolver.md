<!-- sprout-task
{"schema_version":1,"id":"m1-repository-resolver","status":"implemented","created_at":"2026-08-29T06:55:50.389921423Z","implemented_at":"2026-08-29T20:09:15.319648668Z"}
-->

# Read-only repository intent resolution

Status: implemented. Implementation record for `m1-repository-resolver`.

Implemented `internal/repository` as a read-only resolver over the validated manifest, deterministic identity, centralized paths, and hardened Machine Git profile. It resolves canonical Git top levels, clean porcelain-v2 status, exact SHA-1 commit IDs, task-expanded Git-validated branches, worktree paths, private-store paths, and local publish destinations without exposing any materialization collaborator.

Machine Git observation now sets `GIT_OPTIONAL_LOCKS=0`, and intended branches are checked through the same deny-by-default execution profile. Remote repository resolution remains rejected by the existing Milestone 1 executable-subset gate and belongs to Milestone 2.

## Scope and dependencies

- Milestone 1. Dependencies: m1-manifest-loader, m1-identity-paths, m1-machine-git-profile. Source: THE_BEGINNING.md Sections 7.2, 8.1, and 9.3.

## Behavior

- Read-only resolve local roots, cleanliness, full commit IDs, trusted destinations, intended branches, worktree paths, and Git-store paths.
- Return typed facts suitable for Plan compilation without creating state rows, files, stores, or worktrees.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Resolution leaves repository, filesystem, state, Lima, Compose, and ports unchanged.
- Dirty sources fail before any materialization.
- Malicious Git configuration cannot affect trusted resolved destinations.

## Implementation details

- Add a read-only `internal/repository` resolver that accepts the already validated Workspace model, deterministic Box identity, task key, centralized path provider, and the hardened `machinegit` observation boundary. Return typed source root, clean base commit, intended branch, trusted publish destination, worktree path, and private Git-store path facts for later Plan compilation.
- Treat the canonical local source from the manifest as discovery input, then use script-safe Git plumbing to find the actual top level, observe porcelain-v2 status including untracked files, resolve exactly one 40-character commit, and validate the expanded branch. A dirty source returns a typed refusal with parsed changed paths and no partial facts.
- Derive the trusted destination only from the independently canonicalized local source root, never from repository-writable remotes, URL rewrites, environment, or Git config. Expand only the loader-approved `{{ box.task }}` token and construct all Agent Box-owned paths through `internal/paths` without creating them.
- Harden all observation calls with `GIT_OPTIONAL_LOCKS=0`, then exercise real Git repositories, dirty/rename/untracked cases, hostile local/global Git configuration, nested-source discovery, cancellation/error paths, and before/after filesystem snapshots. State, Lima, Compose, and port collaborators are deliberately absent from this package, making materialization structurally unavailable.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- `HERMES_CRUSHER_REAL_GIT=/usr/bin/git go test -race -count=10 ./internal/machinegit ./internal/repository`: PASS. The real-Git suite repeatedly resolved a nested source under a path containing spaces and Unicode, returned a full commit and centralized Box paths, and proved byte/mode/mtime-identical before/after filesystem snapshots.
- The same real-Git suite returned a typed dirty-source refusal with exact staged-rename and untracked paths, no partial facts, an absent application-state root, and an unchanged repository tree. Hostile local hooks, fsmonitor, SSH command, credential helper, URL rewrite, and remote URL settings neither executed a marker nor changed the trusted local destination.
- `make check`: PASS on Linux; pinned golangci-lint reported `0 issues`, module verification passed, and all 13 packages passed unit tests, race tests, vet, builds, spike smoke, shell checks, and Python automation tests.
- `go test -race -count=10 ./...`: PASS across all 13 Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation evidence only; no Darwin runtime behavior is claimed.
- `git diff --check`: PASS.
