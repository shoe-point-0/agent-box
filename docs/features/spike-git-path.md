# Private Git-store path invariance

Status: implemented.

The `git-path` spike scenario verifies that a Box-private bare Git repository
and locked linked worktree behave identically across Machine and container
boundaries when mounted at identical canonical absolute paths, without mutating
the source repository or rewriting Git pointers manually. Results are recorded
as typed `abox.spike.git-path/v1` JSON evidence.

This Linux probe exercises bind mounts only and makes no Darwin, VZ, VirtioFS,
or macOS Git support claim; the same fixtures remain available for the
Darwin/arm64 release gate.

## Scope and dependencies

- Milestone 0 substrate probe. Depends on [spike harness](spike-harness.md). Source: `THE_BEGINNING.md` Sections 10 and 19.2.

## Behavior

- Seeds a self-contained bare Git store using `--no-hardlinks` (no shared hardlinks or alternates) and creates a locked linked worktree.
- Runs stock Git on the Machine and inside a network-disabled container against identical absolute worktree and common-directory paths.
- Exercises plain paths, spaces, Unicode (`unicodé-路径`), 200-character path components, symlinked source roots, and versioned `worktree.useRelativePaths` capability probing.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Status, add, commit, log, and lock inspection agree across Machine and container boundaries.
- Source repository metadata and object inodes remain unchanged after Box Git writes.
- Unsupported path or Git-version combinations fail or report explicitly with retained evidence rather than rewriting pointers manually.

## Implementation details

- The `git-path` scenario owns one marker-verified fixture tree and cleans it only after verifying the persisted run ID, same-filesystem containment, and directory permissions.
- Each fixture snapshots source Git metadata, clones a bare store with `--no-hardlinks`, attaches and locks a linked worktree, mounts the retained run root into a network-disabled container at the exact same absolute path with Machine UID/GID mapping, and runs Git commands via literal argv without shell interpolation.
- Verification checks for zero alternates, zero shared source/Box object inodes, byte- and mode-identical source metadata before and after writes, identical Machine/container `HEAD` and `git log -1 --format=%H`, clean porcelain-v2 status, and matching worktree lock registration.
- `worktree.useRelativePaths` is probed as a versioned capability; when Git does not advertise it, the scenario records an explicit unsupported result rather than editing `.git` pointers manually.
- Scenario-specific cleanup journals removed resources before generic harness teardown; generic cleanup still refuses unknown or active unsupported resource kinds.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the Git path probe against every declared path fixture

## Validation evidence

- Passed on Linux `7.0.0-28-generic` `amd64` with Machine Git `2.43.0`, Docker `29.6.1`, and container Git `2.47.3` from locally cached `golang:1.26` (`sha256:73204d15b474cccdcf60645cfc88bf3d8d68cbc1687334c70f66bd03832e8259`) with networking disabled.
- All five fixtures (plain, spaces, `unicodé-路径`, 200-character component, and symlinked source) completed Machine and container commits with matching `HEAD`, `git log`, clean porcelain-v2 status, observed lock registration, no alternates, no shared object inodes, and unchanged source metadata snapshots.
- Container Git executed under mapped Machine UID/GID `1000:1000` (after an initial run exposed root-owned objects and motivated UID/GID mapping plus cleanup ownership preflights); complete ownership scans found only `(1000, 1000)` and final cleanup left zero probe containers or roots.
- Neither Machine Git `2.43.0` nor container Git `2.47.3` advertised `worktree.useRelativePaths`, recorded as unsupported with manual pointer rewriting forbidden.
- `make check`, `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed.
