<!-- sprout-task
{"schema_version":1,"id":"m0-routing-probe","status":"implemented","created_at":"2026-08-29T06:48:35.876421469Z","implemented_at":"2026-08-29T08:59:07.330570647Z"}
-->

# Prove wildcard localhost routing through one listener

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Prove wildcard localhost routing through one listener

## Context

- Milestone 0 Linux implementation probe. Dependencies: m0-spike-harness. Prove host routing against isolated local backends now; the complete Lima-forward path is rerun by m5-darwin-arm64-hardware-gate. Source: THE_BEGINNING.md Sections 13 and 19.2.

## Requirements

- Route two distinct Box hostnames under .abox.localhost through one unprivileged loopback HTTP listener.
- Exercise curl, a browser resolver path, IPv4/IPv6 behavior, WebSocket, SSE, and streaming.
- Record behavior under common VPN and proxy configurations where available.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Requests for two exact `.abox.localhost` authorities reach only the selected backend; unknown or malformed authorities fail closed.
- [x] The router removes every inbound `X-Abox-*` control header, injects only route-derived control identity, and rebuilds `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` from the trusted connection/request.
- [x] Real curl and headless-Chromium requests resolve through the same unprivileged IPv4 loopback listener; WebSocket, SSE, streaming request, and streaming response behavior pass protocol assertions.
- [x] IPv4/IPv6 resolver and listener behavior, active VPN-like interfaces, and explicit-proxy/bypass behavior are emitted as typed evidence; unexercised or unsupported cases remain explicit compatibility limits.

## Technical approach

- Add one disposable `localhost-routing` spike to the existing retained-evidence harness. It starts two isolated loopback backends and one `127.0.0.1:0` Machine-router listener, with an exact normalized-authority table for two `.abox.localhost` names.
- Use `httputil.ReverseProxy` with its `Rewrite` boundary: discard caller-supplied forwarding metadata and all `X-Abox-*` headers, rebuild forwarding metadata, preserve the original authority, then inject trusted Box/route identity. Reject unknown hosts and non-HTTP requests before proxying.
- Exercise the route with real curl and installed headless Chromium, plus protocol-level WebSocket, SSE, and bidirectional streaming checks. Use only ephemeral process-local listeners and an owned browser profile beneath the retained spike root.
- Inspect actual resolver results, loopback-family reachability, proxy environment presence, and active VPN-like interfaces. Run an explicit rejecting-proxy test and a `.localhost` bypass test without mutating Machine network configuration; report exact limits instead of extrapolating Darwin/Lima behavior.
- Emit an `abox.spike.localhost-routing/v1` JSON report and unit/integration tests. All servers close synchronously; cleanup removes only the harness-owned root after marker/inventory checks.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run the two-host routing probe and protocol tests

## Validation evidence

- On Linux 7.0.0-28-generic x86_64 with Go 1.26.3, `go run ./cmd/abox-spike run --parent /tmp localhost-routing` passed and retained an `abox.spike.localhost-routing/v1` report. One `127.0.0.1:40927` listener routed `alpha.box-a.abox.localhost` only to alpha and `beta.box-b.abox.localhost` only to beta; both observations reported correct trusted Box/route identity and forwarding metadata.
- curl 8.5.0 sent hostile `X-Abox-*`, `Forwarded`, and `X-Forwarded-*` values to both routes. Each backend observed no injected control/RFC `Forwarded` value, exact route-derived controls, the original authority (including listener port), `http` scheme, and only the real loopback peer in `X-Forwarded-For`. Unknown hosts returned 404 and malformed authorities returned 400 in tests.
- Chromium 151.0.7922.108 snap resolved and rendered the alpha route through the same listener using a marker-owned disposable profile. Protocol assertions passed for a WebSocket upgrade/echo, SSE event framing, request data reaching the backend before upload completion, and response data reaching the client before backend completion.
- The live resolver returned only `127.0.0.1`; IPv4 was reachable and IPv6 resolution/reachability were false for the intentionally IPv4-only listener. `tailscale0` was active while curl, Chromium, and protocol checks passed. No proxy environment was configured. A controlled explicit HTTP proxy intercepted the route, while curl's `.localhost` no-proxy bypass reached beta directly; this requirement is recorded as a compatibility limit.
- The first real run exposed a bad validation hypothesis: it compared forwarding metadata to the hostname without the ephemeral listener port. The router had correctly preserved the complete original authority. The validator now normalizes authority only for route selection while requiring `X-Forwarded-Host` to preserve the exact authority.
- The retained run root was ownership-cleaned after inspection. `make check`, `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed; cross-compilation is artifact evidence only and makes no Darwin runtime claim.

## Outcome and follow-ups

- Delivered the `localhost-routing` executable spike, typed retained report, exact-authority routing table, forwarding/control-header trust boundary, and protocol integration tests.
- Browser profile creation is journaled before use, guarded by an exact owner marker, removed synchronously after Chromium exits, and recoverable through scenario cleanup after interruption. In-process listeners shut down with bounded contexts and create no persistent Machine networking state.
- Linux curl, Chromium, WebSocket, SSE, streaming, active-Tailscale, IPv4 resolver, and controlled proxy behavior are proven. IPv6 wildcard resolution, ambient enterprise proxies, Lima forwarding, and Darwin resolver/browser behavior are explicit non-claims; unchanged final validation belongs to `m5-darwin-arm64-hardware-gate`.

## Original request

Implement prove wildcard localhost routing through one listener for Agent Box according to the cited specification and dependencies.
