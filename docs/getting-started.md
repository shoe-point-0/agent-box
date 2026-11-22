# Getting started

Agent Box is pre-alpha. The CLI and daemon implement real lifecycle paths, but this checkout is **not a turnkey runtime distribution**. Building the binaries, installing `limactl`, or starting a host Docker daemon does not supply the verified guest installation required by `plan` and `up`. No supported downloadable Agent Box guest bundle is supplied by this guide; artifact packaging and hardware acceptance remain release gates in the [roadmap](../ROADMAP.md).

## Build and install the commands

Build prerequisites are Git, GNU Make, and Go 1.26.3 as declared in [`go.mod`](../go.mod). Dependency downloads require network access. The full `make check` gate also uses Bash and Python 3, installs the pinned linter into `.tools/bin`, and runs race tests (which require a working C toolchain).

From the repository root:

```sh
make build
./bin/abox --version
./bin/aboxd --version
make install
export PATH="$HOME/.local/bin:$PATH"
abox --help
```

`make install` builds and installs **both** `abox` and `aboxd` into `~/.local/bin`, without requiring sudo. To choose another directory:

```sh
make install INSTALL_DIR="$HOME/bin"
export PATH="$HOME/bin:$PATH"
```

Add the appropriate `PATH` export to your shell startup file if needed. Alternatively, use `./bin/abox` directly; it finds the adjacent `./bin/aboxd`. Installation does not register a system service or install runtime dependencies. `make check` is the development quality gate, not proof of VM readiness.

## Three separate prerequisite layers

| Layer | What is needed | What it establishes |
| --- | --- | --- |
| Build tools | Git, Make, Go; Bash, Python and C toolchain for the complete checks | Ability to build and test the commands |
| Host tools | Git; Lima `limactl` and its platform prerequisites for runtime operations; optional host Docker/Compose for diagnostics | Host capabilities, not the contents or readiness of a Box guest |
| Runtime installation | Explicit `installation.json`, verified local artifacts, and a matching preinstalled Linux guest image | Inputs for production plan/apply and guest verification |

For Lima installation consult [Lima's installation documentation](https://lima-vm.io/docs/installation/). The current adapter pins Lima **2.0.0**; installing an arbitrary newer version is not a compatibility guarantee. The daemon uses the artifact path recorded in installation metadata, not merely whichever `limactl` appears first on `PATH`.

Run the read-only diagnostic command:

```sh
abox doctor
abox doctor --json
```

Doctor probes host Git, Lima, Docker and Compose versions and observes available evidence. It does not start the daemon, download artifacts, repair configuration, create a VM, or certify guest readiness. A missing host Docker runtime is distinct from missing Docker inside a Box. Installing Docker Desktop or starting host Docker does not replace the required guest Docker service. Host tool version success also does not establish the pinned artifact digests. Unknown/advisory evidence is not a runtime acceptance result.

## Current platform boundary

- Linux is the implementation/test lane. Production `up` preflight currently allows only `darwin/arm64` with accelerator `vz`, so installing Lima or QEMU on Linux does **not** unlock production provisioning.
- The Lima configuration generator requires driver `lima-vz` and uses VirtioFS. Although the daemon selects the name `lima-qemu` on non-Darwin hosts, that is not a completed Linux backend.
- Darwin/arm64 is the intended product lane. The single-repository Milestone 1 slice (`abox up` cold READY, warm no-op, detached CLI completion, and refusal-first `abox rm`) has passed real Apple M1 VZ validation ([Apple Silicon M1 gate report](architecture/apple-silicon-m1-gate.md)). Broad multi-version macOS release support, two-VM forwarding checks, and full fault recovery remain gated on Milestone 5. Hosted build/test CI does not establish those results.
- Windows and Intel macOS are not production runtime targets in the current preflight.

These limits are enforced in [`cmd/aboxd/production_up.go`](../cmd/aboxd/production_up.go) and [`internal/lima/config.go`](../internal/lima/config.go). The [substrate evidence](architecture/substrate-spikes.md), [Apple Silicon M1 gate report](architecture/apple-silicon-m1-gate.md), and [architecture baseline](../THE_BEGINNING.md) describe the measured and remaining acceptance boundaries.

## Filesystem behavior on Apple Silicon

The current readiness probe allows up to two seconds for changed bytes, modes,
renames, and deletions to become visible across the Machine/guest boundary.
Lima's experimental event forwarding is enabled for writable mounts. It
synthesizes Linux `IN_ATTRIB` events for host creates and modifies without
changing file contents or modes. A workspace watcher must still receive the
tested create/modify event within one second. Host deletion events are not
forwarded; configure your project's watcher to poll if it requires deletion
events or ignores `IN_ATTRIB`. See [the filesystem decision](decisions/0011-bounded-filesystem-visibility.md)
for the measured reason and acceptance limits.

Instances generated with the earlier mount or forwarding configuration are
refused rather than silently changed. Preserve your work and review the removal
preview before recreating an existing Box.

## Create a workspace manifest

In the repository you want to use as a workspace:

```sh
abox init
```

Exact syntax is `abox init [--force]`: there is no positional directory argument. It writes `.abox/box.yaml` in the current directory without contacting the daemon or provisioning anything. It refuses an existing manifest; use `--force` only when intentionally replacing it.

The starter is a template, not a discovered application configuration. Review and edit it before planning:

- `source.local: ..` resolves relative to the manifest directory, so `.abox/box.yaml` points at the containing repository.
- The checkout starts at `HEAD` and creates branch `abox/{{ box.task }}/repository` in the managed worktree.
- It expects `compose.yaml`, profile `development`, and workspace service `app`; change these to match your committed repository contents.
- It requests 2 CPUs, 4 GiB memory, and a 20 GiB disk and declares the workspace mount and policy settings.

`init` does not create a Compose application, commit files, or validate that those services exist. See the [schema contracts](../schemas/v1alpha1/) and [implemented features](features.md) for the manifest and policy boundaries.

## Configure the runtime installation

**Stop before `plan` or `up` if you do not have a real matching artifact set and guest image.** The loader does not download or install anything. Test fixtures with synthetic artifacts are not usable guest distributions.

The default metadata filename is `installation.json` beside the daemon's `state.db`. Its strict JSON contract is implemented in [`internal/installation/installation.go`](../internal/installation/installation.go):

| Field | Required contents |
| --- | --- |
| `schemaVersion` | `abox.installation/v1alpha1` |
| `artifacts.lima` | Host Lima executable artifact |
| `artifacts.guestImage` | Local bootable guest image artifact |
| `artifacts.docker`, `artifacts.compose` | Host-accessible copies of the exact Docker and Compose bytes installed in the guest |
| `artifacts.utilities` | Named artifact entries including `cat`, `sha256sum`, `stat`, `systemctl`, and `test` |
| Each artifact entry | `version`, `digest` (`sha256:` plus the actual lowercase SHA-256 hex digest), and clean absolute host `path` |
| `guest.dockerPath`, `guest.composePath` | Clean absolute paths inside the guest to the corresponding binaries |
| `guest.utilityPaths` | The same utility names as `artifacts.utilities`, mapped to clean absolute guest paths |
| `guest.workdir`, `guest.dockerSocket` | Clean absolute guest working-directory and Docker-socket paths |
| `guest.dockerService` | Exactly `docker.service` |

The metadata must be a regular non-symlink file, at a clean absolute path, no larger than 1 MiB. Unknown fields and trailing JSON values are rejected. Every configured artifact is verified before loading succeeds. Do not invent version/digest values or substitute your host's native utilities for the guest binaries on a different architecture.

Install the complete pinned Lima distribution, including its guest agent and support files; the `limactl` executable alone is insufficient.

The image must **already contain** the matching Docker/Compose/utilities and a functioning systemd Docker service. The Lima control user must be able to run `/usr/bin/sudo -n` inside the guest. Administrative guest commands use this noninteractive root access; the host CLI still runs as your normal user. The [guest provisioner](../internal/guesttoolchain/provisioner.go) verifies bytes and versions, enables/starts `docker.service`, and requires rootful Docker, cgroup v2, `overlay2`, bridge networking, the pinned Compose plugin discoverable through `docker compose`, and a root-owned socket without world read/write access. It does not install missing packages. The host-side artifact files are verification inputs, not an automatic file-copy installer.

## Daemon startup and paths

`abox list`, `plan`, `up`, and `rm` contact the per-user Unix-socket daemon and attempt to start it if unavailable. The CLI first looks for `aboxd` next to itself, then on `PATH`. Autostart uses `aboxd serve --runtime-dir ... --state-db ...`, inherits the environment, and appends output to the daemon log. `init`, help/version, and `doctor` do not autostart it.

Default locations:

| Item | Linux | macOS |
| --- | --- | --- |
| State directory | `${XDG_STATE_HOME:-$HOME/.local/state}/agent-box` | `$HOME/Library/Application Support/Agent Box` |
| Database | `state.db` in the state directory | `state.db` in the state directory |
| Installation metadata | `installation.json` in the state directory | `installation.json` in the state directory |
| Runtime directory | `$XDG_RUNTIME_DIR/agent-box` when set; otherwise `runtime` in the state directory | `runtime` in the state directory |
| Autostart log | `aboxd.log` in the state directory | `$HOME/Library/Logs/Agent Box/aboxd.log` |

XDG paths must be clean and absolute. Run as your normal user. Keep Unix-socket paths short enough for the operating system's socket-path limit.

For foreground operation, or an explicit metadata location, start this in a separate terminal **before** issuing daemon-backed CLI commands:

```sh
aboxd serve --installation /absolute/path/to/installation.json
```

The complete daemon syntax is:

```text
aboxd serve [--runtime-dir PATH] [--state-db PATH] [--installation PATH]
```

Lima also creates Unix sockets beneath the state directory. Long default macOS
paths can exceed its limit even when the daemon socket itself fits. For a fresh
validation installation, use a short canonical state path and keep the default
runtime directory so the CLI can connect:

```sh
aboxd serve --state-db /absolute/short/path/state.db --installation /absolute/path/to/installation.json
```

Create a private directory owned by your normal user first. This selects new
state; it does not migrate existing Boxes. Do not move a live state directory,
because Plans and Git worktrees contain canonical absolute paths.

Use Ctrl-C/SIGTERM to stop that foreground daemon. There is no `abox daemon start` or `abox daemon stop` command. The CLI has no socket override flag: a manually overridden runtime directory must still match the CLI's resolved per-user location. Prefer the defaults for interactive use. An installation override affects that daemon, not the CLI autostart defaults or doctor's default-path observation. Starting a daemon successfully does not prove that installation metadata exists; runtime requests load it separately.

## Plan, provision, inspect, remove

These are the implemented command forms, **not a promise they can run successfully on an unconfigured checkout**:

```text
abox plan [--json] [--manifest PATH] TASK
abox up [--json] [--manifest PATH] TASK
abox list [--json]
abox rm [--json] [--manifest PATH] [--yes] [DISCARD FLAGS] TASK
```

`TASK` is positional, for example `demo-task`, not `--task demo-task`. It is 1–128 ASCII bytes, starts/ends with a letter or digit, and may contain letters, digits, `-`, `_`, and `.`. Workspace identity derives from the canonical manifest path; Box identity derives from workspace identity plus the task key.

`plan`, `up`, and `rm` discover `.abox/box.yaml` by walking upward from the current directory. Alternatively, supply `--manifest /absolute/path/to/.abox/box.yaml` (relative explicit paths are resolved from the current directory). `list` is user-wide and takes neither a task nor a manifest argument.

After configuring the installation and workspace:

```sh
abox plan demo-task
abox plan --json demo-task
```

Plan compiles proposed operations without provisioning a VM. It is **not** an offline manifest-only linter: the production endpoint loads and verifies the runtime installation and inspects Git. Connecting may start the daemon and initialize its database/profile even though the plan itself does not apply Box changes.

Only on an appropriately configured, eligible runtime host, and when ready to create resources:

```sh
abox up demo-task
abox list
abox list --json
```

`up` executes the reconcile pipeline: preflight, plan/preparation, managed Git stores and worktrees, Lima runtime, guest checks, Compose apply, and filesystem readiness. `--json` streams versioned NDJSON events for `up` and `rm`; `plan --json` and `list --json` return their versioned JSON documents. `list` reports recorded inventory/drift, not a substitute for live hardware acceptance.

Removal starts with a read-only loss preview:

```sh
abox rm demo-task
```

Without `--yes`, no deletion is authorized. After reviewing the preview, a no-loss removal can be authorized with:

```sh
abox rm --yes demo-task
```

If the preview identifies losses, `--yes` alone is insufficient. Add only the flags corresponding to losses you explicitly intend to accept:

- `--discard-worktree-changes`: dirty, untracked, and ignored files.
- `--discard-unpublished-commits`: unpublished commits.
- `--delete-volumes`: Box-owned volumes.

Each invocation previews again and binds confirmation to that exact preview digest; do not treat these as generic force flags. Keep the manifest available so the CLI can resolve the Box identity.

For diagnostics scoped to a task, use `abox doctor TASK` or `abox doctor fs TASK`. These remain observation-only and may report unknown evidence rather than contacting a running Box.

## Common blockers

- **`abox: command not found`:** use `./bin/abox`, or run `make install` and add the installation directory to `PATH`.
- **Cannot find/start `aboxd`:** install both binaries together; inspect the autostart log and daemon path permissions.
- **`tool.lima` unavailable:** install the appropriate Lima tool, but also satisfy the verified installation contract; this is not the last provisioning prerequisite.
- **Host Docker unavailable:** distinguish CLI/plugin diagnostics from guest service requirements. A host daemon alone cannot make a Box ready.
- **Installation metadata absent or verification failed:** provide real artifacts and matching metadata. `plan` also needs them. There is no automatic supported guest download fallback.
- **Linux provisioning rejected:** this is the current production platform boundary, not a missing package that can be fixed by installing QEMU.

For development progress and validation evidence, see [features](features.md), [roadmap](../ROADMAP.md), and [release policy](../RELEASES.md).
