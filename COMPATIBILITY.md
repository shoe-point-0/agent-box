# Compatibility Policy

Agent Box currently has no supported release. Source, command behavior, schemas, and state on `main` may change without migration until a release explicitly declares otherwise. Versioned `v1alpha1` payload names prevent accidental ambiguity but do not make the pre-alpha contract stable.

## Versioned surfaces

The following surfaces evolve independently even when shipped together:

- CLI and daemon protocol;
- Workspace manifest;
- Plan and context documents;
- CLI JSON and event output;
- symbolic error codes and exit codes;
- SQLite state schema; and
- guest image and tool bundle.

Every persisted or exchanged format carries an explicit schema or protocol version. CLI and daemon communication is allowed only when declared protocol ranges overlap. Unknown required versions fail closed with an actionable error rather than being guessed.

## Pre-1.0 contract

A pre-1.0 release must publish the exact supported Machine OS/architecture, Lima, guest image, Docker Engine, Compose, Git, and schema matrix backed by release-candidate evidence. Cross-compilation proves compilation only.

Within one released minor line:

- patch releases do not remove JSON fields, symbolic error codes, flags, or supported configurations;
- readers tolerate additive optional fields where the schema permits them;
- state migration is forward-only and must preserve the previous database until successful replacement or provide an equivalently tested rollback boundary before release;
- generated files remain internal unless release notes explicitly promote a format; and
- a security fix may narrow unsafe behavior, with the incompatibility and remediation documented.

No current code claims the database backup boundary required for released migration. That gap blocks a release that needs such migration; documentation does not manufacture the mechanism.

## Deprecation and breaking changes

A supported public contract is deprecated in release notes and user-facing diagnostics for at least one minor release before removal. The notice names the replacement, migration steps, earliest removal version, and rollback limits. Before `v1.0.0`, a breaking change requires an ADR, a previewable migration when persisted user data or manifests are affected, and a minor-version increment. At and after `v1.0.0`, incompatible public changes require a major version except for documented security containment.

Unsupported internals may change without deprecation, but moving a public boundary to “internal” is itself a breaking change.

## Support claims

The release acceptance evidence is authoritative. Linux development tests, Darwin cross-builds, hosted macOS tests, and real Apple Silicon Lima/VirtioFS tests are distinct evidence classes and must be labeled as such. A configuration absent from the release matrix is unverified, not implicitly supported.
