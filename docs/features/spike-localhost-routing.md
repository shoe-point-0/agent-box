<!-- sprout-task
{"schema_version":1,"id":"m0-routing-probe","status":"implemented","created_at":"2026-08-29T06:48:35.876421469Z","implemented_at":"2026-08-29T08:59:07.330570647Z"}
-->

# Wildcard localhost routing through one listener

Status: implemented.

The `localhost-routing` spike scenario verifies that one unprivileged IPv4
loopback HTTP listener can route multiple exact `*.abox.localhost` hostnames to
isolated backends while enforcing a fail-closed header trust boundary and
supporting curl, headless Chromium, WebSocket, SSE, and bidirectional
streaming. Results are emitted as typed `abox.spike.localhost-routing/v1` JSON
evidence.

This Linux probe proves local host routing only. IPv6 wildcard resolution,
ambient enterprise proxies, Lima forwarding, and Darwin resolver/browser
behavior remain explicit non-claims deferred to `m5-darwin-arm64-hardware-gate`.

## Scope and dependencies

- Milestone 0 Linux implementation probe. Depends on [spike harness](spike-harness.md). Source: `THE_BEGINNING.md` Sections 13 and 19.2.

## Behavior

- Routes two exact `.abox.localhost` authorities (`alpha.box-a.abox.localhost` and `beta.box-b.abox.localhost`) through one unprivileged `127.0.0.1` listener, failing closed on unknown (`404`) or malformed (`400`) authorities.
- Strips all caller-supplied `X-Abox-*`, `Forwarded`, and `X-Forwarded-*` headers, preserves the original request authority (including port) in `X-Forwarded-Host`, rebuilds `X-Forwarded-For` and `X-Forwarded-Proto` from the trusted connection, and injects route-derived Box identity headers.
- Exercises curl, headless Chromium, WebSocket upgrade/echo, SSE framing, streaming request/response delivery, resolver/interface inspection, and controlled proxy bypass checks.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Requests for two exact `.abox.localhost` authorities reach only the selected backend; unknown or malformed authorities fail closed.
- The router removes every inbound `X-Abox-*` control header, injects only route-derived control identity, and rebuilds `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` from trusted connection metadata.
- Real curl and headless Chromium requests resolve through the same unprivileged IPv4 loopback listener, and WebSocket, SSE, and streaming request/response assertions pass.
- IPv4/IPv6 resolver behavior, active VPN-like interfaces, and proxy/bypass limits are recorded as typed evidence without overclaiming unexercised environments.

## Implementation details

- `localhost-routing` starts two isolated loopback backends and one `127.0.0.1:0` `httputil.ReverseProxy` listener backed by an exact normalized-authority routing table.
- The proxy's `Rewrite` hook normalizes authority strictly for route lookup while preserving the full original `Host` (including ephemeral port) in `X-Forwarded-Host`, discarding spoofed forwarding and `X-Abox-*` headers before injecting trusted route identity.
- Headless Chromium uses an owner-marked disposable profile beneath the retained spike root that is journaled before creation, removed synchronously on exit, and recoverable by scenario cleanup after interruption. All listeners shut down cleanly with bounded contexts and leave no persistent Machine networking state.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run the two-host routing probe and protocol tests

## Validation evidence

- Passed on Linux `7.0.0-28-generic` `x86_64` with Go `1.26.3`, curl `8.5.0`, and Chromium `151.0.7922.108` snap via `go run ./cmd/abox-spike run --parent /tmp localhost-routing`, retaining an `abox.spike.localhost-routing/v1` report.
- One `127.0.0.1:40927` listener routed each authority exclusively to its target backend; hostile `X-Abox-*`, `Forwarded`, and `X-Forwarded-*` headers sent via curl were stripped and replaced with exact route identity, original authority with port, `http` scheme, and loopback peer address.
- Headless Chromium rendered the alpha route through the same listener; WebSocket upgrade/echo, SSE framing, pre-completion upload streaming, and pre-completion response streaming all passed.
- Resolver diagnostics showed IPv4 `127.0.0.1` resolution with `tailscale0` active and IPv6 disabled on the intentionally IPv4-only listener; an explicit HTTP proxy intercepted `.abox.localhost` unless `.localhost` no-proxy bypass was configured (recorded as a compatibility requirement).
- Ownership-verified cleanup removed the retained run root. `make check`, `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (artifact evidence only) passed.
