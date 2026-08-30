<!-- sprout-task
{"schema_version":1,"id":"m5-governance-readiness","status":"implemented","created_at":"2026-08-29T06:48:36.686316691Z","implemented_at":"2026-08-30T09:14:55.299751018Z"}
-->

# Establish credible project governance and maintenance

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Establish credible project governance and maintenance

## Context

- Milestone 5. Dependencies: m1-project-foundation. Source: THE_BEGINNING.md Sections 22.4 and Foundation Readiness.

## Requirements

- Publish roadmap, governance, maintainer ladder, decision process, contribution path, release policy, compatibility policy, telemetry policy, and trademark/name review status.
- Choose and document DCO or CLA mechanics and issue/review expectations.
- Define adoption and project-health metrics without treating foundation membership as a release goal.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.

## Acceptance criteria

- [x] A contributor can discover how decisions, reviews, releases, security reports, and maintainer promotion work.
- [x] Governance permits maintainers from multiple organizations.
- [x] No document claims CNCF or Linux Foundation readiness without measured community evidence.

## Technical approach

- Keep policy at stable, discoverable repository-root entry points: `GOVERNANCE.md`
  owns roles and decisions, `MAINTAINERS.md` owns the current roster,
  `ROADMAP.md` owns direction, and focused release, compatibility, telemetry,
  trademark, and project-health documents own their respective contracts.
- Extend `CONTRIBUTING.md` with the DCO choice, issue/PR/review flow, and links
  into governance. Index every policy from `README.md`; do not duplicate the
  existing ADR, conduct, or security processes.
- Be explicit about bootstrap reality: one current maintainer, no operational
  private vulnerability channel, no release, no trademark clearance, and no
  measured adoption baseline. Permit promotion regardless of employer while
  requiring public evidence, conflict disclosure, and documented votes.
- Add an offline governance-document conformance test to the existing
  `automation-check` gate. Validate local links and the clean-clone path from a
  temporary tracked-file export rather than relying on review by inspection.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [x] Implement the smallest complete change.
- [x] Run every verification item and address failures.
- [x] Record concrete validation evidence and completion notes in this task.

## Verification

- [x] Validate governance document links and contributor flow
- [x] Perform a clean-clone contribution-path review

## Validation evidence

- `python3 -m unittest scripts.tests.test_governance_docs -v` passed all
  four governance conformance tests: every local link resolved inside the
  repository, `README.md` indexed every policy, the contribution path named
  checks/reviews/DCO, and governance exposed roles, decisions, multi-organization
  eligibility, and current gaps.
- `make automation-check` passed, including all seven repository automation
  tests and the new governance-document test suite.
- `make check && go test -race -count=10 ./... && GOOS=darwin GOARCH=arm64
  CGO_ENABLED=0 go build ./...` exited zero on the completed working tree.
  The Darwin build is compilation evidence only, not Lima/VirtioFS runtime
  evidence.
- A temporary clean checkout was materialized from exactly the prospective
  tracked and untracked repository inputs, committed in an isolated temporary
  Git repository, and confirmed clean before following `CONTRIBUTING.md`.
  `make check`, `make build`, `./bin/abox --version`, and
  `./bin/aboxd --version` all passed. The first-run path installed only pinned
  `golangci-lint v2.13.1` under `.tools/bin`; lint reported zero issues, module
  verification passed, and unit, race, vet, build, smoke, and automation gates
  passed.
- A repository-wide Markdown search found no claim that Agent Box is
  foundation-ready, CNCF-ready, ready for CNCF/Linux Foundation, or a Linux
  Foundation chartered project. `PROJECT_HEALTH.md` instead records the
  unmeasured pre-alpha baseline and evidence required before any neutral-home
  discussion.

## Outcome and follow-ups

- Published a discoverable roadmap, governance model, maintainer/reviewer
  roster, promotion and succession ladder, decision and review process, DCO
  mechanics, release policy, compatibility/deprecation contract, no-telemetry
  policy, trademark review status, and adoption/project-health measures.
- Recorded bootstrap reality rather than inventing maturity: one independent
  maintainer, no independent review, no operational private vulnerability
  channel, no supported release, no trademark clearance, and no measured
  adoption baseline. Governance explicitly permits maintainers from multiple
  organizations without employer ownership or voting weight.
- Added offline link and contributor-flow conformance to the existing
  `automation-check` gate. The private security channel, release supply chain,
  supported-version matrix, operational release runbook, name clearance, and
  broader maintainer recruitment remain honest pre-release work owned by their
  respective policies and milestone packets; none was simulated here.

## Original request

Implement establish credible project governance and maintenance for Agent Box according to the cited specification and dependencies.
