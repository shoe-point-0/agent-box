<!-- sprout-task
{"schema_version":1,"id":"m3-tcp-allocator","status":"implemented","created_at":"2026-08-29T06:48:36.42631655Z","implemented_at":"2026-08-30T08:50:16.462766373Z"}
-->

# Stable raw TCP endpoints

Status: implemented. Implementation record for `m3-tcp-allocator`.

- Added `internal/tcpallocator`, whose manager owns the exact `net.Listener`
  used for reservation and proxy publication. Target transport is a narrow
  `Dialer`, allowing the later Lima/guest routing assembly to supply streams
  without handing Machine listener ownership to Lima.
- Allocation is fenced by the complete active Box lease in the same SQLite
  transaction as insert, adoption, readiness update, recovery, or removal.
  Schema v9 adds stable Box/name and globally unique Machine-port ownership.
- Initial allocation probes before persistence, serving, or return. Restart
  recovery rebinds the recorded port, refuses occupation rather than silently
  reallocating, and retains a not-ready listener while its stopped target is
  unavailable. Context generation accepts only a complete planned set of ready
  loopback observations.
- TCP intent is now canonical Plan content, included in the digest and schema.
  Diff rules identify per-endpoint create/change versus removal, and absent-Box
  planning exposes endpoint creation explicitly.
- This Linux slice exercises real loopback allocation and forwarding. Wiring a
  Lima-backed target `Dialer` into the complete route/readiness pipeline remains
  owned by the dependent Milestone 3 integration tasks; it is not inferred from
  the Darwin cross-build.

## Scope and dependencies

- Milestone 3 transport. Dependencies: m1-operation-journal, m1-lima-adapter. Source: THE_BEGINNING.md Section 13.3.

## Behavior

- Own a daemon-held loopback listener for each allocation and proxy to the Box target so reservation and publication use the same kernel object; if direct Lima binding is retained, prove an atomic descriptor handoff before implementation.
- Persist allocation ownership under the Box lease and keep the listener bound while stopped; detect occupied recorded ports after daemon or Machine restart and refuse or explicitly reallocate with an atomic context update.
- Verify target connectivity and recover allocations after restart without races.
- Add TCP allocation Plan/context projection, state migration, and diff rules in the same reviewable slice.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Concurrent allocations cannot receive the same Machine port.
- No allocation is published before its target is reachable.
- Removal releases only the selected Box allocation.

## Implementation details

- Add a daemon-local TCP allocation manager that binds each candidate port on
  `127.0.0.1` itself and retains that exact `net.Listener` for the allocation's
  lifetime. The manager probes the configured Box target before persisting,
  returning, or serving an allocation, then proxies accepted streams with
  bounded dial timeouts and owned connection cleanup.
- Advance the SQLite schema with a `port_allocations` table keyed by Box and
  endpoint name, unique on Machine port, and fenced by the exact active Box
  lease. Allocate and recover compare the complete lease identity in the same
  transaction as persistence. Recovery rebinds the recorded port or returns an
  occupied-port error; it never silently changes the context-visible endpoint.
- Keep stopped allocations bound by separating listener lifetime from target
  availability. Remove one allocation only after the retained listener identity
  and active lease prove ownership; close only that listener and delete only its
  Box/name row.
- Project normalized TCP endpoint intent into the canonical Plan, JSON schema,
  digest, and diff engine. Context continues to accept only live, ready
  `127.0.0.1` observations, so an unprobed or failed target cannot be published.
- Exercise real loopback sockets for concurrency, reachability/proxying,
  restart recovery, occupied recorded ports, range exhaustion, stopped-target
  retention, and selected-allocation removal. No Lima or shared Machine
  resource is required for this Linux contract slice.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run allocator race, restart, exhaustion, and TCP reachability tests

## Validation evidence

- `go test -race -count=10 ./internal/tcpallocator ./internal/state ./internal/plan ./internal/plandiff ./internal/contextdoc`: PASS. Four independent listener registries concurrently allocated eight endpoints without a duplicate Machine port; every endpoint proxied exact bytes through real loopback sockets. The suite also proved unreachable targets leave no row or listener, exact-port restart recovery, occupied-port refusal, stopped-target listener retention and later readiness refresh, range exhaustion without state mutation, and removal isolation between two Boxes.
- `make check`: PASS. The pinned `golangci-lint` reported `0 issues`; module verification, all unit tests, full race suite, vet, builds, spike smoke, shell checks, and Python automation tests passed.
- `go test -race -count=10 ./...`: PASS across every package after the final implementation.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`: PASS as compilation evidence only; no Darwin networking or Lima runtime behavior is claimed.
- `sprout check --all`: PASS for every active packet.
- `git diff --check`: PASS.
