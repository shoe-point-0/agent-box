# Repository agent instructions

Follow the repository's established conventions in [CONTRIBUTING.md](CONTRIBUTING.md) and keep changes focused.

When implementing a feature or bug fix:

- Lay of the Land: Inspect relevant code, tests, architecture decisions, and repository instructions before editing.
- Tracer Bullet: First prove feasibility via the shortest possible end-to-end path.
- Hot Path Exploration: Build out the primary execution path and core logic.
- Safe Passage: Harden the implementation, run `make check`, and resolve any regressions.
- Closing Review: Update reader-facing documentation under `docs/features/` (indexed in `docs/features.md`) and record real verification evidence without widening platform support claims beyond observed execution.
