# Stable raw TCP endpoints

Status: implemented.

The `internal/tcpallocator` package provides daemon-held loopback (`127.0.0.1`)
TCP port reservation, reachability probing, stream proxying, and lease-fenced
SQLite persistence (schema `v9`). Because the allocation manager retains the
exact kernel `net.Listener` used for reservation as the proxy publication
socket, there is no reserve-then-rebind race and Machine listener ownership
never transfers to Lima.

TCP endpoint intent is projected into the canonical Plan, JSON schema, digest,
and Plan diff engine, while `internal/contextdoc` continues to publish only
live, ready `127.0.0.1` observations. Wiring a Lima-backed guest target
`Dialer` into the complete Milestone 3 routing and readiness pipeline remains
owned by the dependent Milestone 3 integration tasks.

## Scope and dependencies

- Milestone 3 transport allocator. Depends on [operation journal](operation-journal.md) and [Lima runtime-host adapter](lima-runtime-host.md). Source: `THE_BEGINNING.md` Section 13.3.

## Behavior

- Binds each allocated Machine port on `127.0.0.1` inside the daemon and retains that exact `net.Listener` for the allocation's lifetime, proxying accepted connections to a narrow target `Dialer` with bounded dial timeouts and owned connection cleanup.
- Probes target connectivity before persisting, serving, or returning an allocation so unreachable targets leave neither database rows nor bound listeners.
- Persists allocations in the `port_allocations` SQLite table (schema `v9`, keyed by Box and endpoint name with a unique Machine-port constraint), checking the complete active Box lease inside the same transaction for insert, adoption, readiness update, recovery, and removal.
- Separates listener lifetime from target availability so stopped allocations remain bound on their recorded port; on restart recovery, rebinds the recorded port and returns an occupied-port error rather than silently reallocating a different port.
- Releases an allocation only after verifying the retained listener identity and active Box lease, closing only that listener and deleting only its row.
- Projects normalized TCP endpoint intent into the canonical Plan, schema, digest, and Plan diff rules (`create`/`change` vs. `remove`), exposing endpoint creation in absent-Box planning while gating `context.json` publication on complete ready loopback observations.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Never silently reassigns an occupied recorded port across restarts without explicit atomic context updates.

## Verified acceptance criteria

- Concurrent allocations never receive the same Machine port.
- No allocation is persisted or published in `context.json` before its target is reachable.
- Removal releases only the selected Box's listener and database row.

## Implementation details

- `internal/tcpallocator` abstracts guest connectivity behind a narrow `Dialer` interface so Milestone 3 Lima/guest routing assemblies plug in without transferring Machine socket ownership to Lima.
- Schema migration `v9` in [SQLite state store](sqlite-state-store.md) enforces Box/name composite keys and Machine-port uniqueness at the database layer alongside active-lease fencing in every write transaction.
- Plan compilation (`internal/plan`), diff classification (`internal/plandiff`), and live context projection ([context document](context-document.md)) share the normalized TCP endpoint contract.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run allocator race, restart, exhaustion, and TCP reachability tests

## Validation evidence

- `go test -race -count=10 ./internal/tcpallocator ./internal/state ./internal/plan ./internal/plandiff ./internal/contextdoc` passed on Linux with real loopback sockets: four independent listener registries concurrently allocated eight endpoints with zero duplicate Machine ports and proxied exact bytes end to end.
- Negative and lifecycle tests verified unreachable targets leave no row or listener, exact-port restart recovery, occupied-port refusal, stopped-target listener retention and subsequent readiness refresh, range exhaustion without state mutation, and removal isolation across Boxes.
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only; Lima-backed Darwin routing remains Milestone 3/5 work), `sprout check --all`, and `git diff --check` passed.
