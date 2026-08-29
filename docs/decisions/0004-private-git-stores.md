# ADR 0004: Contain Git state in a private store per repository and Box

- Status: Accepted
- Date: 2026-08-29
- Scope: v0.1 Git metadata, worktree creation, and publication boundary

## Context

Coding agents need ordinary Git operations, but a worktree linked directly to the user's repository shares refs, configuration, hooks, locks, and administration data with that checkout and sibling tasks. Agent-written Git configuration must also never become a Machine command-execution or credential bridge during publication.

## Decision

For every declared repository, create one self-contained bare Git store owned by that Box and one locked linked worktree registered only with that store.

Local sources must be clean before creation. Seed the private store without object alternates or writable hardlinks, enable reflogs, resolve the base to a full commit ID, create the generated task branch, lock the worktree with the Box identity, and verify registration with script-oriented, NUL-safe Git output. Record canonical path and filesystem identity for both the store and worktree.

The default workspace policy grants read-write access to Box-private Git metadata. It never grants access to the user's source repository metadata. Active submodules are rejected in v0.1.

Publication is explicit. Box-side Git produces an exact bundle; the Machine validates it, imports it into a fresh Agent Box-controlled temporary bare repository with hooks disabled, and pushes only to the Plan-recorded destination using Machine credentials. The Machine does not trust Box-writable Git configuration or hooks.

Removal refuses dirty files, ignored/untracked residue, unpublished commits, registration mismatch, or ambiguous ownership. Destructive override requires every applicable explicit discard flag.

## Alternatives considered

- Create linked worktrees from the user's repository: rejected because agents and Boxes would share mutable Git administration state.
- Full non-bare clone per worktree: rejected because it adds an unnecessary checkout while still requiring a publication boundary; a bare store plus linked worktree models the ownership directly.
- Git alternates or hardlinked objects: rejected because corruption, pruning, or writes can cross the Box/source boundary.
- Automatically commit or push agent work: rejected because it changes source history or external state without an explicit user action.
- Push directly from Box-writable Git configuration: rejected because hooks, `core.sshCommand`, filters, remotes, and helper configuration become Machine code-execution inputs.

## Consequences

- Agents can use normal Git while their direct metadata blast radius remains one Box.
- Stores duplicate some object data and require explicit fetch/publish synchronization.
- Unpushed commits and uncommitted files remain inside the accepted one-Box loss envelope.
- Safe deletion and publication require graph reachability, dirty-state, registration, identity, and destination checks rather than directory-name checks.
- Source checkout dirtiness fails closed because v0.1 does not copy an implicit working tree.

## Evidence

- The retained [`git-path` probe](../features/m0-git-path-probe.md) passed private bare-store creation, locked worktrees, source-metadata isolation, inode isolation, Machine/container commits, and ownership-aware cleanup with Git 2.43.0 and 2.47.3 on Linux.
- [The aggregate substrate report](../architecture/substrate-spikes.md) records the exact matrix and limits the result to Linux bind mounts.
- Git documents script-oriented worktree inspection and locking in [`git worktree`](https://git-scm.com/docs/git-worktree), and object-copy isolation in [`git clone`](https://git-scm.com/docs/git-clone).
- [ADR 0001](0001-milestone-0-substrate-gate.md) makes a red Git or cleanup probe blocking.

## Revisit triggers

Supersede this ADR if:

- failure injection finds a private-store write that changes source or sibling-Box objects, refs, config, hooks, locks, or inodes;
- removal or recovery cannot prove worktree registration, lock ownership, dirty state, and unpublished-commit reachability without destructive ambiguity;
- publication testing demonstrates Machine command execution or destination selection can be influenced by Box-writable Git state;
- object duplication exceeds the documented disk budget for the multi-repository acceptance fixture and an alternative passes the same isolation and corruption suite; or
- submodule support, working-copy import, shared object caching, or automatic external backup enters v0.1 scope.
