# Crash-consistent atomic file writes

Status: implemented.

The `internal/atomicfile` package provides crash-consistent file replacement
using same-directory temporary files, restrictive owner-only permissions, full
write completion, file `fsync`, atomic `rename`, and parent-directory `fsync`.
It exposes typed stage/commit errors and an injectable operations seam for
deterministic fault testing, and also backs generated-file replacement in the
Milestone 0 spike harness.

## Scope and dependencies

- Milestone 1 durability primitive. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Section 15.2.

## Behavior

- Writes complete bytes to a mode-`0600` `.abox-atomic-*` temporary file in the destination directory, applies only owner-readable (`0400`) or owner-readable/writable (`0600`) modes, syncs and closes the file, renames it atomically onto the target, and syncs the parent directory.
- Refuses symlink destinations, non-regular targets, and existing or requested group, other, or execute permission bits, revalidating the destination immediately before rename.
- Reclaims temporary files on every recoverable pre-rename failure and surfaces post-rename directory-`fsync` errors (including `ENOTSUP`) with `Committed=true` instead of silently downgrading durability guarantees.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Abrupt process death (`SIGKILL`) before rename can leave a restrictive `.abox-atomic-*` temporary file beside the intact old destination for later reconciliation; this package does not perform startup garbage collection.

## Verified acceptance criteria

- Injected failures leave either the complete old file or the complete new file, never torn content.
- Temporary files are reclaimed on recoverable failures or remain clearly identifiable (`.abox-atomic-*`, mode `0600`) after abrupt interruption.
- Symlink destinations and unsafe permissions are refused without mutating targets.

## Implementation details

- `internal/atomicfile` wraps OS file creation, write loops, `chmod`, `fsync`, `rename`, and parent-directory `fsync` behind an internal operations seam so fault tests deterministically inject short writes, file-sync failures, rename errors, and unsupported directory sync (`ENOTSUP`).
- Returned errors carry the failing stage and whether rename committed, preserving error unwrapping (`errors.Is`).
- The spike harness delegates generated-file replacement to `internal/atomicfile` while keeping its separate create-only and append-only persistence paths.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- On Linux/`amd64` with Go `1.26.3`, `make check` (`0` lint issues across six packages), `go test -race -count=10 ./...`, and `git diff --check` passed.
- `TestWriterFailureLeavesCompleteVersion` injected partial-write, file-fsync, rename, and `ENOTSUP` directory-fsync failures: pre-rename failures preserved complete old bytes with zero temporary residue, and directory-fsync failure preserved complete new bytes with `Committed=true` and `errors.Is(err, syscall.ENOTSUP)`.
- `TestWriterInterruptionLeavesIdentifiableTemporaryFile` sent `SIGKILL` to a helper process immediately before rename, confirming the old file stayed intact beside one mode-`0600` `.abox-atomic-*` temporary file.
- Negative tests confirmed refusal of symlink targets (target unchanged), requested mode `0644`, existing mode-`0644` files, and directory destinations, while short-write sequences synced the Linux parent directory and preserved exact bytes.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation evidence only; durability fault tests were further expanded in [durability fault testing](durability-fault-testing.md).
