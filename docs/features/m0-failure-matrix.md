<!-- sprout-task
{"schema_version":1,"id":"m0-failure-matrix","status":"implemented","created_at":"2026-08-29T06:48:35.892187635Z","implemented_at":"2026-08-29T09:29:29.670197467Z"}
-->

# Crash residue at substrate boundaries

Status: implemented. Implementation record for `m0-failure-matrix`.

- Delivered the executable `failure-matrix` scenario, private `SIGKILL` worker boundary, typed `abox.spike.failure-matrix/v1` report, stable operation IDs, independent residue observers, idempotent adoption, reverse-order cleanup, and refusal-first ownership checks.
- Unexpected harness cleanup preflights every case before mutation, refuses dirty/unregistered Git state or mismatched Docker ownership, and removes only exact owner-marked resources.
- Lima creation/teardown, VirtioFS mounts, and Darwin forwarding remain explicitly deferred to `m5-darwin-arm64-hardware-gate`; this packet makes Linux Git, Compose, and process-local routing/cleanup claims only.

## Scope and dependencies

- Milestone 0 Linux implementation gate. Dependencies: m0-git-path-probe, m0-compose-probe, m0-routing-probe. Darwin VZ and VirtioFS kill points move to m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 9, 17, and 19.2.

## Behavior

- Add deterministic kill points before and after each Linux-exercisable Git, Compose, routing, and cleanup side effect; preserve the same operation IDs for later Darwin Lima/VirtioFS evidence.
- Inventory observed resources after each killed run without trusting the harness journal alone.
- Demonstrate adoption or safe refusal and deterministic cleanup for every residue class.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- The matrix records expected and observed residue for every kill point.
- Recovery never deletes dirty worktrees, volumes, or resources lacking ownership proof.
- A rerun converges or emits an actionable conflict instead of duplicating resources.

## Implementation details

- Add a disposable `failure-matrix` scenario to the existing retained-evidence harness. Keep stable operation IDs for the Linux Git-private-store, Git-worktree, Compose-project, loopback-listener, and symmetric cleanup boundaries; reserve those IDs unchanged for the later Darwin Lima/VirtioFS rerun.
- Execute every operation at deterministic `before` and `after` kill points in a real child process. The child writes only a control receipt before sending itself `SIGKILL`; the parent then inventories filesystem/Git, Docker labels, and listener reachability directly rather than deriving observed state from the append-only harness inventory.
- Use a fresh ownership-marked case per kill point. Recovery re-observes resources, adopts only exact owner/path/label matches, converges missing or partially-created resources without duplication, and tears down in reverse order. A mismatched marker/label produces an actionable ownership conflict and no destructive mutation.
- Add explicit dirty-worktree and unproved-volume refusal cases. Verify the dirty file and foreign volume remain byte-for-byte/present after refusal, then remove only matrix-created canaries using the parent process's exact case identity and prove a clean rerun converges.
- Emit a typed JSON matrix containing expected and observed residue, recovery disposition, refusal evidence, and final absence checks. Unit-test kill-point enumeration, observation/classification, ownership refusal, and idempotent recovery; execute the complete real Linux matrix as the acceptance boundary.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Execute the complete substrate kill-point matrix

## Validation evidence

- On Linux 7.0.0-28-generic x86_64 with Go 1.26.3, Git 2.43.0, Docker client/server 29.6.1, and Compose 5.3.0, `go run ./cmd/abox-spike run --parent /tmp/abox-failure-matrix-run failure-matrix` passed from 2026-08-29T09:23:58Z through 09:28:32Z.
- Eight stable logical operation IDs covered private-store creation, worktree creation, Compose apply, loopback bind, loopback close, Compose removal, worktree removal, and private-store removal. All 16 `before`/`after` workers reached their exact receipt and died by real `SIGKILL`.
- Parent-side observation used `os.Stat`, Git `worktree list --porcelain -z`, direct Docker container/network/volume inspection, and a fresh TCP dial. All 16 observed residue records exactly matched expected residue without consulting the harness journal.
- Every killed case reran from observed state, adopted exact owner matches, avoided duplicates, and converged to zero private stores, worktrees/registrations, containers, networks, volumes, route receipts, or reachable listeners.
- A dirty untracked worktree canary caused an actionable conflict and remained byte-for-byte intact. A uniquely named volume carrying a foreign owner label caused an actionable ownership conflict and remained present. After the parent removed only its disposable canaries, both cases converged cleanly.
- The retained run root was cleaned through the ownership-aware harness command. Independent Docker queries found no remaining resource carrying `dev.abox.spike.owner`; the run root and matrix tree were absent.
- The first execution exposed Docker's network-miss spelling (`not found`, not `No such`); absence classification now handles both forms. A later execution disproved the assumption that `git worktree remove` deletes its branch; recovery now observes and reuses the surviving branch instead of issuing duplicate `-b`.
- `make check`, `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed. Cross-compilation is artifact evidence only and does not claim Darwin runtime support.
