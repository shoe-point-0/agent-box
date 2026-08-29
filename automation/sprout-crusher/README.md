# Alph Sprout Crusher

This is a repository-local adapter from Snowball's durable iteration runner to the Hermes `alph` profile. Snowball keeps the master prompt, iteration ledger, locks, git integrity checks, stall/error budgets, and completion protocol. Hermes performs each fresh coding cycle.

## Why this seam

Snowball is derived from the original Crusher script and already supports custom agents through `--agent-command`: it sends the compiled iteration prompt on stdin and consumes the final promise marker from stdout. `scripts/alph-snowball-agent` translates that contract into:

```text
alph --yolo --in <repo> --skills ... chat --quiet --query-file -
```

Snowball's `--unsafe` flag is intentionally not used because it configures Codex only. Hermes YOLO mode is applied in the adapter. Snowball still enforces a fixed branch, prompt integrity, iteration locking, commit policy, stall limits, and durable external state.

Hermes' built-in `/goal` is useful for one standing objective inside one session. Snowball is used here because this backlog needs fresh-session rehydration, an external immutable master prompt, Sprout-aware task selection, git commit guards, and a durable per-iteration audit trail.

## Prerequisites

- The `alph` Hermes profile exists and is authenticated.
- The Snowballing skill is installed in the Alph profile.
- `sprout`, `git`, Go, Docker, and repository-specific prerequisites are available.
- Start from a clean branch. Dirty worktrees are rejected by default.

Sanity checks:

```bash
alph doctor
sprout check --all
python3 scripts/sprout_frontier.py
```

## Kickoff

Always inspect the compiled prompt first:

```bash
./scripts/crush-sprout dry-run
```

Launch or resume the durable loop:

```bash
./scripts/crush-sprout run
```

Inspect progress:

```bash
./scripts/crush-sprout status
```

Archive the previous session and start again:

```bash
./scripts/crush-sprout fresh
```

The launcher performs a mandatory Snowball dry run before every `run` or `fresh` execution.

## Controls

Environment overrides:

```bash
SNOWBALL_MAX_ITERATIONS=80 ./scripts/crush-sprout run
SNOWBALL_MAX_MINUTES=240 ./scripts/crush-sprout run
HERMES_CRUSHER_MAX_TURNS=300 ./scripts/crush-sprout run
HERMES_CRUSHER_REASONING=xhigh ./scripts/crush-sprout run
HERMES_CRUSHER_TOOLSETS=hermes-cli ./scripts/crush-sprout run
```

`SNOWBALL_MAX_ITERATIONS` is the total ceiling for the durable session, not a per-invocation increment. Raise it when resuming a session that reached its ceiling.

The adapter explicitly selects the supported `hermes-cli` platform toolset for each iteration. This bypasses stale per-platform entries such as the retired `messaging` toolset without rewriting the Alph profile's global configuration.

For an intentionally dirty starting tree, first inspect every change, then opt in explicitly:

```bash
SNOWBALL_ALLOW_DIRTY=1 ./scripts/crush-sprout dry-run
SNOWBALL_ALLOW_DIRTY=1 ./scripts/crush-sprout run
```

Do not make dirty mode the normal path. Snowball preserves the changes but does not create a rollback point.

## Commit and push policy

The runner uses `--allow-commits`. The master prompt requires meaningful conventional commits, normally one per completed Sprout packet. Branch switching and unauthorized commits are independently detected by Snowball.

Pushes are not authorized. The Hermes adapter prepends a Git shim that delegates normal Git operations to the real executable but exits `97` for `git push` and `git send-pack`. The immutable prompt independently forbids remote mutation. An agent could still bypass a PATH shim with an absolute Git path or another network client, so this is defense in depth rather than a sandbox. Review the resulting commits and push manually after the loop stops.

## Safety boundary

`alph --yolo` bypasses Hermes command approvals. It is appropriate here only because the master prompt restricts destruction to disposable, task-created resources after ownership proof. YOLO is not a sandbox and the profile still runs as the Machine user.

The loop must stop BLOCKED for credentials, product decisions, Apple Silicon hardware gates, shared infrastructure, ambiguous ownership, or destructive work outside the prompt's authorization.

## State

Snowball state stays outside the repository under:

```text
${SNOWBALL_STATE_HOME:-${XDG_STATE_HOME:-$HOME/.local/state}/snowballing}
```

The same repository and prompt paths resume the same session automatically. Iteration prompts, outputs, git snapshots, prompt diffs, state, and progress are retained there for auditability.
