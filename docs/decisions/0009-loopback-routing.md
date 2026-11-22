# ADR 0009: Route declared services through loopback-only Box endpoints

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 Machine HTTP and TCP exposure

## Context

Parallel Boxes need stable endpoints without privileged ports, Machine-wide DNS changes, or collisions. HTTP can multiplex by authority, while arbitrary TCP cannot. Repository-published ports would bypass daemon ownership and collide across Boxes. `.localhost` is reserved for loopback use, but resolver families and explicit proxy settings vary by platform.

## Decision

Expose only manifest-declared routes and ports on IPv4 loopback.

For HTTP, one user daemon listener defaults to `127.0.0.1:7777` and routes exact authorities of the form `<route>.<box-id>.abox.localhost:<port>`. It rejects malformed, unknown, duplicate, or suffix-only matches. It removes untrusted Agent Box control/forwarding headers and reconstructs trusted original host and scheme metadata.

Each Box has an in-VM gateway generated from the immutable Plan. The gateway joins the Box Compose network, resolves targets by service name, and listens on one fixed guest port. Lima maps that port to a unique daemon-reserved Machine loopback port; the Machine router forwards the selected Box request to that allocation. Container IPs are never persisted.

For raw TCP, the daemon reserves a kernel-selected port within the configured loopback range, holds the reservation through forward creation, persists it under the Box lease, verifies the target, and reports it through the API/context. TCP allocations remain stable while a Box is stopped. UDP is out of scope.

Preflight verifies exact-host resolver behavior and proxy bypass for `.abox.localhost`. A failed preflight blocks route creation before mutation. v0.1 provides plain HTTP, no managed CA, TLS termination, privileged ports, `/etc/hosts` edits, DNS daemon, IPv6 claim, or Machine-wide resolver mutation.

## Alternatives considered

- Publish repository container ports directly: rejected because collisions, authority, and cleanup escape the daemon.
- Allocate one Machine HTTP port per route: rejected because URLs become unstable and multiplexing is unnecessary duplication.
- Install a DNS server or edit `/etc/hosts`: rejected because it mutates Machine-wide configuration and needs elevated cleanup.
- Bind ports 80/443 and install a local CA: rejected because privileged listeners and trust-store mutation are not required for v0.1.
- Discover routes from Docker events or `EXPOSE`: rejected because exposure must be explicit user intent.

## Consequences

- Browsers and ordinary HTTP clients get stable hostnames while one daemon owns the listener.
- Explicit proxies can still break local routing; resolver/proxy preflight is mandatory, not documentation advice.
- The router and gateway must support streaming bodies, WebSockets, SSE, bounded headers, keep-alive, and timeouts without buffering unbounded data.
- Raw TCP consumes one Machine port per declaration and needs lease-aware recovery.
- Lima forwarding and Darwin resolver/browser behavior remain unverified until the hardware gate.

## Evidence

- [localhost routing spike](../features/spike-localhost-routing.md) passed exact-host fail-closed routing, authority preservation, trusted-header reconstruction, curl, Chromium, WebSocket, SSE, and bidirectional streaming on one unprivileged IPv4 Linux listener.
- [The substrate report](../architecture/substrate-spikes.md) records IPv4-only resolution and proves that `.localhost` bypasses a controlled proxy only when no-proxy behavior applies.
- [ADR 0001](0001-milestone-0-substrate-gate.md) keeps Lima forwarding, Darwin resolution, ambient proxies, and IPv6 outside the accepted evidence boundary.
- Two-VM forwarding collision and leak behavior belong to [`m0-lima-isolation-probe`](../../.sprout/tasks/m0-lima-isolation-probe.md) and the real-hardware gate.

## Revisit triggers

Supersede this ADR if:

- the real Darwin matrix fails exact `.abox.localhost` resolution, proxy bypass, browser routing, or two-VM static forwarding without collisions;
- an unknown/malformed authority reaches a Box, an inbound control header survives sanitization, or an undeclared endpoint becomes Machine-reachable;
- restart recovery changes a retained TCP allocation or leaks a listener/forward after ownership-verified removal;
- IPv6 becomes a support claim and passes resolver, bind, proxy, forwarding, collision, and cleanup parity tests; or
- real project adoption requires managed TLS or UDP and an explicit installation, trust, ownership, rollback, and conformance design is accepted.
