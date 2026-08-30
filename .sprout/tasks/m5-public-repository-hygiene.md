<!-- sprout-task
{"schema_version":1,"id":"m5-public-repository-hygiene","status":"open","created_at":"2026-08-30T03:20:21.975903775Z"}
-->

# Prepare the repository for public project scrutiny

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Prepare the repository for public project scrutiny

## Context

- Milestone 5 final story. Dependencies: m5-rc-acceptance. Run after the development cycle and release-candidate evidence are complete so the review covers the repository in its intended public state.
- The target is the engineering and project-health standard expected of a credible Linux Foundation or CNCF project, not an unsupported claim of foundation endorsement or readiness.

## Requirements

- Inventory every tracked release-facing file, including source comments and user-visible strings, documentation, examples, scripts, CI, issue templates, generated artifacts, and repository-root metadata; classify content as public product documentation, durable maintainer documentation, or disposable development residue.
- Remove or rewrite internal development-cycle language, private agent/operator instructions, personal or machine-specific references, stale implementation notes, temporary workarounds, debug artifacts, obsolete TODO/FIXME/HACK markers, hype, and operational cruft that does not belong in a public release.
- Preserve useful technical rationale and contributor workflows by relocating or rewriting them as durable public documentation; do not erase known limitations, security boundaries, deferred decisions, provenance, or validation evidence.
- Audit packaging and release manifests so private workflow files, local state, test residue, credentials, logs, and development-only artifacts cannot enter source or binary distributions.
- Add a narrow automated repository-hygiene check with an explicit reviewed allowlist for prohibited local paths, private identifiers, development markers, unsupported foundation claims, and release-forbidden artifacts.

## Constraints

- Do not commit, push, publish a release, apply for foundation membership, or mutate shared infrastructure as part of this task.
- Do not turn this into cosmetic history rewriting or delete evidence needed for security, licensing, attribution, architecture decisions, support claims, or reproducibility.
- Linux Foundation and CNCF names may describe an engineering bar or factual ecosystem relationship only; never imply endorsement, acceptance, membership, or readiness without independently verifiable evidence.

## Acceptance criteria

- [ ] A clean-clone reviewer can understand what Agent Box does, its support and security boundaries, how to install and operate it, how decisions are made, and how to contribute without encountering private development-session language or unexplained internal jargon.
- [ ] Tracked files and release artifacts contain no credentials, machine-local paths, personal identifiers, disposable logs, debug output, stale scratch notes, private agent prompts, or unreviewed development-only artifacts.
- [ ] Every retained TODO, FIXME, exception, and internal-looking file is documented by a reviewed allowlist entry with a public maintenance reason and owner or tracking reference.
- [ ] Foundation-related language is factual and evidence-backed, and the repository makes no claim of Linux Foundation or CNCF endorsement, acceptance, membership, or project readiness.
- [ ] Public documentation links, examples, package contents, licensing and attribution files, and contributor entry points remain complete and internally consistent after cleanup.

## Technical approach

_Refine this section after inspecting the repository and before editing._

## Execution checklist

- [ ] Inspect the relevant code, tests, and repository instructions.
- [ ] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] Run the automated repository-hygiene scan against all tracked files and fail on unallowlisted findings
- [ ] Build source and binary release artifacts, inspect their file manifests, and scan extracted contents for forbidden development residue and secrets
- [ ] Run documentation link and example validation plus the full repository quality gate
- [ ] Perform and record an independent clean-clone public-readiness review from adopter, contributor, maintainer, security-reporter, and downstream-packager perspectives

## Validation evidence

_Not recorded yet._

## Outcome and follow-ups

_Not completed yet._

## Original request

Remove internal development language and operational cruft from the completed Agent Box story, leaving a sober, maintainable, foundation-caliber public repository.
