<!-- sprout-task
{"schema_version":1,"id":"m1-compose-adapter","status":"implemented","created_at":"2026-08-29T06:48:36.151865633Z","implemented_at":"2026-08-30T01:14:02.342658598Z"}
-->

# Implement Compose lifecycle, exec, logs, and observation

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Implement Compose lifecycle, exec, logs, and observation

## Context

- Milestone 1. Dependencies: m1-process-runner, m1-compose-compiler, m1-compose-policy-core, m1-guest-toolchain, m1-ownership-core. Source: THE_BEGINNING.md Sections 9.3, 12.2, and Issue 8.

## Requirements

- Observe, pull/build, apply, inspect, stop, and remove using an explicit project name and generated files only after normalized policy validation succeeds.
- Observe containers, health, networks, volumes, images, labels, and actual port bindings.
- Propagate cancellation and preserve bounded diagnostics while supporting streaming commands.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] A second apply does not duplicate Compose resources.
- [x] Every observed resource is attributable by recorded ID and labels.
- [x] Exec returns exact exit status and logs stop cleanly on cancellation.

## Technical approach

- Add a typed runtime adapter beside the read-only Compose compiler. Bind every
  invocation to the immutable Plan, positive generation, exact generated root
  and override paths, explicit project name, pinned Docker path, and the Lima
  guest invocation boundary; never accept caller-selected Compose identity.
- Before pull, build, apply, stop, remove, exec, or logs, rerun Compose
  normalization through the generated files, apply the existing normalized
  policy checks, and byte/digest-compare it with the compiler's retained model.
  A changed Plan, generated file, model, label set, or policy therefore stops
  before mutation.
- Observe reality from Docker JSON rather than human Compose output: enumerate
  project containers, networks, and volumes by the explicit Compose project
  label, inspect their opaque IDs, ownership labels, state/health, image IDs,
  network attachment, mounts, and actual port bindings, then inspect every
  referenced image. Sort and validate the complete observation deterministically.
- Make apply idempotent with `compose up --detach --remove-orphans` followed by
  fresh observation. Keep noninteractive exec and follow logs streaming through
  the existing bounded, cancellation-aware process boundary, preserving its
  exact exit result and typed cancellation. Require recorded ID/label ownership
  proofs for the complete live resource set before destructive removal.
- Prove the command and decoding contracts with fake guest fixtures, then run a
  uniquely named disposable project against the available Linux Docker Engine
  and Compose runtime. Exercise apply twice, observation, exact exec status,
  canceled follow logs, stop, ownership refusal, removal, and leak checks.

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
- [x] Run Compose adapter contract and real-runtime integration tests

## Validation evidence

- `TestRuntimeLifecycleAgainstRealDocker` passed against the locally executed
  Linux Docker Engine 29.6.1 / Compose 5.3.0 boundary using a uniquely named,
  disposable two-service project with one named volume. It applied the same
  generated model twice and byte-compared the unchanged container, network,
  and volume ID sets; observed two containers, one network, one volume, one
  referenced image, live state/health, mounts, network attachments, labels,
  and zero actual Machine port bindings; and removed every owned resource.
- The real fixture returned exact exec status `37`, streamed a live log line,
  canceled follow logs within the three-second assertion deadline, stopped
  both containers, refused a forged recorded Box label without changing the
  live project, then removed the freshly proved recorded set. Post-removal
  Docker queries for project-labeled containers, networks, volumes, and images
  were empty. The same fixture passed three targeted race repetitions and ten
  full-repository race repetitions.
- `TestRuntimePullBuildAndPolicyPreflightCommandContract` proved pull and build
  receive the explicit project, every selected profile, and only the exact
  generated root/override files. It also proved changed normalized privilege
  policy stops after the read-only config command and before pull mutation.
- The compiler now projects the five ownership labels onto every declared
  named volume as well as services and the default network. Runtime observation
  records opaque IDs plus only the non-empty attribution label subset; removal
  requires the complete recorded container/network/volume set to pass fresh
  `ownership` proofs. Images are observed by immutable image ID and their exact
  container references but deliberately retained as potentially shared data.
- `make check` passed formatting, pinned lint (`0 issues`), module verification,
  all unit/integration tests, the full race suite, vet, builds, smoke, and
  automation checks. `go test -race -count=10 ./...` passed; the Compose package
  completed ten real lifecycle fixtures in 259.754s. `GOOS=darwin GOARCH=arm64
  CGO_ENABLED=0 go build ./...` and `git diff --check` passed; cross-compilation
  is compilation evidence only and makes no Darwin runtime claim.

## Outcome and follow-ups

- Added the typed `compose.Runtime` guest boundary for policy-gated observe,
  pull, build, idempotent apply, stop, ownership-proved remove, noninteractive
  exec, and bounded/cancelable logs. Every operation re-normalizes through the
  exact generated inputs and compares canonical bytes and digest to compiler
  evidence before mutation or streaming execution.
- Docker JSON inspection is deterministic and fail-closed for project
  containers, networks, volumes, referenced images, ownership labels, health,
  mounts, attachments, and actual published ports. Human-formatted Compose
  output is never parsed.
- This Linux host does not have `limactl`; production invocation through a real
  Lima VM remains covered by the typed `lima.Adapter` contract rather than a
  runtime support claim. The real integration exercised the same adapter over
  the local Linux Docker Engine. Darwin VZ/VirtioFS remains hardware-deferred.

## Original request

Implement implement compose lifecycle, exec, logs, and observation for Agent Box according to the cited specification and dependencies.
