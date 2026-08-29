<!-- sprout-task
{"schema_version":1,"id":"m1-machine-git-profile","status":"implemented","created_at":"2026-08-29T06:55:50.379176125Z","implemented_at":"2026-08-29T20:00:27.54247821Z"}
-->

# Harden every Machine-side Git invocation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Harden every Machine-side Git invocation

## Context

- Milestone 1 security prerequisite. Dependencies: m1-process-runner. Source: THE_BEGINNING.md Sections 10 and 16.6.

## Requirements

- Define one controlled Git environment and argv profile for source observation, seeding, fetch, bundle import, reachability, and push.
- Disable unintended hooks, pagers, editors, fsmonitor, replacements, alternates, environment config, unsafe protocols, remote helpers, and option injection while preserving explicitly supported credentials.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Adversarial source/global/system Git config cannot execute marker commands or redirect operations.
- [x] Crafted refs, URLs, helpers, hooks, askpass, sshCommand, filters, and ext transports are rejected or inert.
- [x] All Machine Git adapters use this profile.

## Technical approach

- Add one `internal/machinegit` leaf adapter over `internal/process`; no other
  product package may assemble Machine-side Git commands.
- Expose typed operations for source observation, private-store seeding,
  explicit fetch, bundle verification/import, reachability, and non-forced
  push. Each operation owns its complete argv shape and validates canonical
  paths, refs/refspecs, and supported URL transports before process start.
- Execute an absolute Git binary with a complete environment. Skip system and
  global config, disable terminal prompts and inherited Git variables, deny
  protocols by default, and opt into only the transport required by the typed
  request. Permit credentials only through explicitly supplied absolute
  askpass/SSH executables and SSH agent socket.
- Override repository-controlled execution surfaces (`hooksPath`, fsmonitor,
  pager/editor, credential helpers, SSH command, replacements, recursive
  submodules, and bundle URI fetching). Reject repositories with object
  alternates before invoking Git; use bare/no-checkout data paths so filters
  cannot execute on the Machine.
- Prove the boundary with real Git 2.43.0 tests using hostile system, global,
  and repository configuration plus crafted refs, URLs, helper syntax,
  askpass, SSH commands, hooks, filters, replacement refs, and alternates.

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
- [x] Run the real-Git adversarial execution-profile suite

## Validation evidence

- `go test -race -count=10 ./internal/machinegit` passed ten executions of
  the real-Git 2.43.0 adversarial suite. The suite exercised hostile system,
  global, and repository config; standard hook names; a replacement ref;
  required clean filters; object alternates; helper/ext URLs; option-like and
  malformed refs/refspecs; ambient askpass/SSH variables; explicit SSH; bare
  initialization; bundle verification/import; and reachability. No hostile
  marker executed. Configured filters and alternates were refused before the
  affected operation.
- `make check` passed: golangci-lint reported `0 issues`; module tidy/verify,
  all Go tests, race tests, vet, builds, smoke checks, shell syntax checks, and
  Python tests passed.
- `go test -race ./...` passed for all 12 Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as
  cross-compilation evidence only, not Darwin runtime support.
- `TestProductionMachineGitCallsCannotBypassProfile` passed and rejects new
  production imports of `os/exec` or `internal/process` outside the shared
  process runner and `machinegit` adapter; Milestone 0 spike code remains an
  explicitly excluded disposable harness.

## Outcome and follow-ups

Implemented one typed `internal/machinegit` boundary over the shared safe
process runner. It owns source observation, private bare-store initialization,
explicit fetch, bundle verification/import, reachability, and non-forced push
argv. The profile uses an absolute Git binary, a complete controlled
environment, protocol deny-by-default, explicit credential channels, strict
path/ref/URL validation, and repository execution-surface overrides.

Repository-configured clean/smudge/process filters and object alternates are
refused rather than guessed safe. HTTPS and SSH are the only network
transports; local object transfer uses validated bundles. Future Machine-side
Git operations must extend this typed adapter and its adversarial suite rather
than invoke Git directly.

## Original request

Implement harden every machine-side git invocation for Agent Box.
