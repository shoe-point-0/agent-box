<!-- sprout-task
{"schema_version":1,"id":"m1-doctor-command","status":"implemented","created_at":"2026-08-29T06:55:50.510725152Z","implemented_at":"2026-08-30T03:26:04.589717043Z"}
-->

# Machine and Box prerequisite diagnostics

Status: implemented. Implementation record for `m1-doctor-command`.

Implemented deterministic read-only `abox doctor`, `abox doctor TASK`, and
`abox doctor fs TASK` modes with human and strict versioned JSON output. Every
check has a stable symbolic result plus an independent evidence class and
status; all non-passing results require bounded remediation. The Linux observer
reports Machine platform/identity, state and runtime filesystem capacity,
hardened tool versions and Git relative-worktree capability, Docker runtime,
router-port uncertainty, and artifact-integrity uncertainty without attempting
to reserve or repair anything.

Box mount, port, runtime, ownership, filesystem-probe, and watcher facts are
reported explicitly as unknown or warning when no retained live readiness
evidence exists. Doctor does not start the daemon or rerun the mutating
`.abox-probe` acceptance suite merely to manufacture an answer. Persisting and
serving those facts from successful reconciliation remains with the consuming
single-Box operation; until then filesystem mode gives polling guidance rather
than claiming unexecuted VirtioFS behavior.

## Scope and dependencies

- Milestone 1 command. Dependencies: m1-machine-preflight, m1-filesystem-verifier, m1-cli-foundation. Source: THE_BEGINNING.md Sections 10.5, 11.4, 14.1, and 17.

## Behavior

- Implement general, Box-specific, and fs doctor modes using read-only observations.
- Report relative-worktree safety, watcher behavior/polling guidance, tools, mounts, ports, disk, runtime, and ownership evidence.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Doctor never provisions or repairs resources.
- Every failed prerequisite has a stable symbolic result and remediation text.
- JSON output distinguishes tested fact, warning, unsupported case, and unknown.

## Implementation details

- Add a value-only `internal/doctor` contract with explicit machine, Box, and
  filesystem modes. Each check carries a stable symbolic result, a separate
  evidence class (`tested`, `warning`, `unsupported`, or `unknown`), observed
  and required facts, and remediation whenever it does not pass. Reject
  malformed or duplicate observations rather than rendering ambiguous output.
- Keep collection behind a context-aware read-only observer. The Linux live
  observer may inspect paths, filesystem capacity, process identity, tool
  versions/capabilities, and existing listeners/runtimes, but it cannot create
  directories, bind ports, start the daemon, provision a VM, run a filesystem
  canary, or repair state. Box and filesystem evidence that cannot be obtained
  without those effects is reported explicitly as unknown with a bounded
  remediation instead of being inferred from desired configuration.
- Expose `abox doctor`, `abox doctor TASK`, and `abox doctor fs TASK` directly
  from the thin CLI. Render a deterministic human table or one versioned JSON
  document, return the prerequisite exit category when a check fails or is
  unsupported, and retain success for warnings/unknown evidence so diagnostics
  remain usable on partially installed Machines.
- Add a strict JSON Schema and command/package tests proving all four evidence
  classes, stable failed results and remediation, deterministic ordering,
  cancellation, argument handling, and zero calls to mutation-capable daemon
  dependencies. Exercise the built command on Linux and compare relevant
  filesystem state before and after; do not describe that as Darwin, Lima, VZ,
  or VirtioFS runtime evidence.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...

## Validation evidence

- `go test -race -count=10 ./internal/doctor/... ./internal/machinegit/...
  ./internal/toolchain/... ./cmd/abox/...` passed ten iterations of the report
  contract, all three command modes, schema-valid JSON, argument handling,
  cancellation, fixed tool probes, and hardened Git diagnostics. `go vet` and
  focused `golangci-lint` passed with `0 issues`.
- The first implementation attempted to invoke Git through the generic process
  runner. The full gate correctly rejected that hypothesis in
  `TestProductionMachineGitCallsCannotBypassProfile`. The corrected slice adds
  a non-mutating `machinegit.DiagnosticClient` with global/system config,
  hooks, fsmonitor, credentials, transports, optional locks, editors, and
  pagers disabled. Lima/Docker/Compose probes moved behind fixed argv-only
  `toolchain.Diagnostics`; `internal/doctor` no longer imports the generic
  process boundary.
- A built Linux CLI executed `abox doctor --json`, `abox doctor TASK-47
  --json`, and `abox doctor fs TASK-47 --json`. They returned respectively 12,
  16, and 18 sorted checks and each represented all four evidence classes:
  `tested`, `warning`, `unsupported`, and `unknown`. Git `2.43.0`, Docker
  client/server `29.6.1/29.6.1`, and Compose `5.3.0` were observed; missing
  Lima produced the stable failed result `tool.lima` with remediation, while
  Git correctly reported relative-worktree capability unsupported and retained
  invariant absolute-path guidance.
- Before and after that live smoke, a recursive metadata snapshot of the
  existing user-scoped Agent Box state tree was byte-identical. Command tests
  replace daemon path/client/start dependencies with panics and complete all
  modes without invoking them. The command does not bind or connect to the
  router port, start `aboxd`, create state paths, run a worktree canary, or
  provision/repair any resource.
- `make check` passed on Linux after the command-boundary correction: pinned
  lint reported `0 issues`, module verification, all tests and race tests, vet,
  builds, smoke, shell guards, and Python automation checks passed.
- `go test -race -count=10 ./...` passed across every package, including real
  Docker/Compose integration. `sprout check --all` and `git diff --check`
  passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only. No Darwin, VZ, VirtioFS, mount, watcher, or port runtime
  behavior is claimed.
