# Adoption and Project Health

Agent Box measures whether the software and community are sustainable; it does not use foundation membership, stars, downloads, or roadmap volume as substitutes for health.

## Current baseline

The project is pre-alpha, has no supported release, one maintainer, no appointed reviewers, no published adopters or integrations, no staffed security rotation, and no measured issue or review baseline. Unknown values remain “not measured”; they are not reported as zero success or inferred from repository activity.

## Quarterly public snapshot

Once public contribution traffic exists, maintainers publish a quarterly snapshot in an issue or repository document with source queries and the reporting window. It tracks:

| Area | Measure |
| --- | --- |
| Adoption | Named opt-in repeat users, maintained integrations, published case studies, and release-to-release retention |
| Contributors | Unique merged contributors; first-time and returning contributors; contributor and maintainer affiliation distribution |
| Reviews | Median and 90th-percentile time to first substantive review and merge, with bot-only activity excluded |
| Issues | Opened/closed counts, median age, stale count, and severity-labeled backlog |
| Delivery | Release cadence, release-gate pass rate, regressions, rollbacks, and time to remediate broken main |
| Security | Supported versions, private-channel availability, acknowledgement/remediation performance against any published targets, and unresolved advisories |
| Reliability | Acceptance-matrix pass rate, platform evidence coverage, flaky-test rate, and escaped lifecycle or cleanup defects |
| Upstream | Issues and patches contributed to Lima, OCI, Compose, Go, or other dependencies, including accepted outcomes |
| Sustainability | Active reviewers and maintainers, organizations represented, bus factor, succession coverage, and conduct/security staffing gaps |

Raw counts are accompanied by definitions and context. Maintainers do not publish reporter identities, vulnerability details, private conduct data, or deanonymizing slices. Affiliation is self-disclosed and used to identify concentration, not rank people.

## Decision thresholds

Metrics trigger investigation rather than automatic punishment. Maintainers record corrective actions when review latency, stale critical issues, flaky gates, security response, or bus factor worsens across two snapshots. Release gates remain binary where their policy says so; favorable community metrics cannot waive a failed safety gate.

A neutral foundation home may be discussed only after evidence shows repeat users, maintained downstream integrations, multiple active maintainers from more than one organization, reliable releases, an operational security process, and community demand for neutral stewardship. Meeting those conditions starts a public decision; it does not guarantee acceptance or make foundation status a product milestone.
