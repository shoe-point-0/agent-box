# Release Policy

Agent Box has not published a supported release. This policy defines how a release becomes authorized; it does not claim that the required release machinery exists today.

## Versioning and channels

Release versions follow Semantic Versioning. Before `v1.0.0`, minor versions may change unstable contracts only under [`COMPATIBILITY.md`](COMPATIBILITY.md); patch versions are backward-compatible fixes. Pre-release identifiers such as `-alpha`, `-beta`, and `-rc.1` indicate increasing validation, not production support by themselves.

A Git tag, source archive, locally built binary, or successful cross-build is not an Agent Box release. A release exists only when the complete approved artifact set is published through the documented release process.

## Authorization

A maintainer volunteers as release manager and opens a release pull request identifying the exact commit, version, change notes, compatibility impact, supported-platform evidence, known limitations, and rollback or revocation plan. The release requires:

- all gates for that channel passing on the exact commit;
- a clean dependency and license review;
- reproducible binaries for every claimed target;
- checksums, an SBOM, provenance, and signatures verifiable through published instructions;
- an updated supported-version and compatibility statement;
- no unresolved release-blocking security report; and
- two non-conflicted maintainer approvals when two are available.

During bootstrap, the sole maintainer may authorize a pre-release only after all mechanical evidence passes and the pull request records that independent maintainer review was unavailable. Stable `v1.x` releases require at least two maintainers. The release manager may not waive a failed gate by relabeling it a non-claim; a waiver requires a public policy or ADR change before release.

Signed artifacts, provenance, and release automation remain unimplemented until their owning milestone packets close. No release should be published before then.

## Cadence and support

Releases are need-driven, not calendar-forced. The project does not promise a cadence or patch SLA before it has a staffed maintenance rotation. [`SECURITY.md`](SECURITY.md) is authoritative for security response and supported versions.

Each release notes added behavior, fixes, breaking changes, migrations, deprecations, security impact, evidence matrix, and known limitations. Claims are limited to configurations exercised on the exact release candidate.

## Rollback and revocation

Before publication, the release manager verifies installation, upgrade, rollback, and signature checks from clean environments. If an artifact, signing identity, or release is compromised, maintainers stop distribution, mark the release withdrawn, publish the affected digests and reason, revoke or rotate credentials where supported, and issue remediation instructions. Tags and artifacts are not silently replaced. A corrected build receives a new version.

The operational runbook and dry-run evidence are owned by the release-operations milestone; this policy does not substitute for that work.
