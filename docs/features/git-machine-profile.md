<!-- sprout-task
{"schema_version":1,"id":"m1-machine-git-profile","status":"implemented","created_at":"2026-08-29T06:55:50.379176125Z","implemented_at":"2026-08-29T20:00:27.54247821Z"}
-->

# Hardened Machine-side Git execution

Status: implemented.

`internal/machinegit` is the single typed leaf adapter for all Machine-side Git execution over the shared safe process runner (`internal/process`, see [process runner](process-runner.md)). It owns argv construction and execution hardening for source observation, private bare-store initialization, explicit fetch, bundle verification and import, reachability, and non-forced push—using an absolute Git binary, a controlled environment, protocol deny-by-default, explicit credential channels, strict path/ref/URL validation, and repository execution-surface overrides.

Repository-configured clean/smudge/process filters and object alternates are refused rather than assumed safe. HTTPS and SSH are the only permitted network transports; local object transfer uses validated bundles. Future Machine-side Git operations extend this typed adapter and its adversarial test suite rather than invoking Git directly.

## Scope and dependencies

- **Milestone**: Milestone 1 security prerequisite.
- **Dependencies**: [process runner](process-runner.md).
- **Design reference**: [THE_BEGINNING.md](../../THE_BEGINNING.md) Sections 10 and 16.6; [ADR 0010](../decisions/0010-security-boundaries.md).

## Behavior

- **Unified hardened execution profile**: Enforces one controlled Git environment and typed argv profile across source observation, private-store seeding, explicit fetch, bundle verification and import, reachability, and non-forced push.
- **Controlled environment and transport allowlist**: Executes an absolute Git binary with a complete environment that skips system and global configuration, disables terminal prompts and inherited Git variables, denies protocols by default, and opts into only the transport required by the typed request (HTTPS or SSH for network transfers; validated bundles for local object transfer).
- **Explicit credential channels**: Permits credentials only through explicitly supplied absolute askpass or SSH executables and an explicit SSH agent socket; ambient askpass and SSH environment variables are ignored.
- **Execution-surface overrides and refusals**: Overrides repository-controlled hooks (`hooksPath`), fsmonitor, pagers, editors, credential helpers, `sshCommand`, replacement refs, recursive submodules, and bundle URI fetching. Repositories with object alternates or configured clean/smudge/process filters are refused before the affected operation runs; bare/no-checkout data paths prevent checkout filters from running on the Machine.
- **Input validation**: Validates canonical paths, refs/refspecs (rejecting option-like and malformed values), and supported URL transports (rejecting helper and `ext` URLs) before process launch.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- No production package outside `internal/process` and `internal/machinegit` may assemble Machine-side Git commands (enforced by `TestProductionMachineGitCallsCannotBypassProfile`; Milestone 0 spike code is excluded as a disposable harness). Downstream components such as [private Git stores](git-private-store.md), [linked Git worktrees](git-worktrees.md), and [Git publication status](git-publication-status.md) extend `internal/machinegit` rather than invoking Git directly.
- Standalone closure evidence covers Linux with Git `2.43.0`; `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` provides cross-compilation evidence only (subsequently exercised on Apple M1 in [`abox up`](abox-up.md)).

## Verified acceptance criteria

- Adversarial source, global, and system Git configurations cannot execute marker commands or redirect operations.
- Crafted refs, URLs, remote helpers, hooks, askpass settings, `sshCommand`, filters, replacement refs, alternates, and `ext` transports are rejected or rendered inert.
- All Machine-side Git adapters execute through `internal/machinegit`.

## Implementation details

- **Leaf adapter boundary (`internal/machinegit`)**: Wraps `internal/process` as the sole production owner of Machine-side Git commands, guarded by `TestProductionMachineGitCallsCannotBypassProfile`.
- **Typed operations**: Exposes typed entry points for source observation, private-store seeding, explicit fetch, bundle verification/import, reachability, and non-forced push. Each operation assembles its own complete argv and validates canonical paths, refs/refspecs, and allowed URL transports before launching Git.
- **Hardening controls**: Launches an absolute Git binary with a scrubbed complete environment (no system/global config, no prompts, protocol deny-by-default, explicit askpass/SSH/agent channels only), overrides repository execution surfaces (`hooksPath`, fsmonitor, pager/editor, credential helpers, SSH command, replacements, recursive submodules, bundle URI fetching), rejects object alternates and configured filters prior to execution, and uses bare/no-checkout data paths on the Machine.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the real-Git adversarial execution-profile suite

## Validation evidence

- `go test -race -count=10 ./internal/machinegit` passed ten executions of the real-Git `2.43.0` adversarial suite. The suite exercised hostile system, global, and repository configuration; standard hook names; a replacement ref; required clean filters; object alternates; helper/`ext` URLs; option-like and malformed refs/refspecs; ambient askpass/SSH variables; explicit SSH; bare initialization; bundle verification/import; and reachability. No hostile marker executed, and configured filters and alternates were refused before the affected operation.
- `make check` passed: `golangci-lint` reported `0 issues`; module tidy/verify, all Go tests, race tests, vet, builds, smoke checks, shell syntax checks, and Python tests passed.
- `go test -race ./...` passed across all 12 Go packages.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as cross-compilation evidence only, not Darwin runtime support.
- `TestProductionMachineGitCallsCannotBypassProfile` passed, rejecting new production imports of `os/exec` or `internal/process` outside the shared process runner and `machinegit` adapter while excluding the disposable Milestone 0 spike harness.
