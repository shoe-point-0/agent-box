<!-- sprout-task
{"schema_version":1,"id":"m1-cli-foundation","status":"implemented","created_at":"2026-08-29T06:48:36.18714109Z","implemented_at":"2026-08-29T23:31:56.219620274Z"}
-->

# CLI transport and output contracts

Status: implemented. Implementation record for `m1-cli-foundation`.

Implemented the thin `abox up` boundary: deterministic manifest identity and
digest input, idempotent versioned Unix-socket requests, bounded daemon
auto-start, resumable strict event streams, human/NDJSON rendering, versioned
event schema, and stable symbolic exit mapping. Shared user-path construction
now prevents `abox` and `aboxd` from drifting.

The CLI deliberately does not remove a stale socket after daemon death. Exact
ownership reconstruction and stale-endpoint adoption remain owned by
`m4-recovery-reconstruction`; this slice detects the failed launch quickly and
fails closed rather than deleting ambiguous state. Successful Box creation also
remains owned by `m1-single-repo-up`; current `aboxd` correctly returns
`ABOX_FEATURE_NOT_SUPPORTED` until that backend is implemented.

## Scope and dependencies

- Milestone 1. Dependencies: m1-daemon-api. Source: THE_BEGINNING.md Sections 14.1 through 14.3.

## Behavior

- Discover the manifest, validate local arguments, start or connect to aboxd, send versioned requests, and render events.
- Keep stdout parseable in --json/NDJSON mode while human progress uses stderr.
- Map stable symbolic errors to the documented exit codes.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- CLI help exposes only implemented commands and never claims unavailable behavior.
- JSON output validates against its versioned schema.
- Transport, usage, not-found, safety, readiness, and compatibility errors map consistently.

## Implementation details

- Keep command parsing and rendering in `cmd/abox`; isolate the reusable,
  bounded Unix-socket protocol in an internal daemon-client package.
- Expose only `up`, `version`, and help. `up` discovers or accepts the canonical
  `.abox/box.yaml`, validates the task key locally, derives stable identities and
  a content digest, negotiates API v1, and sends one idempotent mutation request.
- Connect to the user-scoped socket first. If it is absent, launch a detached
  sibling/PATH `aboxd`, wait within a fixed startup budget, and never unlink or
  repair an endpoint the CLI does not own.
- Treat the daemon stream as an API boundary: strictly decode bounded failure and
  event payloads, resume an interrupted stream by operation ID and sequence, and
  map only stable symbolic error codes to process exits.
- Render human events and errors on stderr. In `--json` mode emit only versioned
  NDJSON event/error objects on stdout and validate representative golden output
  against repository schemas.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run CLI golden tests for human, JSON, NDJSON, and exit-code behavior

## Validation evidence

- `make check` passed with zero lint issues, module verification, all unit
  tests, the full race suite, vet, both binaries, smoke, and automation checks.
- `go test -race -count=10 ./...` passed across every package. Focused CLI,
  daemon-client, and daemon stress tests also passed before the full run.
- CLI contract tests exercised exact human stderr, schema-valid event/error
  NDJSON on stdout, local usage failures, all documented exit categories,
  manifest discovery and bounds, daemon launch, API negotiation, strict event
  decoding, contiguous replay, and terminal validation before output.
- A live Linux smoke built `abox` and `aboxd`, connected over the real private
  Unix socket, and observed `exit=2`, `operation.finished`, and
  `ABOX_FEATURE_NOT_SUPPORTED`; this is the honest backend state until the
  single-repository up slice installs a real runner.
- A disposable crash injection sent `SIGKILL` to the test daemon. The CLI did
  not unlink the unowned stale socket, observed replacement-daemon exit, and
  returned the bounded safe refusal `ABOX_DAEMON_UNAVAILABLE` in 1.02 seconds
  with recovery explicitly required.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only. Darwin runtime behavior remains unclaimed.
- `sprout check --all` passed.
