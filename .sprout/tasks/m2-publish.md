<!-- sprout-task
{"schema_version":1,"id":"m2-publish","status":"open","created_at":"2026-08-29T06:48:36.343840374Z"}
-->

# Publish Box commits through a sanitized Machine path

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Publish Box commits through a sanitized Machine path

## Context

- Milestone 2. Dependencies: m1-machine-git-profile, m1-git-publication-status, m2-remote-repositories, m2-git-metadata-policy. Source: THE_BEGINNING.md Section 10.3.

## Requirements

- Produce exact-ref Git bundles inside the Box and validate/import them into a fresh controlled temporary bare repository.
- Push to the trusted Plan destination with Machine credentials and hooks disabled.
- Detect unpublished commits and require explicit policy for non-fast-forward publication.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [ ] Selected commits reach the declared local or remote destination.
- [ ] Box-writable hooks, config, core.sshCommand, filters, and destinations cannot execute or redirect Machine publication.
- [ ] Temporary repositories and bundle material are removed after recorded success or failure.

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
- [ ] Run adversarial publish integration tests with malicious Box Git configuration

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Implement publish box commits through a sanitized machine path for Agent Box according to the cited specification and dependencies.
