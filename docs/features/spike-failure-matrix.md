<!-- sprout-task
{"schema_version":1,"id":"m0-failure-matrix","status":"implemented","created_at":"2026-08-29T06:48:35.892187635Z","implemented_at":"2026-08-29T09:29:29.670197467Z"}
-->

# Crash residue at substrate boundaries

Status: implemented.

Delivers the executable `failure-matrix` Linux scenario, a private `SIGKILL` worker boundary, a typed `abox.spike.failure-matrix/v1` retained report, eight stable logical operation IDs, independent residue observers, idempotent adoption, reverse-order cleanup, and refusal-first ownership checks.

Unexpected harness cleanup preflights every case before mutation, refuses dirty or unregistered Git state and mismatched Docker ownership, and removes only exact owner-marked resources. Lima creation/teardown, VirtioFS mounts, and Darwin forwarding kill points remain deferred to `m5-darwin-arm64-hardware-gate`; this record makes Linux Git, Compose, and process-local routing/cleanup claims only.

## Scope and dependencies

- **Milestone**: Milestone 0 Linux implementation gate.
- **Dependencies**: [Git path invariance spike](spike-git-path.md), [Compose include spike](spike-compose-include.md), [localhost routing spike](spike-localhost-routing.md).
- **Platform boundary**: Darwin VZ and VirtioFS kill points move to `m5-darwin-arm64-hardware-gate` under the same stable operation IDs.
- **Design reference**: [THE_BEGINNING.md](../../THE_BEGINNING.md), Sections 9, 17, and 19.2.

## Behavior

- Executes deterministic `before` and `after` kill points across eight stable Linux substrate operations (Git private store, Git worktree, Compose apply, loopback bind, and their symmetric cleanup boundaries) while reserving those operation IDs unchanged for later Darwin Lima/VirtioFS reruns.
- Inventories observed filesystem, Git, Docker, and TCP listener state directly after each real `SIGKILL` without trusting the append-only harness journal alone.
- Demonstrates idempotent adoption or refusal-first conflict reporting and deterministic reverse-order cleanup for every residue class.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- The matrix records expected and observed residue for every kill point.
- Recovery never deletes dirty worktrees, volumes, or resources lacking ownership proof.
- A rerun converges or emits an actionable conflict instead of duplicating resources.

## Implementation details

- **Stable operation IDs and kill-point boundary**: Registers the disposable `failure-matrix` scenario with eight logical operations spanning private-store creation, worktree creation, Compose apply, loopback bind, loopback close, Compose removal, worktree removal, and private-store removal (16 `before`/`after` kill points).
- **Child `SIGKILL` worker and independent observation**: Runs each kill point in a real child process under a fresh ownership-marked case. The child writes only a control receipt before sending itself `SIGKILL`. The parent independently observes residue via `os.Stat`, `git worktree list --porcelain -z`, direct Docker container/network/volume inspection, and a fresh TCP dial rather than reading the harness inventory.
- **Idempotent adoption and reverse-order teardown**: Recovery re-observes live resources, adopts only exact owner/path/label matches, converges missing or partially created resources without duplication, and tears down in reverse order. Any mismatched marker or label produces an actionable ownership conflict without destructive mutation.
- **Refusal canaries**: Exercises explicit dirty-worktree (untracked file) and unproved foreign-owner Docker volume cases, verifying that both remain byte-for-byte intact and present after refusal before the parent removes only its own disposable canaries via exact case identity and confirms a clean rerun converges.
- **Typed report and coverage**: Emits `abox.spike.failure-matrix/v1` recording expected versus observed residue, recovery disposition, refusal evidence, and final absence checks. Unit tests cover kill-point enumeration, observation and classification, ownership refusal, and idempotent recovery.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Execute the complete substrate kill-point matrix

## Validation evidence

- On Linux `7.0.0-28-generic` x86_64 with Go `1.26.3`, Git `2.43.0`, Docker client/server `29.6.1`, and Compose `5.3.0`, `go run ./cmd/abox-spike run --parent /tmp/abox-failure-matrix-run failure-matrix` passed from `2026-08-29T09:23:58Z` through `09:28:32Z`.
- All eight stable logical operations (private-store creation, worktree creation, Compose apply, loopback bind, loopback close, Compose removal, worktree removal, and private-store removal) and all 16 `before`/`after` workers reached their exact control receipt and exited via real `SIGKILL`.
- Parent-side observation (`os.Stat`, `git worktree list --porcelain -z`, direct Docker container/network/volume inspection, and fresh TCP dials) matched expected residue across all 16 cases without consulting the harness journal.
- Every killed case reran from observed state, adopted exact owner matches without duplicates, and converged to zero private stores, worktrees or registrations, containers, networks, volumes, route receipts, or reachable listeners.
- A dirty untracked worktree canary produced an actionable conflict and remained byte-for-byte intact; a uniquely named volume with a foreign owner label produced an actionable ownership conflict and remained present. After parent cleanup of only its disposable canaries, both cases converged cleanly.
- Ownership-aware harness cleanup removed the retained run root; independent Docker queries found no remaining resource carrying `dev.abox.spike.owner`, and both the run root and matrix tree were absent.
- **Design findings during validation**: The first execution revealed Docker's network-miss wording (`not found` rather than `No such`), so absence classification handles both forms. A later execution showed `git worktree remove` retains its branch, so recovery observes and reuses surviving branches instead of issuing a duplicate `-b`.
- `make check`, `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed. Cross-compilation is artifact evidence only and makes no Darwin runtime claim.
