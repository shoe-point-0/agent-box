# Agent Box

Agent Box is an early-stage local development runtime for humans and coding agents. Its intended unit of execution is a Box: one task workspace, one isolated Linux runtime host, one coordinated worktree set, and one container graph.

Status: pre-alpha foundation work. The repository does not yet provide `abox up`, a running daemon, Lima provisioning, or a supported runtime. The only production-binary behavior currently implemented is version reporting. Milestone 0 substrate probes remain available through `cmd/abox-spike` and are evidence harnesses, not product APIs.

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

Generated binaries are written to `bin/`. Run `make clean` to remove binaries and repository-local tools.

## Platform boundary

Linux is the implementation and continuous-verification lane. CI is configured to compile and test the platform-neutral foundation on a GitHub-hosted Apple Silicon macOS 15 runner; that remote job has not yet produced execution evidence for this foundation commit. The runner does not provide nested virtualization, so even a green job is not evidence that Lima VZ, VirtioFS, forwarding, or the Agent Box runtime works on macOS. Real Darwin/arm64 runtime support remains gated on the dedicated hardware acceptance tasks documented in `THE_BEGINNING.md`.

## Project documents

- Product and architecture baseline: [`THE_BEGINNING.md`](THE_BEGINNING.md)
- Measured substrate boundary: [`docs/architecture/substrate-spikes.md`](docs/architecture/substrate-spikes.md)
- Architecture decisions: [`docs/decisions/`](docs/decisions/README.md)
- Contribution guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security reporting policy and current private-channel gap: [`SECURITY.md`](SECURITY.md)
- Code of conduct: [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)

Agent Box is licensed under Apache-2.0. See [`LICENSE`](LICENSE).
