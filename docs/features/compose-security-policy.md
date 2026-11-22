<!-- sprout-task
{"schema_version":1,"id":"m1-compose-policy-core","status":"implemented","created_at":"2026-08-29T06:55:50.434296344Z","implemented_at":"2026-08-29T20:50:43.83028602Z"}
-->

# Pre-apply Compose security policy

Status: implemented.

The Compose compiler in `internal/compose` enforces a deterministic pre-apply
security policy gate on both the repository-normalized Compose model and the
final Agent Box-adapted model before any artifact is retained or any lifecycle
operation can pull, build, create, or start containers. Running the gate at
both stages ensures unsafe repository fields emit Agent Box's stable field
diagnostics before colliding with generated overrides, and proves the final
effective mount and privilege set before execution.

Policy rejections surface as typed `PolicyViolation` values carrying service,
JSON field path, and reason so [Compose runtime adapter](compose-runtime.md) and
higher control-plane layers classify refusals without parsing prose. The core
gate is deny-by-default for capability additions and permits only
`no-new-privileges=true` in normalized `security_opt`; broader hardening rules
remain tracked in `m5-compose-security`.

## Scope and dependencies

- Milestone 1 security prerequisite. Depends on [Compose compiler](compose-compiler.md). Source: `THE_BEGINNING.md` Sections 12 and 16.3.

## Behavior

- Validates normalized `docker compose config --format json` output prior to artifact publication and before any `pull`, `build`, `create`, or `start` invocation.
- Rejects `privileged: true`, host namespaces (`network`, `pid`, `ipc`, `uts`, `user`, `cgroup`), capability additions, unsafe `security_opt` entries (permitting only `no-new-privileges=true`), `devices` and `device_cgroup_rules`, Docker API socket mounts, `/proc`, `/sys`, `/dev`, and `cgroup` mounts regardless of mount type, and any bind mount whose source, target, service, propagation, or access mode deviates from the immutable Plan allowlist.
- Emits deterministic typed `PolicyViolation` errors identifying the offending service and JSON field path.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Policy refusal occurs solely after read-only version and `config` normalization, creating zero images, containers, networks, volumes, listeners, or retained artifacts.

## Verified acceptance criteria

- Rejected fixtures create no image build, container, network, volume, or listener.
- Stable field-aware `PolicyViolation` errors identify every forbidden primitive.
- Live container inspection proves allowed fixtures run with the exact Plan-declared mount and privilege set.

## Implementation details

- `internal/compose.Compile` decodes normalized Compose JSON into field-aware policy views while preserving canonical JSON retention.
- Both pre-override repository normalization and post-override final normalization must pass policy validation before any generated file is atomically published or passed to [Compose runtime adapter](compose-runtime.md).

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Exercise policy refusal through real pinned Compose normalization
- [x] Inspect and clean up an allowed live Linux container

## Validation evidence

- `TestCompileRejectsForbiddenNormalizedPolicyBeforeApply` verified `24` field-aware refusal cases (privileged mode, all six host namespaces, capability additions, unsafe seccomp/AppArmor/SELinux options, devices, device cgroup rules, Docker API sockets, `/proc`/`/sys`/`/dev`/cgroup mounts, undeclared or rewritten binds, access widening, shared propagation, and missing Plan binds), passing `2,400` cases (`-count=100`) with only read-only version/config commands executed and zero published artifacts.
- `TestCompileRejectsForbiddenPolicyWithRealComposeWithoutResources` passed on Ubuntu `24.04.4` x86_64 with Docker `29.6.1` and Compose `5.3.0`: a privileged fixture failed at `services.workspace.privileged`, and before/after Docker project-label queries found zero images, containers, networks, or volumes.
- `TestAllowedPolicyMatchesLiveContainerInspect` applied a validated fixture with `--no-build --pull never` on cached `alpine:3.20`; live `docker inspect` confirmed exact Plan-declared worktree/Git binds and access modes, only `no-new-privileges=true`, zero privilege/namespace/capability/device/port escalations, and all five ownership labels, followed by clean teardown (`compose down -v --remove-orphans`).
- `make check` (`0` lint issues across 16 packages), `go test -race -count=10 ./...` (`internal/compose` in `132.290s`), and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed.
