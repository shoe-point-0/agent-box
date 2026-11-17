<!-- sprout-task
{"schema_version":1,"id":"m0-git-path-probe","status":"implemented","created_at":"2026-08-29T06:48:35.822326556Z","implemented_at":"2026-08-29T07:49:12.167881984Z"}
-->

# Private Git-store path invariance

Status: implemented. Implementation record for `m0-git-path-probe`.

- Delivered the `git-path` Linux scenario with typed `abox.spike.git-path/v1` evidence and scenario-owned marked-tree cleanup.
- Added path invariance, Git source non-mutation, no-hardlink/no-alternate, Machine/container status/add/commit/log, lock, UID/GID, cleanup preflight, and unsupported-capability checks.
- Extended the harness with scenario-specific cleanup that journals removed resources before generic retained-root teardown; generic cleanup still refuses active unsupported resource kinds.
- No Darwin, VZ, VirtioFS, or macOS Git behavior is claimed. The unchanged fixtures are available for the final Darwin/arm64 hardware gate.

## Scope and dependencies

- Milestone 0. Dependencies: m0-spike-harness. Source: THE_BEGINNING.md Sections 10 and 19.2.

## Behavior

- Seed a self-contained bare repository without hardlinks or alternates and create a locked linked worktree.
- Run stock Git on the Machine and from a container using identical absolute worktree and common-directory paths.
- Exercise spaces, Unicode, symlinked roots, long components, and supported relative-worktree behavior.

## Constraints

- Support claims require real execution evidence on the claimed platform.

## Verified acceptance criteria

- Status, add, commit, log, and lock inspection agree across boundaries.
- Source repository metadata and object inodes are not mutated by Box Git writes.
- Unsupported path or Git-version combinations fail with retained evidence.

## Implementation details

- Register a Linux `git-path` scenario that owns one marked fixture tree and cleans it only after matching the persisted run ID.
- For plain, spaces, Unicode, long-component, and symlinked-source fixtures, create a source repository, snapshot its metadata, clone a self-contained bare store with `--no-hardlinks`, create and lock a linked worktree, and exercise Machine Git plus stock Git 2.47.3 from the locally cached `golang:1.26` image.
- Mount the complete retained run root into the container at the identical absolute path, disable networking, and run container Git without shell interpolation.
- Verify no alternates, no shared source/Box object inodes, no source metadata drift, matching status/HEAD/log behavior, and exact worktree lock registration.
- Probe `worktree.useRelativePaths` as a versioned capability. The current Machine Git 2.43.0 does not advertise it, so the relative-mode subcase must record an explicit unsupported result rather than mutating pointers manually or pretending support.
- Emit a typed JSON report in retained diagnostics and make fixture-tree cleanup scenario-specific; this Linux result makes no Darwin/macOS filesystem claim.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run the Git path probe against every declared path fixture

## Validation evidence

- Final real probe passed on Linux 7.0.0-28-generic amd64 with Machine Git 2.43.0, Docker 29.6.1, and container Git 2.47.3 from the locally cached image digest `sha256:73204d15b474cccdcf60645cfc88bf3d8d68cbc1687334c70f66bd03832e8259` with networking disabled.
- Plain, spaces, Unicode (`unicodé-路径`), 200-character component, and symlinked-source fixtures each created a self-contained bare store, locked linked worktree, Machine commit, and container commit.
- Every fixture reported identical Machine/container HEAD and `git log -1 --format=%H`, clean porcelain-v2 status, observed lock registration, no alternates, no shared source/Box object inodes, and a byte/mode-identical source Git metadata snapshot before and after Box writes.
- Docker diagnostics prove the complete retained run root was mounted at the identical absolute path and container Git ran as Machine UID/GID `1000:1000`; a complete ownership scan found only `(1000,1000)`.
- The first cleanup attempt exposed root-owned container Git objects. The probe was corrected to map Machine UID/GID, and cleanup gained a full same-filesystem/directory-permission preflight. The final run cleaned without repair and left no probe containers or root.
- Machine Git 2.43.0 and container Git 2.47.3 did not advertise `worktree.useRelativePaths`; the report records the combination as unsupported and explicitly forbids manual pointer rewriting.
- `make check` and `go test -race -count=10 ./...` passed. An artifact-free `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed but remains explicitly unvalidated hardware evidence.
