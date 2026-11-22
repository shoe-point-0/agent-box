<!-- sprout-task
{"schema_version":1,"id":"m1-process-runner","status":"implemented","created_at":"2026-08-29T06:48:35.952766065Z","implemented_at":"2026-08-29T19:48:47.509667038Z"}
-->

# Safe subprocess execution

Status: implemented.

The `internal/process` package provides Agent Box's hardened Unix subprocess
runner: literal argv-only execution, explicit complete child environments,
process-group lifecycle control with bounded shutdown escalation, independent
bounded stdout/stderr capture and streaming sinks, and stateful multi-form
secret redaction across arbitrary byte chunk boundaries.

## Scope and dependencies

- Milestone 1 security and runtime foundation. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Section 18.1 and Issue 4.

## Behavior

- Executes argv arrays directly without invoking a shell or inheriting ambient daemon environment variables; resolves bare executable names strictly through the caller-supplied `PATH` and requires an absolute working directory.
- Starts every child in a fresh process group, signals the entire group with `SIGTERM` followed by `SIGKILL` after a bounded grace period on cancellation, sink failure, or output overflow, reaps the child, drains both pipes, and cleans up descendants even if the group leader exits first.
- Compiles a size-bounded secret registry before process start (exact, URL path/query escaped, and standard/raw/URL base64 forms) and redacts stdout and stderr statefully across chunk boundaries before bytes reach captures or streaming sinks.
- Returns elapsed timing, partial redacted output, and typed errors (`ExitError`, `CancellationError`, `SinkError`, `OutputLimitError`, `PipeError`, `StartError`, `ShutdownError`) that never embed raw child output or sink text.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- Streaming sinks follow Go's `io.Writer` contract synchronously and must eventually return; if a sink returns an error or short write, the runner disables it, terminates the process group, and drains both pipes, but cannot interrupt an arbitrary writer blocked forever inside `Write`.

## Verified acceptance criteria

- Shell metacharacters in arguments remain literal strings.
- Cancellation leaves no owned descendant process running.
- Registered canary secrets never appear in captured output, streaming sinks, errors, events, or logs.
- One-byte chunking, overlapping secrets, pipe backpressure, sink failure, partial output, and cancellation races leak zero secrets, pipe goroutines, or descendant processes.

## Implementation details

- `process.Runner` replaces ad-hoc `exec.CommandContext` calls with explicit `Setpgid: true` process-group management and configurable `CaptureLimit`, `TermGrace`, and `ShutdownTimeout` bounds (also carrying optional bounded literal `Stdin` added for stdin-based guest transport in [`abox up`](abox-up.md)).
- Secret compilation rejects empty, oversized, or excessive-count/total registries before spawning. An immutable trie drives independent per-stream longest-match redactors that retain candidate prefixes across arbitrary read sizes.
- Stdout and stderr collectors drain concurrently into bounded buffers and optional sinks so pipe writers never deadlock during error teardown.

## Verification

- [x] `go test ./...`
- [x] `go test -race ./...`
- [x] `go vet ./...`
- [x] Run focused process cancellation and redaction stress tests

## Validation evidence

- `go test -race -count=20 ./internal/process` passed 20 full repetitions covering literal shell metacharacters, explicit environment/working-directory isolation, typed nonzero exits, exact and transformed secret redaction, one-byte/overlapping redaction, raw and captured output limits under pipe backpressure, sink failure, partial output, cancellation, 20 cancellation/descendant races per run (400 total), and descendant cleanup after group-leader exit.
- Real helper subprocesses emitted canary values in exact, URL-escaped, and standard/raw/URL base64 forms one byte at a time across stdout and stderr; byte scans found zero surviving secret forms in captures, sinks, errors, or diagnostics.
- Cancellation tests observed real descendant PIDs, canceled the context, verified typed `context.Canceled` errors, and confirmed `kill(pid, 0)` returned `ESRCH`.
- `make check` (`0` lint issues across eleven packages), `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only), and `git diff --check` passed.
