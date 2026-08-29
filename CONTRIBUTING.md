# Contributing to Agent Box

Agent Box is pre-alpha. Contributions should improve measured behavior without widening support claims beyond evidence.

## Development setup

Install Git, GNU Make, and the Go version declared in `go.mod`. Then run:

```sh
make check
```

The first run downloads a pinned `golangci-lint` release into `.tools/bin`; it does not install tools globally. `make check` is the aggregate gate configured for Linux and Apple Silicon macOS CI. Individual targets are available through `make help`.

Before proposing a change:

1. Read `AGENTS.md` and the relevant architecture or Sprout packet.
2. Keep the change focused and preserve explicit platform, ownership, cleanup, and security boundaries.
3. Add tests for observable behavior and failure paths.
4. Run `make check` from the repository root.
5. Record real execution evidence; do not turn a cross-build or mock into a support claim.

Use `make clean` to remove `bin/` and `.tools/`.

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

## Commits and pull requests

Use focused conventional commits such as `feat(cli): report build version` or `docs(security): define disclosure path`. Pull requests should explain the boundary changed, tests run, cleanup behavior, and any remaining non-claims.

By participating, you agree to follow [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Follow [`SECURITY.md`](SECURITY.md) for vulnerability handling; the project does not currently have an operational private disclosure channel, so never put vulnerability details in a public issue.
