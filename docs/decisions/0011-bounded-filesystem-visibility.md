# ADR 0011: Bound filesystem visibility and forward writable-mount events

- Status: Accepted; validated on the single-repository Apple M1 gate. The [Milestone 5 release matrix](../../ROADMAP.md#milestone-5-releasable-v01-candidate) remains open.
- Date: 2026-11-21
- Scope: Darwin/arm64 filesystem readiness; amends the coherency assumption in ADR 0005

## Context

A real Apple M1 run with macOS 27.0.1 (26A434), Lima 2.0.0, VZ,
and VirtioFS reached healthy workspace containers but failed filesystem readiness.
After a host rename, the guest could still read the old name at observations
starting 0.13 and 0.42 seconds after the rename; it was absent at the observation
starting 1.54 seconds. This was one diagnostic sample, not a latency distribution.
A separate diagnostic allowed the name cache to settle, established the native
in-container inotify handshake, then failed to receive the host create event
within the required second. Native events were tested with `mountInotify: false`.

Immediate name visibility and unassisted native event delivery are therefore not
valid assumptions for this measured stack. Broadening mounts or rewriting Git
metadata would not solve them. Increasing only the test timeout would leave the
product behavior undefined.

## Decision

The readiness probe allows at most two seconds for each observation of exact
bytes, mode, rename, or deletion in either direction. It retries observations
only: writes, renames, and deletions execute once. Command errors fail immediately;
cancellation and expiry prevent READY. A rename succeeds only when the old name
is absent and the new name has the expected bytes and mode in the same attempt.
This is bounded eventual visibility, not cross-machine atomic observation.

Enable `mountInotify: true` in the pinned Lima configuration. The workspace
still uses an in-container native inotify watcher and an activation handshake,
but host events are synthesized by Lima's bridge via `IN_ATTRIB` (metadata
change, reported as `e` by BusyBox `inotifyd`) without altering file contents
or permissions. Event delivery must still finish within one second after the
host write; polling file contents does not substitute for that event. Read-only
Git metadata is not made writable to enable events.

[Lima documents](https://lima-vm.io/docs/config/mount/#mount-inotify) this bridge
as experimental, limited to writable mounts, and unable to report host deletion
events. Projects whose watcher ignores `IN_ATTRIB` or requires host deletion
events must configure their own polling watcher. A readiness pass establishes
the tested create/modify path only.

## Compatibility and recovery

The generated configuration is immutable. Existing instances with the old
`mountInotify` or automatic port-forward policy are rejected rather than edited
in place. Use a fresh fixture for validation. For an existing Box, retain work,
review `abox rm`'s loss preview, and recreate only after an explicitly authorized
removal; never delete its VM or worktrees to bypass ownership checks.

The same exact mount allowlist, read-only metadata policy, canary denial,
path-identity checks, and final READY transaction remain required. Generated
Compose inputs travel as bounded private guest temporary files rather than
adding the Box root or Machine home to the mount set.

## Validation boundary

Regression tests must prove convergence without repeated mutations, stale-stat
read-race handling without accepting unreadable or corrupted files, permanent
mismatch refusal, cancellation cleanup, and mandatory bridge configuration.
Real cold, warm, detached-client, and ownership-aware removal runs passed this
revised contract on Apple M1 (macOS 27.0.1 build 26A434, Lima 2.0.0 VZ, Docker
29.1.3, Compose 2.40.3): bridged `IN_ATTRIB` delivery measured 11.6–14.3 ms
across five trials with mode `0600` and exact bytes intact, canceled watchers
exited cleanly within 510.1 ms with zero leaked processes/zombies/FIFOs, cold
`abox up demo` reached READY in 40.79 s, warm `abox up demo` preserved every
worktree/Git/generated-file/VM/container/network identity in 15.36 s, and
`SIGKILL` of the attached CLI during live cold reconciliation still converged to
a single READY generation. See
[apple-silicon-m1-gate.md](../architecture/apple-silicon-m1-gate.md) and
[apple-silicon-m1-evidence.json](../architecture/apple-silicon-m1-evidence.json).
The earlier strict filesystem gate did not pass, and this revision does not
establish the full minimum/current-macOS release matrix or perfect event
semantics.

Revisit if the two-second bound or one-second event deadline fails, if event
forwarding changes file contents or modes, or if a pinned Lima upgrade changes
its semantics. Do not automatically increase these deadlines.
