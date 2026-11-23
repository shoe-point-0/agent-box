# Versioned daemon API over a Unix socket

Status: implemented.

The `internal/daemon` package and `cmd/aboxd` serve path provide the local
Unix-socket control plane for Agent Box. The daemon listens exclusively on a
user-owned mode-`0600` Unix socket inside a mode-`0700` runtime directory (no
TCP listener), authenticates every connection via kernel peer credentials
before HTTP parsing, negotiates versioned API ranges, enforces durable
idempotency through the operation journal, and streams sequenced NDJSON events
so CLI disconnects detach without interrupting daemon-owned work.

When the daemon transport first landed, Box mutation handlers returned a typed
`ABOX_FEATURE_NOT_SUPPORTED` placeholder; single-repository `up` and `rm`
runners were subsequently wired in [`abox up`](abox-up.md)
and [`abox rm`](abox-rm.md), and Apple Silicon (`darwin/arm64`)
`LOCAL_PEERCRED` socket lifecycle and detached execution were exercised on real
hardware in [`abox up`](abox-up.md) (full multi-version
Darwin release matrix remains [open](../../ROADMAP.md#milestone-5-releasable-v01-candidate)). A running
operation left behind by a dead daemon fails closed with a retryable operation
conflict; adoption and reconstruction remain [Milestone 4 work](../../ROADMAP.md#milestone-4-recovery-and-fault-containment).

## Scope and dependencies

- Milestone 1 daemon control-plane transport. Depends on [operation journal](operation-journal.md) and [reconciliation engine](reconciliation-engine.md). Source: `THE_BEGINNING.md` Sections 6.1 and 14.5.

## Behavior

- Serves HTTP+JSON and NDJSON operations from a user-owned mode-`0700` runtime directory over a mode-`0600` Unix socket with no-follow path checks, a restrictive creation umask, an exclusive kernel-backed `flock` daemon lock, stale-socket refusal, and post-bind UID, file-type, and mode verification.
- Authenticates every accepted connection using kernel peer credentials (`SO_PEERCRED` on Linux, `LOCAL_PEERCRED`/`Xucred` on macOS) before HTTP parsing, closing connections on credential error or UID mismatch.
- Negotiates integer API ranges before dispatch, returning `ABOX_API_INCOMPATIBLE` for disjoint ranges.
- Enforces bounded strict JSON decoding, lossless canonical request digests, durable idempotency keys via the operation journal, same-daemon attachment, and resumable sequenced NDJSON streaming where client disconnect detaches rather than cancels work.
- Owns startup, signal-bounded graceful shutdown, active request draining, exact-inode socket cleanup, and retained lock exclusion if a shutdown deadline expires.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Never opens a TCP listener and never unlinks a pre-existing socket whose stale ownership cannot be proven.

## Verified acceptance criteria

- Compatible clients can start, detach from, and reattach to a single long-running operation without duplicate execution.
- Incompatible API ranges return stable `ABOX_API_INCOMPATIBLE` errors.
- Connections from other local users are rejected at the transport boundary.

## Implementation details

- `internal/daemon` owns the HTTP server, Unix listener, active request set, shutdown deadline, exact-inode socket unlink, and exclusive advisory lock for the full daemon lifetime while keeping platform path selection outside the transport package.
- Runtime directory, lock file, and socket setup use `lstat` and no-follow opens, owner-only modes, a restrictive creation umask, and post-bind UID/type/mode validation; any pre-existing socket causes startup refusal.
- OS-specific peer-credential checks live in build-tagged files (`SO_PEERCRED` on Linux and `LOCAL_PEERCRED`/`Xucred` on Darwin).
- Mutation handlers require bounded idempotency keys, preserve distinct large JSON integers in canonical request digests, persist monotonic events in `internal/journal`, and stream NDJSON starting from any caller-supplied sequence number.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run Unix-socket permission, handshake, streaming, and shutdown integration tests

## Validation evidence

- `go test -race -count=20 ./cmd/aboxd ./internal/daemon` passed against a live Linux `aboxd` Unix socket, verifying owner UID, `0700` runtime and `0600` socket modes, rejection of symlinks, stale endpoints, second lock owners, and foreign-UID credentials, API range negotiation, duplicate-key reattachment without duplicate execution, digest conflict detection (including large JSON integers), resumable NDJSON streaming, detach, cancellation, shutdown timeout, retry, and exact-inode socket cleanup.
- `go test -race -count=10 ./internal/journal` passed, covering persisted operation/event restart refusal and transport operation lookup.
- `make check` (`0` lint issues), `go test -race -count=10 ./...` (including `internal/daemon` in `11.800s` and `internal/journal` in `54.578s`), and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed.
- Live Apple Silicon (`darwin/arm64`) Unix-socket handshake, `LOCAL_PEERCRED`, and detached `SIGKILL` continuity were subsequently verified on real M1 hardware in [`abox up`](abox-up.md) and [apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md).
