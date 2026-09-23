# Recover an interrupted ledger write

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-23
owner: security
generated: false
security-critical: true
prerequisites: ../getting-started/create-ledger.md
next: verify-evidence.md
-->

Forecast Ledger mutations use a sibling temporary file and a recovery journal.
If a process stops after the journal is durable, retry the same command with the
same `--file`. CLI and MCP writers acquire the normal ledger lock, validate the
journal and retained files, complete or clean the interrupted replacement, and
continue the new mutation without releasing that lock.

An automatic retry proceeds only when the ledger still matches the recorded
original or expected SHA-256 digest and any retained temporary sibling matches
the exact journal name and expected digest. The recovered bytes must parse and
pass the same schema, semantic, and retained-artifact validation as an ordinary
write. A journal left after replacement is cleaned only after the current
ledger is validated.

Target and timestamp operations also keep a resource journal for their target,
request, response, trust, evidence-index, and ledger effects. A failure before
any replacement removes only newly created, operation-owned public files. If a
crash journal records an index or ledger replacement, automatic cleanup keeps
every material file and the journal rather than guessing a rollback across
those identities. Back up that directory and reconcile the recorded before and
after SHA-256 values before retrying.

If the ledger changed, the temporary file is missing or changed, the journal is
malformed, or the prospective ledger is invalid, the command returns a
conflict or invalid-data error and preserves the relevant files. Do not edit or
delete a journal or its temporary sibling by hand. Make a backup of the ledger
directory and investigate which bytes are authoritative before trying another
write. A second concurrent writer cannot perform recovery because only the
holder of the normal writer lock reaches this step.

[How-to index](index.md) · [Security](../security/index.md)
