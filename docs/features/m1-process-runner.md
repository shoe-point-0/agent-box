<!-- sprout-task
{"schema_version":1,"id":"m1-process-runner","status":"implemented","created_at":"2026-08-29T06:48:35.952766065Z","implemented_at":"2026-08-29T19:48:47.509667038Z"}
-->

# Implement the safe subprocess runner

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement the safe subprocess runner

## Context

- Milestone 1. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Section 18.1 and Issue 4.

## Requirements

- Execute argv arrays with explicit environment allowlists, working directories, context cancellation, separate bounded-capture and streaming sinks, timing, and typed exit, cancellation, sink, and output-limit errors.
- Terminate owned process groups on cancellation and bound shutdown waits.
- Register a fixed documented, size-bounded set of exact and transformed secret forms before process start and redact statefully across arbitrary stdout/stderr chunk boundaries.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] Shell metacharacters remain literal arguments.
- [x] Cancellation leaves no owned descendant process running.
- [x] Canary secrets never appear in captured output, errors, events, or logs.
- [x] One-byte output chunking, overlapping secrets, blocked readers, sink failure, partial output, and cancellation races leave no secret, pipe goroutine, or descendant process behind.

## Technical approach

- Add an `internal/process` Unix runner rather than extending the disposable
  Milestone 0 spike runner. The command contract accepts only an argv vector,
  an absolute working directory, and a complete environment map; it never
  invokes a shell or inherits ambient environment variables. Bare executable
  names are resolved only through the supplied `PATH`.
- Start every command in a new process group. Own cancellation instead of using
  `exec.CommandContext`: signal the whole group, allow a bounded TERM grace
  period, escalate to KILL, reap the direct child, drain both pipes, and report
  a typed shutdown failure if the hard bound is exceeded. Also remove
  descendants that outlive a normally exiting group leader.
- Drain stdout and stderr independently into bounded captures and optional
  streaming sinks. Redact before either destination, keep draining after a
  sink or limit failure, terminate the command on the first such failure, and
  return typed exit, cancellation, sink, output-limit, and shutdown errors with
  partial redacted results and elapsed timing.
- Compile secrets before process start into a fixed set of exact, URL-escaped,
  and standard/URL base64 forms. Bound secret count, individual size, and total
  registered form bytes. Use a streaming longest-match redactor that retains
  possible pattern prefixes so matches spanning arbitrary read boundaries and
  overlapping forms cannot leak.
- Exercise the runner with this test binary as a real subprocess: literal shell
  syntax, isolated environment and working directory, exact exit status,
  one-byte redaction, overlapping/transformed secrets, output limits, partial
  output, sink failure, cancellation races, pipe backpressure, and descendant
  process-group cleanup.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] go test ./...
- [x] go test -race ./...
- [x] go vet ./...
- [x] Run focused process cancellation and redaction stress tests

## Validation evidence

- `go test -race -count=20 ./internal/process` passed 20 complete
  repetitions. Each repetition exercised literal metacharacters, explicit
  environment and working-directory behavior, typed nonzero exits, exact and
  transformed redaction, one-byte/overlapping redaction, raw/captured output
  limits under pipe backpressure, sink failure, partial output, cancellation,
  20 cancellation/descendant races, and cleanup after a group leader exits.
- Real helper subprocesses registered canary values and emitted their exact,
  URL path/query escaped, and standard/raw/URL base64 forms across stdout and
  stderr one byte at a time. Captures, streaming sinks, typed errors, and test
  diagnostics were byte-scanned; no registered form survived.
- Cancellation tests started a real descendant, observed its PID through the
  redacted streaming sink, canceled the context, required a typed error
  wrapping `context.Canceled`, and verified `kill(pid, 0)` reached `ESRCH`.
  The race test repeated that path 400 times in the focused stress command.
- `make check` passed all eleven Go packages: formatting, pinned lint (`0
  issues`), module verification, unit tests, race tests, vet, builds, spike
  smoke output, and automation tests.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` passed as compilation
  evidence only; no Darwin runtime behavior is claimed.
- `git diff --check` passed.

## Outcome and follow-ups

- Added `internal/process` with an argv-only runner, complete explicit child
  environment, supplied-`PATH` executable resolution, absolute working
  directories, separate redacted stdout/stderr capture and sinks, timing, and
  typed exit, cancellation, sink, output-limit, pipe, start, and shutdown
  failures. Returned errors never retain sink text or child output.
- Every child starts in a fresh process group. Cancellation, sink failure, and
  output overflow trigger group-wide TERM then KILL under caller-configurable
  hard bounds; the direct child is reaped and both pipes are drained. Owned
  descendants are also removed when the group leader exits first.
- Secret registration is completed before process start and rejects empty,
  oversized, excessive-count, or excessive-total registries. A shared immutable
  trie drives independent stateful stream redactors, choosing the longest match
  and retaining possible prefixes across reads.
- Streaming sinks are synchronous and may apply backpressure; as with Go's
  `io.Writer` contract, a sink must eventually return. A sink that returns an
  error or short write is disabled, the process group is terminated, and both
  pipes continue draining. The runner does not claim it can interrupt an
  arbitrary writer implementation blocked forever inside `Write`.

## Original request

Implement implement the safe subprocess runner for Agent Box according to the cited specification and dependencies.
