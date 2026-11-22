<!-- sprout-task
{"schema_version":1,"id":"m0-substrate-report","status":"implemented","created_at":"2026-08-29T06:48:35.907245415Z","implemented_at":"2026-08-29T09:39:25.423012873Z"}
-->

# Substrate evidence and architecture gates

Status: implemented.

This gate consolidated the Milestone 0 Linux substrate probes into
[substrate-spikes.md](../architecture/substrate-spikes.md) and
[ADR 0001](../decisions/0001-milestone-0-substrate-gate.md), defining the
exact tested Linux matrix, narrowing disproved assumptions in `THE_BEGINNING.md`,
and keeping every unexercised Darwin/arm64 mechanism explicitly gated.

Later Milestone 1 work validated the single-repository Apple Silicon VZ slice
under [ADR 0011](../decisions/0011-bounded-filesystem-visibility.md) (recorded
in [`abox up`](abox-up.md) and
[apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md)); the
broader multi-version Darwin/arm64 release matrix remains open in
`m5-darwin-arm64-hardware-gate`.

## Scope and dependencies

- Milestone 0 Linux implementation gate. Depends on [spike workspace bootstrap](spike-workspace-bootstrap.md), [spike harness](spike-harness.md), [Git path invariance spike](spike-git-path.md), [Compose include spike](spike-compose-include.md), [localhost routing spike](spike-localhost-routing.md), and [failure-matrix spike](spike-failure-matrix.md). Source: `THE_BEGINNING.md` Sections 19.2, 20 Milestone 0, and 25.

## Behavior

- Publishes reproducible commands, exact tool/OS versions, probe results, failure discoveries, and cleanup proofs in [substrate-spikes.md](../architecture/substrate-spikes.md).
- Documents accepted Linux mechanisms and blocking gate rules in [ADR 0001](../decisions/0001-milestone-0-substrate-gate.md).
- Narrows `THE_BEGINNING.md` where substrate probes disproved portability assumptions: path invariance across VirtioFS remained conditional, Compose `!reset []` port suppression requires capability/version verification, and `.localhost` routing requires proxy-bypass preflight.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Every Milestone 0 exit gate links to reproducible evidence.
- No support claim exceeds the tested OS, architecture, Lima, Git, Docker, and Compose matrix.
- Product implementation remains blocked when a critical substrate gate is red.
- Every Linux-exercisable critical probe passed before Milestone 1 implementation began, with Darwin/macOS support explicitly deferred until real hardware execution.

## Implementation details

- [substrate-spikes.md](../architecture/substrate-spikes.md) separates executed Linux facts from unavailable Lima/guest facts and Darwin-only obligations, linking each Milestone 0 gate to its scenario record, reproduction command, and cleanup proof.
- [ADR 0001](../decisions/0001-milestone-0-substrate-gate.md) records the evidence-gated boundary for Git stores, Compose includes, loopback routing, and crash recovery while keeping VZ, VirtioFS, mount isolation, and concurrent Lima forwarding as mandatory real-hardware obligations.
- Broader v0.1 architecture ADRs were subsequently captured in [v0.1 architecture decisions](architecture-decisions.md).

## Verification

- [x] `sprout check --all`
- [x] Verify every command and artifact link in `docs/architecture/substrate-spikes.md`

## Validation evidence

- Fresh aggregate execution on Ubuntu 24.04.4 LTS (Linux `7.0.0-28-generic` `x86_64`) from `2026-08-29T09:31:16Z` through `09:36:24Z` passed `git-path`, `compose-include`, `localhost-routing`, and all 16 real-`SIGKILL` cases in `failure-matrix`.
- Exact tested Linux matrix: Go `1.26.3`, Machine Git `2.43.0`, container Git `2.47.3`, Docker `29.6.1`, Compose `5.3.0`, containerd `2.2.5`, runc `1.3.6`, `overlay2`/extfs, cgroup v2 (`limactl`, guest image, and Darwin runtime unavailable and unclaimed).
- Every retained run root was inspected and cleaned via `abox-spike cleanup`; independent Docker queries for `dev.abox.spike.owner` returned empty and all run roots were absent.
- Mechanical report validation confirmed all 11 artifact links, all four registered scenario names, and the documented `run`/`inspect`/`cleanup` commands.
- `make check`, `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (artifact evidence only), and `sprout check --all` passed.
