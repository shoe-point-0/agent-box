<!-- sprout-task
{"schema_version":1,"id":"m5-governance-readiness","status":"implemented","created_at":"2026-08-29T06:48:36.686316691Z","implemented_at":"2026-08-30T09:14:55.299751018Z"}
-->

# Project governance and maintenance

Status: implemented.

This milestone publishes Agent Box's discoverable repository-root governance,
contribution, release, compatibility, telemetry, trademark, and project-health
documents, enforced by offline link and contributor-flow conformance tests in
`make automation-check`.

Every document records honest pre-alpha bootstrap reality rather than claiming
unmeasured maturity: one independent maintainer, no independent review yet, no
operational private vulnerability channel, no supported release, no trademark
clearance, and no measured adoption baseline. Governance explicitly permits
maintainers from multiple organizations without employer ownership or voting
weight, and no document claims CNCF or Linux Foundation readiness.

## Scope and dependencies

- Milestone 5 governance and contributor readiness. Depends on [project foundation](project-foundation.md). Source: `THE_BEGINNING.md` Sections 22.4 and Foundation Readiness.

## Behavior

- Publishes dedicated repository-root policy documents indexed from [README.md](../../README.md): [ROADMAP.md](../../ROADMAP.md), [GOVERNANCE.md](../../GOVERNANCE.md), [MAINTAINERS.md](../../MAINTAINERS.md), [CONTRIBUTING.md](../../CONTRIBUTING.md) (DCO sign-off, issue/PR/review workflow, and verification steps), [RELEASES.md](../../RELEASES.md), [COMPATIBILITY.md](../../COMPATIBILITY.md), [TELEMETRY.md](../../TELEMETRY.md) (no-telemetry policy), [TRADEMARKS.md](../../TRADEMARKS.md), and [PROJECT_HEALTH.md](../../PROJECT_HEALTH.md).
- Documents open multi-organization maintainer promotion criteria (public evidence, conflict disclosure, and documented votes) alongside explicit current bootstrap gaps.
- Enforces local link integrity, `README.md` policy indexing, contributor-flow coverage, and honest bootstrap disclosures automatically via `scripts/tests/test_governance_docs.py` inside `make automation-check`.

## Constraints

- Support claims require real execution evidence on the claimed platform.
- No document claims foundation readiness, private security operations, release support, trademark clearance, or adoption metrics without measured evidence.

## Verified acceptance criteria

- Contributors can discover decision processes, review expectations, DCO sign-off, release and compatibility policies, security reporting, and maintainer promotion directly from [README.md](../../README.md).
- Governance allows maintainers from multiple organizations without employer control.
- Offline conformance tests validate links and clean-clone contributor onboarding end to end.

## Implementation details

- Policy ownership is split cleanly across focused root Markdown files without duplicating existing ADR ([docs/decisions/README.md](../decisions/README.md)), conduct (`CODE_OF_CONDUCT.md`), or security (`SECURITY.md`) processes.
- `scripts/tests/test_governance_docs.py` runs inside `make automation-check` to verify every relative link across governance documents, confirm `README.md` indexes each policy, and assert multi-organization eligibility and honest gap disclosures.

## Verification

- [x] Validate governance document links and contributor flow
- [x] Perform a clean-clone contribution-path review

## Validation evidence

- `python3 -m unittest scripts.tests.test_governance_docs -v` passed all four governance conformance tests (local link resolution, `README.md` indexing, contribution checks/reviews/DCO flow, and governance roles/decisions/multi-organization eligibility/current gaps).
- `make automation-check`, `make check` (`0` lint issues), `go test -race -count=10 ./...`, and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` (compilation evidence only) passed.
- Clean-clone contributor verification exported tracked and untracked files into an isolated temporary Git repository and executed `CONTRIBUTING.md` from scratch: `make check`, `make build`, `./bin/abox --version`, and `./bin/aboxd --version` all succeeded (bootstrapping only pinned `golangci-lint v2.13.1` into `.tools/bin`, reporting `0` issues).
- Repository-wide Markdown audit confirmed zero claims of CNCF or Linux Foundation readiness, with [PROJECT_HEALTH.md](../../PROJECT_HEALTH.md) recording the pre-alpha baseline and required evidence before any neutral-home discussion.
