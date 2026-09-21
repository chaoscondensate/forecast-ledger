## Context

See `proposal.md` for motivation. The in-progress v2 cutover currently keeps a frozen v1.3 schema subtree, v1 fixtures and seal vectors, a Python legacy verifier, release-pin checks, migration documentation, and OpenSpec requirements that preserve those bytes. Runtime behavior is already v2-only, but the retained material makes the repository and public documentation look as if v1 remains a supported compatibility concern.

The deletion overlaps the active `adopt-forecast-ledger-v2` change. Its artifacts must be reconciled in the same implementation before either change is archived. Exact v2 schema bytes, v2 fixtures, v2 target/seal vectors, and the generic pre-side-effect rejection of unsupported schema versions remain authoritative.

## Goals / Non-Goals

**Goals:**

- Leave one production and conformance contract: Forecast Ledger v2.
- Remove every repository-owned promise to preserve, validate, inspect, convert, or explain v1 data.
- Keep unsupported-version rejection generic, local, and side-effect-free.
- Make release and documentation checks prove that no migration surface or legacy bundle returns.
- Keep only a concise historical breaking-change statement in the changelog.

**Non-Goals:**

- Converting or relabeling any v1 file.
- Preserving an offline v1 verification utility or downloadable compatibility archive.
- Adding deprecation warnings, a grace period, feature flags, or dual-schema support.
- Removing generic uses of “compatibility” that concern operating systems, MCP protocol versions, packages, or current v2 representations.

## Decisions

### Delete legacy material instead of moving it

The v1 schema subtree, fixtures, vectors, digest manifests, and legacy-only tooling will be deleted. They will not be moved to a test-only, archive, examples, or `third_party` directory. Moving them would retain the same maintenance and review obligation under a different path.

Alternative considered: keep a frozen regression bundle outside the runtime embed. Rejected because there are no active users or historical ledgers that require local verification.

### Keep a generic version gate

Ledger loading will continue to compare the declared schema version with the single current version and return `unsupported_schema_version` before mutation, secret creation, artifact creation, or network access. Tests may construct a small unsupported-version document inline, but SHALL NOT vendor a v1 schema or fixture to do so. Error text and public metadata will name the supported v2 version without offering v1-specific instructions.

Alternative considered: let the v2 schema reject old files later. Rejected because an early version gate gives a clearer error and preserves the no-side-effect boundary without constituting backward compatibility.

### Retain only v2 upstream inputs

The vendored upstream subset will contain only files needed to validate v2 behavior and reproduce v2 conformance or cryptographic outputs. V1 compatibility documents, legacy manifests, v1 vectors, and legacy-only reference tools will be removed rather than edited. Application-owned provenance will record only the retained v2 files and their exact digests.

Alternative considered: keep the complete upstream release tree for provenance. Rejected because the complete tree includes the legacy surface this change intentionally stops carrying; exact identity is required per retained file, not by retaining unrelated files.

### Remove guidance instead of replacing it

The maintained migration page and upstream compatibility page will be deleted, their navigation and links removed, and contributor/release instructions simplified to v2-only checks. `CHANGELOG.md` will contain one short breaking-change note stating that no migration is provided because there were no active users. No replacement migration checklist or preservation advice will be created.

Alternative considered: leave a “no migration” compatibility page. Rejected because a dedicated page still advertises compatibility as a supported product concern; the changelog is sufficient historical context.

### Reconcile overlapping OpenSpec artifacts before archive

The active v2 proposal, design, specs, and tasks will be updated to remove requirements for frozen v1 evidence, migration guidance, legacy Python parity, and independent review of deleted legacy paths. The new change will remove the two v1.3 retention requirements that remain after the v2 cutover; the two old runtime-v1 requirements are already removed by the v2 change. Archive order must preserve a coherent main spec: finish/reconcile the v2 change first, then apply and archive this removal.

Alternative considered: archive the v2 change unchanged and override it later. Rejected because that temporarily establishes requirements the project has already decided not to honor and creates conflicting task status.

## Risks / Trade-offs

- [A previously unknown v1 file appears later] → The CLI will reject it safely; recovery would require checking out an old release or using upstream source, not restoring compatibility to the current product.
- [A broad text search deletes unrelated compatibility guidance] → Classify every match and restrict deletion to Forecast Ledger v1/migration content; retain OS, package, protocol, and current-v2 compatibility documentation.
- [Removing mixed upstream tools accidentally weakens v2 conformance] → Replace each affected assertion with v2 fixture/vector coverage before deleting the tool, then rerun the complete conformance suite.
- [Concurrent OpenSpec changes conflict at archive time] → Reconcile `adopt-forecast-ledger-v2` in the implementation and validate all active changes strictly before archive.
- [The changelog overstates product history] → Use the narrow factual statement supplied by the product owner: v2 is breaking, no migration is provided, and there were no active users to migrate.

## Migration Plan

There is no user-data migration. Implementation is a repository cleanup performed before the v2 release:

1. Reconcile the active v2 OpenSpec artifacts and establish the final v2-only requirement set.
2. Remove legacy schema, fixture, vector, tooling, and documentation files and their code references.
3. Regenerate contracts and documentation indexes from the v2-only registry.
4. Run absence checks, v2 conformance, cryptographic vectors, documentation checks, security checks, and release snapshots.
5. Archive the v2 cutover before archiving this cleanup so the resulting main specs retire the v1 capability cleanly.

Rollback, if needed before release, is a source-control revert of this change. It is not a supported runtime migration path.
