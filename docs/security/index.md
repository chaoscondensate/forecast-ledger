# Security documentation

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-23
owner: security
generated: false
security-critical: true
prerequisites: ../getting-started/index.md
next: ../explanation/verification-claims.md
-->

Forecast Ledger CLI is Preview software and has not received a recorded
independent security or cryptographic audit. Before using it with private
material, review:

A stable `0.x` release identifies the supported package channel. It does not
mean that the cryptographic implementation has been independently audited or
formally verified.

- the [evidence, maturity, and audit baseline](../development/documentation-baseline.md);
- the [dependency security review](../development/dependencies.md); and
- the [README evidence boundaries](../../README.md#evidence-boundaries); and
- the [RFC 3161 trust and privacy limits](../how-to/timestamp-forecasts.md);
- the [package allowlist and manifest checks](../how-to/publish-evidence.md);
- the [MCP root, mode, and reveal boundaries](../how-to/run-mcp.md); and
- the repository [security policy](../../SECURITY.md).

Keep private operation input and protected key files under a separate secret
root. Do not place a secret root inside a ledger or package root. A sealed
ledger protects its forecast bundle only while the key
and private input remain separate. Reveal is irreversible publication of the
private fields and requires explicit approval.

The sealed nonce and ciphertext are public commitment material and are returned
by forecast inspection in CLI, JSON, MCP tools, and resources. They are not
secrets. Raw keys, salts, decrypted plaintext, private fields, credentials,
protected paths, and unrestricted parser or crypto errors remain redacted.

The v3 seal and target bind the forecast to one exact question revision. Keep
the protected `forecast-key/v3` file with the matching question, revision, and
forecast IDs; a key from another record must fail. The target contains the full
bound revision, so changing wording or domain requires a new revision and a new
forecast rather than editing old evidence.

Lifecycle events are deliberately outside the immutable target. Withdrawal,
expiry, and reaffirmation change the derived active state without invalidating
the original target or its timestamp. Consumers must check both evidence
validity and current activity instead of treating either as the other.

An activity checkpoint binds `forecast-lifecycle/v2` bytes for one exact event
prefix to retained RFC 3161 evidence. It can detect changes or deletion inside
that covered prefix. A later event makes older verified coverage partial; it
does not invalidate the older proof. No local format can prove that a deleted
event existed after the event, its checkpoint, and every independent copy of
their evidence have all been removed. Verification reports this completeness
limit instead of treating an unbound event stream as verified.

Ordinary public ledger values supplied through CLI flags are visible in process
listings, shell history, terminal logs, and job metadata. Do not place a private
forecast value, rationale, key factor, working comment, raw key, salt, or
credential in those flags or an environment variable. `forecast seal` accepts
those private fields only through protected `--secret-input` and writes keys
only to protected `--key-file` destinations. Sealed initial forecasts use
`--initial-secret-input` for the same reason. Generic public side-loaded
request documents are not supported.

Optional provenance snapshots are public evidence artifacts. Their paths are
confined relative to the ledger and their SHA-256 digests are checked locally;
the CLI does not follow a provenance source URL during normal validation. Do
not place credentials or private source material in snapshots intended for a
ledger or publication package.

`timestamp stamp` sends the SHA-256 digest of the canonical target, a random
nonce, and request timing to the selected RFC 3161 authority. Omission selects
the current FreeTSA HTTPS profile. It does not send forecast plaintext unless
the digest itself is already known to the authority. The authority learns when
the request was made and may retain network metadata. FreeTSA publishes no
numeric rate limit, measurable SLA, independent TSA audit, or succession
assurance in the reviewed first-party material.

Custom URLs are public HTTPS-only and may follow only bounded same-origin
redirects. Built-in profiles use exact compiled HTTPS or HTTP transport policy,
reject every redirect, and validate public DNS/IP results on each request. No
HTTP provider ships now. A future HTTP provider would expose the imprint,
nonce, and timing to network observers and remain vulnerable to observation,
blocking, and response substitution; the CMS signature and retained chain
prevent a substituted response from becoming verified timing.

Optional outcome-source retrieval uses a separate GET-only transport. It
ignores environment proxy settings, resolves each connection once, rejects the
whole answer set if any address is private or reserved, and dials only an
approved numeric address while keeping the original hostname for HTTP Host and
TLS identity checks. The same rule is applied after every bounded redirect.
Loopback, link-local, private, multicast, unspecified, documentation,
benchmarking, CGNAT, and IPv4-mapped forms are rejected. These checks limit
server-side request forgery; they do not make an outcome source authoritative
or prove its content true.

Later status, timestamp verification, layered verification, and package
verification are local. They trust only the CA bundle retained with the ledger,
not the operating-system root store. Built-in trust is copied beside the
evidence and never replaced during later verification. Preserve that bundle, the request, the
response, and the target together. A valid signature proves only that the named
authority issued the token for the digest at its asserted generation time.

Managed evidence is closed to `proofs/` and `trust/` and is reconciled against
`proofs/evidence-index.json`. Unindexed files make the result incomplete;
changed bytes, unrelated index entries, or bindings fail verification. A valid
detached lifecycle target is reported as
`activity.retained_evidence_unreferenced`, not as an unbound stream. Deleting
every event, checkpoint, index entry, artifact, and external copy still removes
the evidence that the event once existed.

A valid document or package manifest is a structural conclusion. Overall
verification `pass` additionally requires at least one applicable
forecast-evidence layer; empty selections return `no_evidence`.

Publication copies the exact ledger, canonical index, and every indexed
artifact. Always inspect the new package before sharing it, because an exact
ledger may contain fields intentionally disclosed by an earlier reveal.

Do not put keys, credentials, private ledgers, or unrevealed forecast material
in public issues. Suspected vulnerabilities belong in
[GitHub Private Vulnerability Reporting](https://github.com/chaoscondensate/forecast-ledger/security/advisories/new).
Conduct reports belong at `andrey@chaoscondensate.com`.

[Documentation index](../index.md)
