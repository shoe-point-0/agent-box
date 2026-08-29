<!-- sprout-task
{"schema_version":1,"id":"m1-machine-git-profile","status":"open","created_at":"2026-08-29T06:55:50.379176125Z"}
-->

# Harden every Machine-side Git invocation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Harden every Machine-side Git invocation

## Context

- Milestone 1 security prerequisite. Dependencies: m1-process-runner. Source: THE_BEGINNING.md Sections 10 and 16.6.

## Requirements

- Define one controlled Git environment and argv profile for source observation, seeding, fetch, bundle import, reachability, and push.
- Disable unintended hooks, pagers, editors, fsmonitor, replacements, alternates, environment config, unsafe protocols, remote helpers, and option injection while preserving explicitly supported credentials.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Adversarial source/global/system Git config cannot execute marker commands or redirect operations.
- [ ] Crafted refs, URLs, helpers, hooks, askpass, sshCommand, filters, and ext transports are rejected or inert.
- [ ] All Machine Git adapters use this profile.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run the real-Git adversarial execution-profile suite

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement harden every machine-side git invocation for Agent Box.
