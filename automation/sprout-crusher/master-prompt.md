# Snowball: agent-box-sprout-burndown

## Objective

Burn down every dependency-ready Sprout task that can be implemented and verified on the Linux engineering lane. Preserve the Agent Box architecture, safety invariants, and Linux-first/Darwin-final support policy. Finish each task through Sprout and leave reviewable conventional commits at coherent verified boundaries without pushing.

## Done when

- [ ] No dependency-ready or non-deferred blocked Linux task remains; every remaining active task is explicitly Darwin-hardware/Milestone-6 deferred or transitively blocked only by one of those deferred gates.
  - Verify: `python3 scripts/sprout_frontier.py --assert-terminal`
- [ ] Every completed Sprout packet has passed its completion gate, moved from `.sprout/tasks/<id>.md` to `docs/features/<id>.md`, and contains concrete execution evidence rather than intent.
  - Verify: `sprout check --all && python3 scripts/sprout_frontier.py --json`
- [ ] The repository's full quality gates pass after the final Linux slice.
  - Verify: `make check && go test -race -count=10 ./... && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`
- [ ] The branch is clean and all loop-created changes are preserved in meaningful conventional commits with no push performed.
  - Verify: `test -z "$(git status --porcelain)" && git log --oneline --decorate -20`

## Scope and constraints

- Work only in this repository and Snowball's external state directory.
- Follow `AGENTS.md`, `THE_BEGINNING.md`, the selected Sprout packet, and repository-local skills before editing.
- Keep Linux as the implementation and continuous verification lane. Do not execute or close Darwin VZ/VirtioFS hardware tasks on Linux, and never infer macOS support from cross-compilation.
- When the worktree is clean, select exactly one next task using `python3 scripts/sprout_frontier.py --next`. If the worktree is dirty, finish or safely resolve the current task before selecting another.
- Take one Sprout packet end to end: inspect, refine its technical approach, implement the smallest honest vertical slice, test real boundaries, record evidence, run `sprout check --complete <id>`, and close it only when every criterion is true.
- Never mark a test, acceptance criterion, or task complete from confidence, simulation, or a partial subset.
- Prefer one conventional commit per completed Sprout task. A large task may use multiple commits only at coherent, independently verified boundaries. Never bundle unrelated tasks in one commit.
- Commits are authorized on the locked branch. Amend, reset, rebase, branch switching, history rewriting, tagging, releases, and pushes are forbidden.
- YOLO mode removes interactive command approvals; it does not expand scope. Destructive operations are authorized only for disposable resources created by the selected task after exact ownership proof and complete preflight. Never mutate shared or production infrastructure.
- Do not install unpinned dependencies, expose credentials, weaken redaction, or persist secret values.
- Preserve existing user work. Do not discard, overwrite, or auto-repair ambiguous state.
- If a task requires Apple Silicon hardware, a credential, a product decision, shared-infrastructure mutation, or destructive action outside the authorization above, stop with BLOCKED.

## Context and starting points

- Product and architecture baseline: `THE_BEGINNING.md`
- Active work: `.sprout/tasks/`
- Implemented evidence: `docs/features/`
- Dependency frontier: `python3 scripts/sprout_frontier.py`
- Task lifecycle: `sprout show <id>`, `sprout check <id>`, `sprout check --complete <id>`, `sprout close <id>`
- Baseline verification: `make check`, `go test -race -count=10 ./...`, `sprout check --all`
- Current platform policy: Linux implementation first; real Darwin/arm64 Lima VZ and VirtioFS validation is a final pre-RC gate.

<!-- snowball:strategy:start -->
## Current strategy

Start from the mechanically computed frontier. Prefer the earliest Milestone 0 Linux task, then close the Linux substrate report before entering Milestone 1. For each selected packet, prove the riskiest external boundary first and keep ownership/cleanup evidence symmetric with creation. Commit only after the task or coherent slice is green and documented.
<!-- snowball:strategy:end -->

## Stop conditions

- Stop complete only when `python3 scripts/sprout_frontier.py --assert-terminal` and every final quality gate pass from a clean branch.
- Stop blocked on missing credentials, user decisions, unavailable Linux prerequisites, Apple Silicon-only validation, shared infrastructure, or cleanup whose ownership cannot be proven.
- Stop immediately on branch drift, unauthorized history changes, unexpected pre-existing edits, leaked credentials, or inability to preserve user work.
- Stop rather than repeatedly retrying the same failed approach without new evidence or a changed hypothesis.

## Iteration expectations

Each iteration must rehydrate repository and Snowball state, continue an existing dirty task or select one dependency-ready task, complete one coherent inspect-act-verify slice, preserve evidence, and end with the exact Snowball promise marker. A completed task should normally end the iteration with a clean conventional commit. Never push.
