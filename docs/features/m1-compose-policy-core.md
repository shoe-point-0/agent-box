<!-- sprout-task
{"schema_version":1,"id":"m1-compose-policy-core","status":"implemented","created_at":"2026-08-29T06:55:50.434296344Z","implemented_at":"2026-08-29T20:50:43.83028602Z"}
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

- [x] Rejected fixtures create no image build, container, network, volume, or listener.
- [x] Field-aware stable errors identify every forbidden primitive.
- [x] Live inspect tests prove allowed fixtures have the exact effective mount and privilege set.

## Technical approach

- Keep policy enforcement inside `internal/compose.Compile`, immediately after
  the final pinned `docker compose config --format json` normalization and
  before any generated artifact is published or any future lifecycle adapter
  can pull, build, create, or start the project.
- Decode the normalized service model into field-aware policy views while
  preserving canonical JSON retention. Reject enabled privileged mode; host
  network/PID/IPC/UTS/user/cgroup namespaces; capability additions; unsafe
  security options; devices and device cgroup rules; Docker API socket mounts;
  and mounts whose source, target, service, or access mode exceeds the exact
  immutable Plan projection. Treat `/proc`, `/sys`, `/dev`, and cgroup paths as
  forbidden mount boundaries regardless of mount type.
- Return deterministic typed policy violations carrying the service and JSON
  field path so callers can classify refusals without parsing prose.
- Prove refusal with a table spanning every primitive and asserting only the
  read-only version/config commands ran and no destination artifact appeared.
  Exercise the same fixtures through real pinned Compose normalization while
  comparing Docker resource identities before and after rejection. For the
  allowed fixture, apply only the already-validated generated files with pulls
  and builds disabled, inspect the disposable container's effective host
  configuration and mounts, then perform ownership-scoped teardown and verify
  absence.

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
- [x] Exercise policy refusal through real pinned Compose normalization
- [x] Inspect and clean up an allowed live Linux container

## Validation evidence

- `TestCompileRejectsForbiddenNormalizedPolicyBeforeApply` covers 24
  field-aware refusal cases: privileged mode; host network, PID, IPC, UTS,
  user, and cgroup namespaces; capability additions; unsafe seccomp, AppArmor,
  and SELinux options; devices and device cgroup rules; Docker API sockets;
  proc/sys/dev/cgroup mounts; undeclared or rewritten binds; access widening;
  shared propagation; and missing Plan binds. Each refusal returned a typed
  `PolicyViolation`, ran only version/config normalization, and published no
  generated artifact. `-count=100` passed 2,400 policy refusal cases.
- `TestCompileRejectsForbiddenPolicyWithRealComposeWithoutResources` passed on
  Ubuntu 24.04.4 x86_64 with Docker client/server 29.6.1 and Compose 5.3.0. The
  normalized malicious fixture was rejected at
  `services.workspace.privileged`; project-label queries before and after found
  no image, container, network, or volume, so no project listener existed.
- `TestAllowedPolicyMatchesLiveContainerInspect` compiled and applied only the
  validated generated files with `--no-build --pull never`, using the existing
  cached `alpine:3.20` fixture image. Live Docker inspection proved exactly the
  Plan-declared worktree and Git binds at their canonical targets and access
  modes; only `no-new-privileges=true`; no privilege, host namespace,
  capability, device, device-cgroup, or port-binding escalation; and all five
  ownership labels. Ownership-scoped `compose down -v --remove-orphans` left
  no project container, network, volume, or image. The live path passed all 10
  repetitions in the full race stress run.
- `go test ./...`, `go test -race ./...`, and `go vet ./...` passed across all
  16 packages. `make check` passed with 0 lint issues and successful module,
  test, race, vet, build, smoke, and automation gates.
  `go test -race -count=10 ./...` passed, including `internal/compose` in
  132.290s. `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as
  compilation evidence only, not Darwin runtime support.

## Outcome and follow-ups

- Added a deterministic pre-apply policy gate to both the repository-normalized
  model and the final Agent Box-adapted model. The first gate emits Agent Box's
  stable field error before unsafe repository fields can conflict with the
  generated override; the second proves the exact final bind and privilege
  projection before artifacts are retained or a lifecycle adapter can apply.
- Exported `PolicyViolation` as the typed service/field/reason contract for
  future daemon and CLI error mapping. No lifecycle operation was added here;
  the dependency-ready Compose adapter must consume only a successful compiler
  result.
- Broader unsafe Compose controls that are not part of this core packet remain
  assigned to `m5-compose-security`; the core gate is deny-by-default for
  capability additions and permits only `no-new-privileges=true` among
  normalized `security_opt` entries.

## Original request

Implement reject forbidden normalized compose capabilities before apply for Agent Box.
