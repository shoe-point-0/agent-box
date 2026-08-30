# Telemetry Policy

Agent Box does not collect or transmit product analytics or usage telemetry. v0.1 will have no telemetry by default and no hidden installation identifier, background analytics request, or automatic upload of commands, paths, repository data, logs, diagnostics, environment values, or crash data.

Local operational state, logs, and diagnostics needed to run or debug a Box are not telemetry while they remain on the Machine. Their collection must still follow documented redaction, ownership, retention, and deletion behavior. A user choosing to attach diagnostics to an issue is an explicit disclosure to that destination, not automatic telemetry; users must inspect and redact the bundle first.

Network traffic required by an explicit user operation—such as fetching source, images, or pinned runtime components—is not analytics. Such traffic must be attributable to the operation and must not carry unrelated usage data.

Any future telemetry proposal requires a public ADR and governance review before implementation. The proposal must specify:

- the user problem and why local diagnostics are insufficient;
- every field, event, destination, processor, and retention period;
- opt-in and disable mechanics, with collection disabled until affirmative consent;
- local inspection and deletion controls;
- treatment of paths, repository identity, commands, IP addresses, and other sensitive data;
- security and privacy review; and
- tests proving no transmission while disabled.

Changing this policy is a compatibility and governance change. Release notes must disclose it prominently; telemetry may not be enabled through acceptance of unrelated terms or a silent upgrade.
