# Security Policy

## Current support status

Agent Box has no released or supported runtime version yet. The repository is pre-alpha, and current binaries implement version reporting only. Linux CI and macOS compilation are engineering evidence, not a claim that VM or container isolation is production-ready.

Security fixes will be prioritized, but the project does not yet publish a response or patch SLA. A supported-version matrix and response targets must be established before the first release candidate.

## Private reporting status

Do not open a public issue for a suspected vulnerability or include secrets, exploit details, or identifying data in logs.

GitHub private vulnerability reporting is currently disabled for this repository, and the project has not yet published another private security contact. The advisory URL reserved for that channel is:

https://github.com/shoe-point-0/agent-box/security/advisories/new

It will not accept a report until a repository administrator enables private vulnerability reporting. This is an explicit pre-release governance blocker, not an invitation to disclose publicly.

Include only what is needed to reproduce and assess the issue:

- affected commit or version;
- host OS and architecture;
- relevant Git, Lima, Docker, Compose, and kernel versions;
- boundary crossed or asset exposed;
- minimal reproduction steps;
- observed impact; and
- suggested mitigation, if known.

Never send real credentials. Replace secrets with canaries and redact unrelated paths or user data.

Until that channel is enabled, do not publish the report. Open a minimal public issue requesting activation of private security reporting without any vulnerability details, or wait for the channel to become available.

## Disclosure process

Maintainers will acknowledge the report when it is seen, validate the affected boundary, coordinate a fix and regression test, and agree on disclosure timing with the reporter when practical. No timeline is promised before maintainership and release operations are staffed.

A fix is not complete until the affected ownership, privilege, cleanup, and sibling paths are checked. Released fixes will include impact, affected versions, remediation, and credit when the reporter wants it.

## Security boundary

The current architecture and known non-claims are documented in `THE_BEGINNING.md`, `docs/architecture/substrate-spikes.md`, and accepted ADRs. A VM boundary is not a container boundary, and Machine integrations such as mounts, sockets, Git metadata, forwarding, and credentials can weaken isolation. Do not deploy the pre-alpha code as a security boundary.
