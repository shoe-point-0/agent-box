<!-- sprout-task
{"schema_version":1,"id":"m1-init-command","status":"implemented","created_at":"2026-08-29T06:55:50.464348415Z","implemented_at":"2026-08-29T23:41:16.216104363Z"}
-->

# Starter workspace manifests with abox init

Status: implemented.

The `abox init` command generates a deterministic, strict-loader-valid starter
`.abox/box.yaml` manifest entirely on the local Machine without contacting
`aboxd` or creating Box state. By default it uses exclusive no-clobber file
creation so existing manifests are never overwritten accidentally; `--force`
provides the sole explicit overwrite path using crash-consistent atomic
replacement.

The starter template intentionally uses generic placeholder names and sets its
single local repository source to `..`, so the file validates cleanly from any
working directory without inspecting Git metadata or requiring an authored
`compose.yaml` up front.

## Scope and dependencies

- Milestone 1 CLI command. Depends on [manifest loader](manifest-loader.md) and [CLI transport](cli-transport.md). Source: `THE_BEGINNING.md` Section 14.1.

## Behavior

- Creates `.abox` (mode `0700` when absent) and writes a deterministic `v1alpha1` starter `.abox/box.yaml` (mode `0600`) whose local repository source resolves to `..`.
- Uses exclusive creation by default, returning `ABOX_IDENTITY_CONFLICT` (exit code `4`) if `.abox/box.yaml` already exists, and allows explicit atomic replacement only when `--force` is passed.
- Rejects symlinked `.abox` directories and symlinked or non-regular target files, cleaning up a newly created empty `.abox` directory if writing fails.
- Runs purely locally without contacting `aboxd`, accessing user state directories, invoking Git or runtime tools, or allocating Box resources.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- No Git store, SQLite row, VM, port, or Box artifact is created by `abox init`.

## Verified acceptance criteria

- Generated YAML passes `manifest.Loader` strict validation without edits.
- Existing manifest files remain untouched unless `--force` is specified.
- No daemon process, Git store, state row, VM, port, or Box directory is created.

## Implementation details

- `init` executes directly inside `cmd/abox` using only the current working directory and never requests daemon paths, constructs a daemon client, starts `aboxd`, or runs Git or runtime subprocesses.
- Default creation opens `.abox/box.yaml` with exclusive flags (`O_CREATE|O_EXCL`) so concurrent creation cannot clobber an existing file; `--force` delegates to `internal/atomicfile`, which enforces mode `0600` and rejects symlinks and non-regular files.
- If `abox init` creates `.abox` itself and manifest creation subsequently fails, it removes the empty directory before returning the error.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`

## Validation evidence

- CLI unit tests verified exact deterministic output bytes, strict acceptance by `manifest.Loader`, mode `0600`, refusal without `--force`, replacement with `--force`, rejection of symlinked `.abox` directories, zero calls to daemon hooks (guarded by panicking stubs), and a clean workspace containing only `.abox/box.yaml`.
- Live Linux binary smoke in disposable directories confirmed exit `0` on initial `abox init`, exit `4` (`ABOX_IDENTITY_CONFLICT`) on repeat invocation, and exit `0` with `--force`, while isolated XDG state and runtime directories remained empty.
- `make check` (`0` lint issues), `go test -race -count=10 ./...`, `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), `sprout check m1-init-command`, and `git diff --check` passed.
