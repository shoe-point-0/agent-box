# ADR 0007: Expose one versioned daemon API over a user Unix socket

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 CLI, daemon, and local control-plane boundary

## Context

Long-lived routing, serialized reconciliation, leases, operation reattachment, and recovery cannot be owned reliably by a short-lived CLI process. Agent integrations need a stable machine contract, but a local single-user runtime does not justify a network service, cluster control plane, or vendor-specific privileged API.

## Decision

`abox` is a thin client to a user-scoped `aboxd`. The daemon serves a versioned HTTP+JSON API only on a filesystem Unix socket created with mode `0600` in the platform runtime directory. It does not listen on TCP.

Where the platform exposes Unix peer credentials, the daemon rejects a peer whose effective user does not match its own. Socket permissions and peer identity are both enforced; neither grants authorization to mutate resources owned by another daemon instance.

Mutating requests carry caller-generated idempotency keys. The daemon persists the operation identity before side effects, serializes mutation under a Box lease, and returns or reattaches to the existing operation on a duplicate key with the same request digest. Reuse with different input is a conflict.

Long operations expose monotonically sequenced NDJSON events and a final typed result. Human progress goes to stderr; JSON/NDJSON stdout remains parseable. Errors have stable symbolic codes and mapped exit classes. API compatibility is explicit; incompatible clients fail before mutation.

The daemon owns Git, Lima, Compose, state, ports, routing, redaction, and reconciliation. The CLI does not bypass it when it is available. MCP, IDE, or vendor adapters may wrap the same API later and receive no privileged side channel.

## Alternatives considered

- Put all lifecycle logic in the CLI: rejected because client death would own long-running resources, routing, and recovery.
- Listen on loopback TCP: rejected because it expands discovery, authentication, firewall, and accidental exposure concerns without a remote-use requirement.
- Use gRPC or a custom binary protocol: deferred because versioned HTTP+JSON and NDJSON cover the v0.1 request and stream contract with standard tooling.
- Make MCP the core API: rejected because it couples runtime semantics to one agent integration layer.
- Add distributed coordination: rejected because the state and resources are local to one Machine user.

## Consequences

- The daemon has explicit startup, stale-socket, duplicate-instance, shutdown, and lease-recovery behavior.
- Client cancellation detaches from an operation unless the API explicitly requests operation cancellation; it does not imply rollback.
- Socket path length, permissions, ownership, and stale endpoint handling require platform tests.
- API schemas and symbolic errors become compatibility commitments once released.
- User-account compromise remains outside this boundary; Unix socket checks do not protect against the same Machine user.

## Evidence

- [The failure-matrix probe](../features/m0-failure-matrix.md) passed stable operation IDs, independently observed residue, idempotent adoption, refusal, and reverse-order cleanup for Linux substrate operations. It did not implement the daemon API.
- [ADR 0001](0001-milestone-0-substrate-gate.md) requires observed-state reconciliation after interrupted clients and processes.
- Go's [`net`](https://pkg.go.dev/net#ListenUnix) and [`net/http`](https://pkg.go.dev/net/http#Server) packages provide Unix listeners and HTTP serving. Peer-credential enforcement remains platform-specific implementation work.
- API behavior will be exercised by `m1-daemon-api`, `m1-core-commands`, and Milestone 4 daemon/routing fault tasks; no current runtime-support claim is made.

## Revisit triggers

Supersede this ADR if:

- tests show socket mode and peer credentials cannot prevent a different local user from issuing a request on any supported platform;
- duplicate idempotency keys or reconnects can create two live operations or duplicate external resources;
- the event stream cannot resume without gaps or duplication from its persisted sequence after daemon restart;
- a measured remote-control requirement has at least two real integrations and a threat model that justifies authenticated transport; or
- HTTP+JSON encoding or streaming overhead consumes more than the documented control-plane latency or memory budget under the concurrency fixture.
