# Agent Box

Agent Box is an early-stage local development runtime for humans and coding agents. Its intended unit of execution is a Box: one task workspace, one isolated Linux runtime host, one coordinated worktree set, and one container graph.

Status: pre-alpha. The CLI implements `init`, `doctor`, `plan`, `up`, `list`, and refusal-first `rm`, backed by a user-scoped daemon and runtime adapters. This is not yet a turnkey supported runtime: provisioning requires an explicit verified artifact installation and matching preinstalled guest image, and the production preflight currently admits only Darwin/arm64 with VZ. A successful build or Linux test run does not establish runtime support. Milestone 0 substrate probes remain available through `cmd/abox-spike` and are evidence harnesses, not product APIs.

Start with [`docs/getting-started.md`](docs/getting-started.md) for installation, command syntax, daemon configuration, and the current runtime blockers.

## Development quick start

Prerequisites:

- Git
- GNU Make
- Go 1.26.3 (the version declared by `go.mod`)
- network access on the first `make check`, so the pinned linter can be installed into `.tools/bin`

From a clean clone:

```sh
make check
make build
./bin/abox --version
./bin/aboxd --version
```

`make check` is the local and CI quality gate. It verifies formatting, lint, module metadata, unit tests, race tests, vet, both product builds, the retained spike smoke path, and repository automation.

Generated binaries are written to `bin/`. Install both commands for your user with `make install` (default `~/.local/bin`), or use `make install INSTALL_DIR="$HOME/bin"`. Put the selected directory on `PATH`. This installs the CLI and daemon only, not Lima, Docker, or a guest image. Run `make clean` to remove build outputs and repository-local tools.

## Platform boundary

Linux is the implementation and continuous-verification lane, not a supported production `abox up` host. Production preflight currently permits only Darwin/arm64 with VZ, and the Lima configuration adapter requires `lima-vz` with VirtioFS. CI is configured to run the quality gate on Ubuntu 24.04 and macOS 15; a green hosted job is not evidence that Lima VZ, VirtioFS, forwarding, or the Agent Box runtime works on real macOS hardware. Real Darwin/arm64 runtime support remains gated on the dedicated hardware acceptance tasks documented in `THE_BEGINNING.md`. See the [getting-started platform boundary](docs/getting-started.md#current-platform-boundary) before attempting provisioning.

## Project documents

- Product and architecture baseline: [`THE_BEGINNING.md`](THE_BEGINNING.md)
- Roadmap and evidence gates: [`ROADMAP.md`](ROADMAP.md)
- Implemented components and validation evidence: [`docs/features.md`](docs/features.md)
- Measured substrate boundary: [`docs/architecture/substrate-spikes.md`](docs/architecture/substrate-spikes.md)
- Architecture decisions: [`docs/decisions/`](docs/decisions/README.md)
- Contribution guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Governance and decision process: [`GOVERNANCE.md`](GOVERNANCE.md)
- Maintainer and reviewer roster: [`MAINTAINERS.md`](MAINTAINERS.md)
- Release policy: [`RELEASES.md`](RELEASES.md)
- Compatibility and deprecation policy: [`COMPATIBILITY.md`](COMPATIBILITY.md)
- Telemetry policy: [`TELEMETRY.md`](TELEMETRY.md)
- Adoption and project-health measures: [`PROJECT_HEALTH.md`](PROJECT_HEALTH.md)
- Name and trademark review status: [`TRADEMARKS.md`](TRADEMARKS.md)
- Security reporting policy and current private-channel gap: [`SECURITY.md`](SECURITY.md)
- Code of conduct: [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)

Agent Box is licensed under Apache-2.0. See [`LICENSE`](LICENSE).
