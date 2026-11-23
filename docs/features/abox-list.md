# Box inventory with abox list

Status: implemented.

`abox list` queries Box inventory over the version-negotiated daemon API (`GET /v1/boxes`). In one read-only SQLite transaction ordered by Box ID, the daemon loads Box and Workspace identities together with the latest persisted runtime-host and Compose observations, derives a stable state (`active`, `stopped`, `degraded`, or `stale`) and drift summary, and returns the result without any reconciliation, mutation, or cleanup capability. Human output renders a deterministic table; `--json` conforms to `abox.list/v1alpha1` (`schemas/v1alpha1/list.schema.json`).

`abox list` intentionally reports the persisted observation cache populated by runtime adapters rather than shelling out to Lima or Docker during listing. Note: transitioning `boxes.recorded_state` in SQLite when `abox rm --yes` completes remains a separate removal state-lifecycle follow-up tracked in [`abox up`](abox-up.md).

## Scope and dependencies

- Milestone 1 command. Dependencies: [daemon API](daemon-api.md), [CLI transport](cli-transport.md). Source: `THE_BEGINNING.md` Section 14.1.

## Behavior

- Lists Boxes in deterministic tabular human format or strict `abox.list/v1alpha1` JSON, displaying Box identity, derived state (`active`, `stopped`, `degraded`, or `stale`), task, workspace, and observed drift reasons when recorded intent and cached observations disagree.
- Reads persisted observations in a single read-only transaction ordered by Box ID without reconciling, starting, or deleting resources.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Listing reads persisted observation state only and never invokes external runtime tools or mutates state.

## Verified acceptance criteria

- Empty, `active`, `stopped`, `degraded`, and `stale` inventory records render deterministically.
- Listing is strictly read-only and never reconciles or deletes resources.
- JSON output validates against `schemas/v1alpha1/list.schema.json` (`abox.list/v1alpha1`).

## Implementation details

- **Dedicated list contract and schema**: Defines `abox.list/v1alpha1` and its strict JSON Schema (`schemas/v1alpha1/list.schema.json`) separately from single-Box operation outputs.
- **Single-transaction state and drift derivation**: Reads Box, Workspace, cached runtime-host, and cached Compose observations in one read transaction ordered by Box ID. Derives `active`, `stopped`, `degraded`, or `stale` and emits stable drift reasons whenever recorded intent and observed components disagree.
- **Read-only daemon endpoint and client**: Served via version-negotiated `GET /v1/boxes` over the bounded Unix-socket client. The listing path has no reconciler, mutation runner, or cleanup capability.
- **Deterministic CLI rendering (`cmd/abox`)**: Renders either schema-valid JSON (`--json`) or a deterministic tabular human view, backed by strict bounded client decoding.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- `make check`: passed (formatting, `0 issues` from the pinned linter, dependency verification, unit tests, race tests, `go vet`, builds, smoke, and automation checks).
- `go test -race -count=10 ./...`: passed across every package.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: passed as compilation evidence only; no Darwin runtime support is inferred.
- Unit and transport tests verified empty, `active`, `stopped`, `degraded`, and `stale` inventories, stable Box-ID ordering, strict bounded client decoding, JSON Schema validation, human table rendering, and unchanged persisted records before and after a list call.
- Live Linux daemon/CLI smoke against an isolated state database and Unix socket (enforcing a `0700` runtime parent; an initial `0775` fixture was rejected by the daemon's secure-directory preflight): empty JSON returned `{"schemaVersion":"abox.list/v1alpha1","boxes":[]}`; after seeding all four states, both human and JSON outputs were deterministic and before/after SQL snapshots were byte-identical. The owned daemon process and temporary root were removed afterward.
