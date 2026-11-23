# Machine and Box prerequisite diagnostics

Status: implemented.

The `internal/doctor` package and `abox doctor` CLI modes (`abox doctor`,
`abox doctor TASK`, and `abox doctor fs TASK`) provide strictly read-only
prerequisite diagnostics in deterministic human table and schema-validated JSON
formats. Every check reports a stable symbolic result, an independent evidence
class (`tested`, `warning`, `unsupported`, or `unknown`), observed and required
facts, and bounded remediation whenever a check does not pass.

`abox doctor` never starts `aboxd`, binds router ports, provisions VMs, runs
mutating `.abox-probe` filesystem canaries, or repairs state. When retained
live readiness evidence for a Box mount, port, runtime, ownership, or watcher
check is unavailable, `doctor` reports `unknown` or `warning` explicitly with
bounded remediation (including polling guidance in `fs` mode) rather than
inferring runtime truth from desired configuration.

## Scope and dependencies

- Milestone 1 diagnostics command. Depends on [Machine preflight](machine-preflight.md), [filesystem verification](filesystem-verification.md), and [CLI transport](cli-transport.md). Source: `THE_BEGINNING.md` Sections 10.5, 11.4, 14.1, and 17.

## Behavior

- Exposes Machine (`abox doctor`), Box (`abox doctor TASK`), and filesystem (`abox doctor fs TASK`) diagnostic modes backed solely by read-only observers.
- Resolves Git from `PATH`. If `HOME` or a `PATH` entry is relative, unclean, empty, or contains a control character, doctor reports the environment failure and skips tool execution. Spaces in absolute paths are valid.
- Reports Machine platform and identity, state and runtime filesystem capacity, hardened tool versions (`git`, `limactl`, `docker`, `docker compose`), Git relative-worktree safety, Docker runtime status, router-port and artifact-integrity state, and Box mount/port/runtime/ownership/watcher evidence with polling guidance when applicable.
- Returns the prerequisite exit category when any check fails or is unsupported while returning success when only `tested`, `warning`, or `unknown` results occur so partially installed Machines remain diagnosable.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Diagnostic execution never mutates filesystem state, starts `aboxd`, allocates ports, or runs mutating Git or runtime commands.

## Verified acceptance criteria

- `abox doctor` never provisions, mutates, or repairs resources.
- Every failed or non-passing prerequisite emits a stable symbolic result and bounded remediation text.
- Versioned JSON output explicitly distinguishes `tested`, `warning`, `unsupported`, and `unknown` evidence classes.

## Implementation details

- `internal/doctor` defines a value-only diagnostic contract with deterministic check sorting and strict validation that rejects malformed or duplicate observations before rendering.
- Read-only collection runs behind a context-aware observer interface. Git capability and version checks go through the hardened non-mutating `machinegit.DiagnosticClient` (with global/system config, hooks, fsmonitor, credentials, transports, optional locks, editors, and pagers disabled) so production code satisfies `TestProductionMachineGitCallsCannotBypassProfile`; Lima, Docker, and Compose probes use fixed argv-only `toolchain.Diagnostics`, and `internal/doctor` does not import the generic process runner.
- `cmd/abox` renders either a deterministic human table or a single versioned JSON document validated against the checked-in doctor JSON Schema, without invoking any daemon-client or daemon-launch dependencies.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- On Darwin/arm64 (Go 1.27.0), regression tests reproduced tool clients being enabled with an escape character in `PATH` or a tab in `HOME`. After the fix both cases reported the environment failure, skipped all tool probes, and retained the independent platform and disk checks. `make check` passed, including the race suite.
- `go test -race -count=10 ./internal/doctor/... ./internal/machinegit/... ./internal/toolchain/... ./cmd/abox/...` passed across all three modes, schema-valid JSON output, argument handling, cancellation, fixed tool probes, and hardened Git diagnostics; `go vet` and `golangci-lint` reported `0` issues.
- `TestProductionMachineGitCallsCannotBypassProfile` enforced that `internal/doctor` routes Git inspection exclusively through `machinegit.DiagnosticClient` and never through the generic process runner.
- Live Linux CLI smoke runs of `abox doctor --json`, `abox doctor TASK-47 --json`, and `abox doctor fs TASK-47 --json` produced `12`, `16`, and `18` sorted checks covering all four evidence classes (`tested`, `warning`, `unsupported`, `unknown`), observing Git `2.43.0`, Docker `29.6.1/29.6.1`, Compose `5.3.0`, missing Lima (`tool.lima` with remediation), and unsupported relative worktrees with invariant absolute-path guidance.
- Recursive metadata snapshots of the user-scoped Agent Box state tree before and after live smoke runs were byte-identical, and command unit tests verified zero calls to daemon path, client, or auto-start hooks (replaced with panicking stubs).
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), `sprout check --all`, and `git diff --check` passed.
