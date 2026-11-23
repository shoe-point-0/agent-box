# Contributing to Agent Box

Agent Box is pre-alpha. Contributions should improve measured behavior without widening support claims beyond evidence.

## Development setup

Install Git, GNU Make, and the Go version declared in `go.mod`. Then run:

```sh
make check
```

The first run downloads a pinned `golangci-lint` release into `.tools/bin`; it does not install tools globally. `make check` is the aggregate gate configured for Linux and Apple Silicon macOS CI. Individual targets are available through `make help`.

Before proposing a change:

1. Read `AGENTS.md` and the relevant architecture or feature documentation.
2. Keep the change focused and preserve explicit platform, ownership, cleanup, and security boundaries.
3. Add tests for observable behavior and failure paths.
4. Run `make check` from the repository root.
5. Sign off every commit under the DCO policy below.
6. Record real execution evidence; do not turn a cross-build or mock into a support claim.

Use `make clean` to remove `bin/` and `.tools/`.

## Feature documentation

Implemented components are documented as reader-facing references under `docs/features/` using descriptive kebab-case filenames and indexed from [docs/features.md](docs/features.md). When adding or changing a delivered capability, document its behavior, scope, constraints, implementation details, verified acceptance criteria, and recorded verification evidence, then run `make automation-check` to validate links and documentation structure. Never document unimplemented or unverified capabilities as shipped behavior.

## Go conventions

- Prefer the standard library and narrow internal packages.
- Pass `context.Context` through cancellable work and own every goroutine and external process lifecycle.
- Keep process invocation in leaf adapters with argv arrays, bounded output, cancellation, and typed errors.
- Define interfaces where they are consumed and add abstractions only when a concrete slice earns them.
- Format with `make fmt`; do not bypass lint, race, or vet failures.

## Tests and platform claims

Unit, race, vet, lint, and build checks must pass on Linux. Platform-neutral code is also checked on GitHub-hosted Apple Silicon macOS. Lima VZ, VirtioFS, and Darwin runtime claims require the dedicated real-hardware acceptance gate; CI compilation alone is not enough.

Tests that create VMs, containers, networks, mounts, worktrees, or temporary roots must use unique ownership markers and deterministic cleanup. Destructive tests may target only resources they created and can prove they own.

## Architecture decisions

Changes to normative architecture, compatibility, security, persistence, or destructive lifecycle behavior require an ADR. Follow [`docs/decisions/README.md`](docs/decisions/README.md). Small implementation choices that do not alter a durable contract do not need an ADR.

## Issues, pull requests, and reviews

An issue is welcome for bugs, design discussion, and work that needs coordination, but it is not required for a focused fix. Search existing issues first. Include the observed behavior, expected behavior, minimal reproduction, platform and tool versions, and sanitized diagnostics. Never report vulnerabilities or private conduct details in a public issue; use the processes linked below.

Open a draft pull request early when a change affects public contracts or needs design feedback. A ready pull request must:

- explain the boundary changed, motivation, tests run, cleanup behavior, and remaining non-claims;
- link its issue or ADR when one is required;
- keep generated files, dependency changes, and unrelated cleanup out unless required;
- pass `make check` and every additional platform gate needed for its claim; and
- answer blocking review findings or document why they no longer apply.

Authors do not approve their own changes. Maintainers classify feedback as blocking only for correctness, security, compatibility, lifecycle, governance, or agreed scope; preferences should be labeled non-blocking. The approval, conflict, bootstrap, and merge rules are in [`GOVERNANCE.md`](GOVERNANCE.md).

## Developer Certificate of Origin

Agent Box uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/), not a Contributor License Agreement. Every commit must include a `Signed-off-by` trailer certifying that the contributor has the right to submit the work under the project license:

```sh
git commit -s
```

The trailer must use a name and email the contributor is authorized to certify. Sign-off is a legal attestation, not a GPG or SSH signature. Maintainers verify it during review; automated DCO enforcement is not yet configured, so missing trailers must be fixed before merge without rewriting another contributor's attestation for them.

## Commits

Use focused conventional commits such as `feat(cli): report build version` or `docs(security): define disclosure path`.

By participating, you agree to follow [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Follow [`SECURITY.md`](SECURITY.md) for vulnerability handling; the project does not currently have an operational private disclosure channel, so never put vulnerability details in a public issue. Release and compatibility changes must also follow [`RELEASES.md`](RELEASES.md) and [`COMPATIBILITY.md`](COMPATIBILITY.md).
