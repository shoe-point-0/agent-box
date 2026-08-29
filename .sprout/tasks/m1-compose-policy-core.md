<!-- sprout-task
{"schema_version":1,"id":"m1-compose-policy-core","status":"open","created_at":"2026-08-29T06:55:50.434296344Z"}
-->

# Reject forbidden normalized Compose capabilities before apply

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Reject forbidden normalized Compose capabilities before apply

## Context

- Milestone 1 security prerequisite. Dependencies: m1-compose-compiler. Source: THE_BEGINNING.md Sections 12 and 16.3.

## Requirements

- Validate normalized Compose JSON before pull, build, create, or start.
- Reject privileged mode, host namespaces, unsafe capabilities/security options, devices, Docker API sockets, sensitive proc/sys/dev/cgroup mounts, and bind sources outside the exact Plan allowlist.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Rejected fixtures create no image build, container, network, volume, or listener.
- [ ] Field-aware stable errors identify every forbidden primitive.
- [ ] Live inspect tests prove allowed fixtures have the exact effective mount and privilege set.

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

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement reject forbidden normalized compose capabilities before apply for Agent Box.
