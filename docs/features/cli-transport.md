# CLI transport and output contracts

Status: implemented.

The `cmd/abox` binary and its internal Unix-socket client package implement the
thin CLI boundary for Agent Box: deterministic manifest discovery and digest
generation, versioned idempotent daemon requests, bounded daemon auto-start,
resumable NDJSON event streaming, human and machine-readable rendering, and
stable symbolic exit-code mapping. Shared user-path resolution prevents `abox`
and `aboxd` from drifting on state or runtime paths.

The CLI never unlinks or repairs a stale socket after daemon death; stale
endpoint adoption and crash reconstruction remain
[Milestone 4 work](../../ROADMAP.md#milestone-4-recovery-and-fault-containment). When the CLI transport first landed, `abox up`
targeted a stubbed backend returning `ABOX_FEATURE_NOT_SUPPORTED`; end-to-end
single-repository `abox up` execution was subsequently wired in
[`abox up`](abox-up.md), and additional Milestone 1 commands
were added by [`abox init`](abox-init.md),
[`abox plan`](abox-plan.md), [`abox list`](abox-list.md),
[`abox doctor`](abox-doctor.md), and
[`abox rm`](abox-rm.md).

## Scope and dependencies

- Milestone 1 CLI foundation. Depends on [daemon API](daemon-api.md). Source: `THE_BEGINNING.md` Sections 14.1 through 14.3.

## Behavior

- Discovers or accepts `.abox/box.yaml`, validates local arguments and task keys, derives deterministic identities and request digests, negotiates API `v1`, and sends idempotent requests to `aboxd` over the user-scoped Unix socket.
- Auto-starts a detached sibling or `PATH` `aboxd` within a fixed startup budget when no socket exists, failing closed with `ABOX_DAEMON_UNAVAILABLE` rather than deleting an unowned stale socket.
- Streams sequenced, schema-validated daemon events with resume support by operation ID and sequence number.
- Keeps `stdout` strictly parseable in `--json` NDJSON mode while writing human progress and errors to `stderr`, mapping only stable symbolic error codes to documented process exit codes.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- CLI help surfaces only implemented commands and never advertises unavailable capabilities.

## Verified acceptance criteria

- CLI help exposes only implemented commands and never claims unavailable behavior.
- JSON and NDJSON output validates against its versioned schema.
- Transport, usage, not-found, safety, readiness, and compatibility errors map consistently to documented exit codes.

## Implementation details

- `cmd/abox` owns command parsing, output rendering, and exit-code mapping, while a dedicated internal daemon-client package isolates the bounded Unix-socket HTTP and NDJSON protocol.
- `up` resolves the canonical `.abox/box.yaml` path, validates the task key locally, computes stable identities and a content digest, negotiates the API version range, and submits one idempotent mutation request.
- The client connects to the user-scoped Unix socket first; if absent, it spawns a detached `aboxd` and waits up to a fixed startup deadline without mutating ambiguous socket paths.
- Stream decoding enforces payload size and schema bounds, resumes interrupted streams from the last processed sequence number, and validates terminal events before emitting final output.
- Human events and failures render to `stderr`; `--json` mode emits only versioned NDJSON event and error objects on `stdout`.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run CLI golden tests for human, JSON, NDJSON, and exit-code behavior

## Validation evidence

- `make check` passed with `0` lint issues, clean module verification, unit and race suites, `go vet`, binary builds, smoke checks, and automation tests.
- `go test -race -count=10 ./...` passed across every package, along with focused CLI, daemon-client, and daemon stress tests.
- Contract and golden tests verified exact human `stderr` formatting, schema-valid NDJSON on `stdout`, local usage errors, all documented exit categories, manifest discovery and size bounds, daemon launch, API negotiation, strict event decoding, contiguous replay, and terminal validation.
- Live Linux socket smoke verified `abox` connecting to `aboxd` over the private Unix socket (observing the initial `exit=2` / `ABOX_FEATURE_NOT_SUPPORTED` stub refusal before backend wiring); live Apple Silicon `abox up` execution was subsequently verified in [`abox up`](abox-up.md).
- Disposable daemon crash injection (`SIGKILL` to a test daemon) confirmed the CLI does not unlink the unowned stale socket, detects replacement-daemon exit, and returns bounded `ABOX_DAEMON_UNAVAILABLE` in `1.02s`.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` and `sprout check --all` passed (Darwin runtime execution was later evidenced separately in [`abox up`](abox-up.md)).
