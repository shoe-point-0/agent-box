# ADR 0006: Keep service graphs in Compose and inject Box policy through generated files

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 service composition and runtime ownership

## Context

Repositories already express images, builds, dependencies, health checks, networks, secrets, volumes, and restart behavior in Compose. Restating those concepts in the Agent Box manifest would create a weaker competing service model. Multi-repository composition still needs deterministic path bases, Box identity, mount policy, port suppression, labels, and a gateway.

## Decision

Repository-owned Compose files remain authoritative for service graphs. Agent Box generates:

1. one root Compose file using `include`, with each include's project directory fixed to its owning repository; and
2. one inspectable override that injects only Box policy: ownership labels, Box network attachment, declared worktree/cache mounts, gateway, readiness metadata, secret delivery, and removal of repository-owned Machine port publications.

Every invocation supplies an explicit project name derived from the Box ID. Generated resources carry managed, workspace, Box, Plan, and generation labels. Cleanup selects recorded resource IDs and verifies labels; a project or name prefix is not ownership proof.

Before apply, the adapter verifies the pinned Compose version and compiles the full normalized model. It must reject service-name conflicts, path escapes, forbidden privilege/socket/namespace/device settings, surviving Machine publications, or normalization that does not clear ports with `!reset []`. The redacted normalized model and input digests become Plan evidence. Docker Engine and Compose run inside the Box VM.

Agent Box does not silently translate arbitrary Dev Container metadata or discover public routes from `EXPOSE`.

## Alternatives considered

- Duplicate Compose fields in `.abox/box.yaml`: rejected because it creates two owners and an incomplete Compose implementation.
- Stack unrelated files with repeated `-f`: rejected because relative path bases become ambiguous across repositories.
- Rewrite repository Compose files: rejected because generated policy must be reviewable and repository files must remain untouched.
- Accept arbitrary Compose versions: rejected because port reset and include normalization are safety-critical capabilities.
- Replace Compose with Kubernetes: rejected as needless distributed-system machinery for a local v0.1 runtime.

## Consequences

- Existing repository workflows remain useful and each repository owns its path base.
- Compose service names must be globally unique in the assembled project.
- Version/capability preflight and normalized-model inspection are release-blocking safety checks.
- Some valid standalone Compose projects are intentionally rejected when they violate the Box boundary.
- Generated files are deterministic, redacted, and inspectable; secret values never enter the stored model.

## Evidence

- [`m0-compose-probe`](../features/m0-compose-probe.md) passed two-repository `include`, repository-relative build contexts, identity-specific images, generated override application, `!reset []` port removal, ownership checks, and cleanup on Docker Compose 5.3.0.
- [The substrate report](../architecture/substrate-spikes.md) limits that result to Compose 5.3.0 and the local Linux daemon.
- Docker documents separate path bases and conflict detection for [`include`](https://docs.docker.com/reference/compose-file/include) and clearing model values with [`!reset`](https://docs.docker.com/reference/compose-file/merge/#reset-value).
- [ADR 0001](0001-milestone-0-substrate-gate.md) requires capability checks before mutation.

## Revisit triggers

Supersede this ADR if:

- a supported Compose release changes normalized `include`, project-directory, label, or `!reset []` behavior and the retained fixture turns red;
- any apply path publishes an undeclared Machine port or creates an unlabeled Box resource;
- the multi-repository acceptance fixture cannot preserve repository-relative build/config paths without editing repository files;
- a required development workload cannot be represented by the supported Compose contract and has at least two adoption cases plus a passing lifecycle/cleanup prototype; or
- an alternate container engine enters supported scope and passes the same normalized-model, isolation, readiness, and cleanup conformance suite.
