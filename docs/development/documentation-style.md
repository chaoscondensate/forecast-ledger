# Documentation style guide

<!-- doc-metadata
coverage: development
reviewed: 2026-08-25
owner: documentation
generated: false
security-critical: false
prerequisites: page-metadata.md
next: visual-style.md
-->

Reviewed: 2026-08-25

Use this guide for public repository prose, examples, generated-reference
introductions, policy files, issue forms, and release notes.

## Voice and language

- Write in plain international English.
- Address the reader as “you” when giving a safe action. Use “the project,”
  “the CLI,” or a named component for responsibility.
- Prefer short sentences, active voice, and concrete verbs.
- Explain an uncommon term on first use and link to the glossary when exact
  meaning matters.
- Avoid idioms, jokes, cultural references, unnecessary Latin abbreviations,
  and words whose only purpose is to sound technical.
- Do not describe Preview or Experimental behavior as production-ready,
  battle-tested, secure, safe, trusted, guaranteed, or proven.

## Product terms and capitalization

- Use **Forecast Ledger CLI** for the product and `forecast-ledger` for the
  executable or command.
- Use **Forecast Ledger** for the data contract or a ledger conforming to that
  contract. Do not shorten it to “FL” in public prose.
- Use **MCP** after writing “Model Context Protocol (MCP)” once on a page for a
  general audience.
- Use **RFC 3161 timestamp authority (TSA)** on first use and **TSA** afterward.
  Use **timestamp response** for the signed token returned by the authority.
- Use **Chaos Condensate** only for the broader project context and link to
  <https://chaoscondensate.com/> when that context is useful.
- Capitalize maintained maturity labels: Development, Preview, Stable,
  Experimental, Deprecated, and Unsupported.
- Follow the [claims and evidence terminology](../explanation/verification-claims.md).

## Headings and page shape

- Use one descriptive level-one heading that states the page's primary need.
- Increase heading levels one step at a time. Do not use headings only for
  visual emphasis.
- Start task pages with prerequisites before the first command.
- Keep tutorials, how-to guides, reference, and explanations distinct. A
  how-to solves one task; a tutorial teaches a complete journey; reference
  states exact facts; explanation supplies context and reasoning.
- End maintained pages with a relative route to their section index and the
  documentation index.

## Links

- Use descriptive link text that makes sense without the surrounding sentence.
  Do not use “click here,” “this,” or a bare URL as link text in prose.
- Prefer relative links for repository files so source archives work offline.
- Link to a versioned or exact-commit technical source when mutable content
  could change the meaning.
- Do not use the Chaos Condensate website, a badge, a hosted file timestamp, or
  repository history as a normative source for CLI behavior.
- Keep enough local safety and installation information that an external-link
  outage does not hide an essential action.

## Commands and code

- Add a language to every fenced code block: `sh`, `powershell`, `json`,
  `yaml`, `go`, `ruby`, or `text` as appropriate.
- Use `sh` only for portable POSIX shell. Label PowerShell commands separately.
- Make commands copyable. Do not include a shell prompt character or comments
  inside a copyable command block unless the page explains them.
- Use explicit files in every ledger example, normally `--file ledger.yaml`.
- Never put a key, credential, secret forecast, private ledger value, or
  security token in argv, an environment variable, output, or a transcript.
- Show representative output only when it is generated or semantically checked.
  Mark shortened output with an explicit explanation outside the code block.
- State network and filesystem side effects before a command that causes them.

## Callouts and safety

Use GitHub-compatible callouts only when ordinary prose would hide a material
decision:

```markdown
> [!NOTE]
> Extra context that does not change safety.

> [!IMPORTANT]
> A prerequisite or limitation needed for a correct result.

> [!WARNING]
> A risk of data loss, secret disclosure, irreversible action, or false claim.
```

Name the risk in text. Never rely on an icon, color, or callout type alone.
Place key-loss, overwrite, network-trust, and irreversible-action warnings
before the command, not after it.

## Versions, maturity, and deprecation

- State the product version or range a page covers when behavior can differ.
- State the schema version, exact commit, and digest on generated interface
  reference pages.
- Label unreleased behavior **Development**, not Stable.
- Label a component **Experimental** next to the first use on every workflow
  page where its additional risk matters.
- A deprecation notice names the deprecated behavior, replacement, first
  deprecated release, planned removal release when known, and a concrete
  replacement action.
- Removed behavior remains in changelog history for supported releases; do not
  silently delete guidance users may have relied on.

## Evidence claims

- Name the exact validation or verification layer and its result.
- Distinguish structural validity, content binding, existence timing, reveal
  validity, outcome evidence, authorship, completeness, and truth.
- Do not call a pending timestamp response verified.
- Do not turn source-control, filesystem, hosting, archive, or self-reported
  timestamps into cryptographic evidence.
- Do not state or imply that successful validation proves authorship,
  completeness, forecast truth, exact self-reported time, or substantive
  outcome-source correctness.
- Avoid an unqualified “proof” or “verified.” Name the protocol, layer, and
  conclusion.

## Review checklist

Before merging prose, verify that it:

1. states one primary user need;
2. matches the candidate binary and pinned contract;
3. identifies prerequisites, side effects, and limitations before commands;
4. uses the approved terms and maturity labels;
5. contains language-tagged code fences and descriptive relative links;
6. exposes no secret or private material; and
7. remains understandable without images, badges, or external services.

[Development documentation](index.md) · [Documentation index](../index.md)
