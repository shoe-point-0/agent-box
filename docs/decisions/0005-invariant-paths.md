# ADR 0005: Preserve canonical absolute paths across Machine, VM, and container

- Status: Accepted; filesystem timing and event assumptions amended by [ADR 0011](0011-bounded-filesystem-visibility.md)
- Date: 2026-08-29
- Scope: v0.1 worktree and Git metadata path topology

## Context

A linked worktree's `.git` file and common-directory metadata contain paths. If the Machine, VM, and workspace service see different paths, stock Git may resolve administration data incorrectly. Rewriting Git internals creates a version-sensitive format boundary and can corrupt repositories. Modern relative-worktree support also changes repository extensions and is not available in the tested Git matrix.

## Decision

Resolve every Box worktree and private Git store to a canonical Machine absolute path and expose each at that identical absolute path in the Lima guest and workspace service. `abox shell` starts in the canonical worktree path.

Only declared worktrees, their required common Git directories, and generated guest inputs enter the mount allowlist. Path validation must use canonical filesystem resolution, reject escape through `..` or symlinks, and test spaces, Unicode, long components, and symlinked source discovery.

Do not manually rewrite `.git` pointers. Do not enable `worktree.useRelativePaths` unless a future compatibility decision proves every participating Git client supports the resulting repository extension.

This is accepted design intent plus Linux bind-mount evidence. VZ/VirtioFS transport, coherency, ownership, modes, watcher behavior, and exact mount denial remain unaccepted until real Darwin hardware passes the unchanged fixture.

## Alternatives considered

- Rewrite `.git` files for guest-relative paths: rejected because Agent Box would own an undocumented, version-sensitive Git metadata transformation and recovery path.
- Enable relative worktree paths immediately: rejected because neither tested Git version advertised the feature and older clients may reject `extensions.relativeWorktrees`.
- Clone again inside the VM: rejected because it introduces a second source authority and merge/synchronization problem.
- Mount a broad common ancestor such as the Machine home directory: rejected because it violates least privilege and exposes unrelated files and credentials.

## Consequences

- Linux guests may contain unusual paths such as `/Users/...`; correctness takes priority over cosmetic guest layout.
- Lima configuration must support explicit non-overlapping absolute mounts and preserve access modes.
- Path canonicalization and allowlist validation happen before VM mutation.
- A failed VirtioFS gate blocks the affected Darwin architecture rather than triggering pointer rewriting.
- The filesystem layer provides visibility, not backup or conflict resolution.

## Evidence

- [Git path invariance spike](../features/spike-git-path.md) passed plain, spaces, Unicode, 200-character, and symlink fixtures using identical paths through a Linux bind mount.
- [The substrate report](../architecture/substrate-spikes.md) records that this did not exercise VirtioFS and that Git 2.43.0/2.47.3 did not advertise relative-worktree support.
- [ADR 0001](0001-milestone-0-substrate-gate.md) explicitly forbids treating bind-mount evidence as Darwin support.
- Lima documents explicit host mounts and VZ VirtioFS behavior in [Filesystem mounts](https://lima-vm.io/docs/config/mount). This is an upstream capability contract, not coherency evidence.

## Revisit triggers

Supersede this ADR if:

- the real Darwin fixture fails any bidirectional read/write, rename/delete, mode, symlink, watcher, lock, or Git operation at invariant paths;
- Lima cannot expose the exact allowlist without making an undeclared Machine canary visible;
- the minimum supported Git version and every guest/Machine client implement relative worktrees and the unchanged Git isolation suite passes in relative mode;
- a supported host filesystem rejects or aliases a required canonical path; or
- measured path-length or mount-count limits prevent the accepted multi-repository fixture from starting.
