# Agent Box Governance

Agent Box is governed in public around technical merit, demonstrated stewardship, and the project's safety invariants. Employment, employer sponsorship, geography, and organizational affiliation neither grant nor prevent authority. The project currently has one bootstrap maintainer; this document describes the model the project uses while growing beyond that bus factor.

## Roles

### Contributor

Anyone who reports an issue, improves documentation, proposes a decision, writes code, tests a release, or otherwise helps the project. Contributors have no commit or release authority by default.

### Reviewer

A contributor with sustained subject-matter experience whom maintainers trust to review a documented area. Reviewers may approve changes in that area but cannot merge, publish releases, appoint maintainers, or make project-wide decisions solely by being a reviewer. Reviewer scope is recorded in [`MAINTAINERS.md`](MAINTAINERS.md).

### Maintainer

A maintainer may merge changes, triage issues, participate in binding votes, and act as release manager. Maintainers are responsible for review quality, security and conduct escalation, release integrity, and enforcing compatibility and ownership boundaries. Their authority is project-wide unless the roster records a narrower temporary restriction.

A maintainer's employer does not own the seat. Maintainers act for the project, disclose conflicts, and recuse when their impartiality could reasonably be questioned. There is no organizational quota or veto. Maintainers from multiple organizations may serve concurrently, as may unaffiliated maintainers.

## Promotion and succession

Reviewer or maintainer candidates must show repeated, constructive work over time, sound judgment at the relevant trust boundaries, dependable reviews, and adherence to the Code of Conduct. Commit count or employer alone is not evidence of readiness.

Any maintainer may nominate a candidate in a public issue. The nomination must summarize contributions, proposed role and scope, affiliations, and known conflicts. Allow at least seven calendar days for community feedback. Promotion requires approval from at least two-thirds of non-conflicted maintainers and no unresolved substantiated conduct or security concern. With fewer than two maintainers, the bootstrap maintainer may appoint a candidate after the same public process and must record the rationale; this exception ends once two maintainers serve.

A maintainer may step down at any time. After six months without project participation, another maintainer may propose emeritus status through the same public notice and vote. Removal for cause requires a private evidence review, recusal of involved parties, and a two-thirds vote of non-conflicted maintainers. When fewer than three non-conflicted maintainers exist, repository access may be suspended to contain immediate risk, but permanent removal must be documented publicly without exposing private reporter data. Emeritus maintainers retain attribution, not authority, and may be nominated again normally.

The current roster and affiliations are authoritative in [`MAINTAINERS.md`](MAINTAINERS.md).

## Decisions

Routine implementation choices are made in pull requests by lazy consensus. The author states the changed boundary and evidence; reviewers surface blocking correctness, security, compatibility, lifecycle, or scope concerns. A maintainer may merge when required review and checks pass and no blocking concern remains.

Changes to durable architecture, public compatibility, persistence, security boundaries, destructive behavior, governance, or release policy require an Architecture Decision Record under [`docs/decisions/README.md`](docs/decisions/README.md). Proposed ADRs remain open for at least seven calendar days unless an actively exploited vulnerability requires an embargoed decision. Maintainers seek consensus first. If consensus fails, a two-thirds vote of non-conflicted maintainers decides; at least two approvals are required once the project has two maintainers. During bootstrap, the sole maintainer records the decision and dissent in the ADR or pull request.

Governance changes use a pull request, a linked public issue, fourteen calendar days for comment, and the same two-thirds vote. No maintainer may unilaterally change the promotion or removal rules once two maintainers serve.

Security fixes may be developed privately under [`SECURITY.md`](SECURITY.md). The public record may be delayed until coordinated disclosure, but the technical rationale and compatibility effect must be published afterward without reporter secrets.

## Reviews and merges

All changes use pull requests except an emergency response needed to stop active harm. A pull request needs:

- the boundary, motivation, tests, cleanup behavior, and remaining non-claims described;
- every commit signed off under the DCO policy in [`CONTRIBUTING.md`](CONTRIBUTING.md);
- required automated checks passing on each claimed platform; and
- approval from one non-author maintainer.

Governance, release-policy, compatibility-policy, and public security-boundary changes require two non-author maintainer approvals when two are available. During bootstrap, the sole maintainer may merge after the applicable public comment period and must not present self-review as independent review. Authors do not approve their own changes. Reviewers and maintainers disclose material employer, financial, or personal conflicts and recuse where appropriate.

For routine changes authored by the sole bootstrap maintainer, all mechanical checks must pass and the pull request remains open for at least seventy-two hours for public review before merge. The record must say that no independent maintainer approval was available. This bootstrap exception ends when a second maintainer is appointed.

Emergency changes must be narrowly scoped, documented within two business days, and reviewed retrospectively. This exception does not bypass release signing or permit silent compatibility breakage.

## Releases and security

[`RELEASES.md`](RELEASES.md) defines release authorization and evidence. [`COMPATIBILITY.md`](COMPATIBILITY.md) defines public stability and deprecation. Vulnerabilities follow [`SECURITY.md`](SECURITY.md); its currently unavailable private channel is a blocker that governance text cannot paper over.

## Conduct and amendments

Participation is governed by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Governance records are public unless privacy, security, or legal safety requires a narrower record. Amendments follow the governance-change process above.
