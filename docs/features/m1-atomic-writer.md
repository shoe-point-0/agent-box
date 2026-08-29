<!-- sprout-task
{"schema_version":1,"id":"m1-atomic-writer","status":"implemented","created_at":"2026-08-29T06:55:50.358054833Z","implemented_at":"2026-08-29T15:27:27.582737378Z"}
-->

# Implement crash-consistent atomic file writes

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement crash-consistent atomic file writes

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 15.2.

## Requirements

- Write temporary files in the destination directory with restrictive modes, complete writes, file fsync, atomic rename, and parent-directory fsync where supported.
- Expose injectable write, fsync, rename, and directory-fsync failures without hiding unsupported durability behavior.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Injected failure yields either the complete old file or complete new file, never torn content.
- [x] Temporary files are reclaimed or safely identifiable after interruption.
- [x] Symlink destinations and unsafe permissions are refused.

## Technical approach

- Add a focused `internal/atomicfile` package. It creates a mode-`0600` temporary file beside the destination, applies only owner-readable (`0400`) or owner-readable/writable (`0600`) requested modes, writes all bytes, fsyncs and closes the file, atomically renames it, and fsyncs the parent directory.
- Refuse existing symlinks, non-regular destinations, and existing or requested group/other/execute permissions. Revalidate the destination immediately before rename so a rejected target is never deliberately followed.
- Keep the operating-system implementation as the default while exposing a complete internal operations seam for deterministic write, file-fsync, rename, and directory-fsync fault injection. Return a typed stage error that records whether rename committed; propagate directory-sync errors, including unsupported-operation errors, rather than treating rename as durable.
- Use an Agent Box-specific temporary-file prefix. Remove temporary files on all recoverable pre-rename failures; prove with a killed helper process that interruption can leave only a restrictive, recognizable temporary file and the complete old destination.
- Cover success and every injectable boundary with byte-for-byte old/new assertions, cleanup checks, symlink and permission refusal tests, and Linux directory-fsync execution evidence.

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

## Validation evidence

- On Linux/amd64 with Go 1.26.3, `make check` passed: pinned `golangci-lint` reported `0 issues`; module verification, all six package test suites, race tests, vet, product and repository builds, spike smoke, and automation checks passed.
- `TestWriterFailureLeavesCompleteVersion` injected a partial-write error, file-fsync error, rename error, and `ENOTSUP` directory-fsync error. The first three retained the complete old bytes, directory-fsync failure exposed the complete new bytes with `Committed=true`, `errors.Is(err, syscall.ENOTSUP)` remained true, and every recoverable path left zero temporary files.
- `TestWriterInterruptionLeavesIdentifiableTemporaryFile` killed a real helper process with `SIGKILL` immediately before rename. The old destination remained complete and exactly one `.abox-atomic-*` file remained beside it with complete new bytes and mode `0600`.
- Tests refused a symlink destination without changing its target, requested mode `0644`, an existing mode-`0644` destination, and a directory destination. A real replacement and an injected short-write sequence both completed, called Linux parent-directory fsync, preserved exact bytes, and left no temporary files.
- `go test -race -count=10 ./...` passed all six packages. `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` also passed as compilation evidence only; no Darwin durability claim is made.
- `git diff --check` passed.

## Outcome and follow-ups

- Added `internal/atomicfile` with complete-write handling, restrictive modes, same-directory temporary files, file and directory fsync, atomic rename, typed stage/commit errors, and an explicit injectable operations seam.
- Routed the spike harness's generated-file replacement helper through the production primitive, removing its duplicate weaker implementation while retaining its separate create-only and append-only persistence paths.
- Recoverable failures reclaim temporary files. Process death can leave a restrictive `.abox-atomic-*` file that is deliberately recognizable for later reconciliation; this slice does not add startup garbage collection.
- Directory-fsync failure after rename is returned as a committed-but-not-proven-durable error. Unsupported directory fsync is never silently downgraded.

## Original request

Implement implement crash-consistent atomic file writes for Agent Box.
