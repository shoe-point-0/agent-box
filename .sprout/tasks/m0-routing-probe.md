<!-- sprout-task
{"schema_version":1,"id":"m0-routing-probe","status":"open","created_at":"2026-08-29T06:48:35.876421469Z"}
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

- [ ] Requests reach only the selected Box backend by Host header.
- [ ] The router strips untrusted Agent Box control headers and preserves required forwarding metadata.
- [ ] Unsupported resolver or proxy cases are reported as compatibility limits.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run the two-host routing probe and protocol tests

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement prove wildcard localhost routing through one listener for Agent Box according to the cited specification and dependencies.
