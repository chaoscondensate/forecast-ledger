# Manage platform records

<!-- doc-metadata
coverage: v0.10.0
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: false
prerequisites: ../getting-started/create-ledger.md
next: ../reference/index.md
-->

Platform records describe external or local places associated with questions.
They do not publish data or contact a service. Every command names the ledger
with `--file`.

Add a platform under a stable ID. The v2.1.0 contract requires both its name and
kind, so an ID alone reports those missing flags instead of inventing defaults:

```sh
forecast-ledger platform add \
  --file ledger.yaml \
  --platform metaculus \
  --name Metaculus \
  --kind scoring_platform \
  --url https://www.metaculus.com/ \
  --account-username example-user
```

The supported kinds are `scoring_platform`, `prediction_market`,
`self_hosted`, `internal`, and `informal`. URLs must be absolute. Account data
must contain at least one of username, user ID, or profile URL.

Update only named fields and use explicit clear flags for removal:

```sh
forecast-ledger platform update \
  --file ledger.yaml \
  --platform metaculus \
  --name "Metaculus forecasting" \
  --clear-url \
  --account-profile-url https://www.metaculus.com/accounts/profile/example-user/
```

Omitted fields stay unchanged. `--clear-url`, `--clear-account`, and the
account-field clear flags remove optional values. They cannot remove the
required name or kind. Public platform data is supplied only by these flags.

List and inspect records without mutation:

```sh
forecast-ledger platform list --file ledger.yaml
forecast-ledger platform show --file ledger.yaml --platform metaculus
```

Both commands accept a ledger on stdin with `--file -`. List order and
referencing question IDs are stable and sorted. JSON output includes the ledger
ID, exact platform record, and reference counts.

Removal is allowed only when no revision, forecast, or lifecycle provenance
references the platform and requires confirmation:

```sh
forecast-ledger platform remove \
  --file ledger.yaml \
  --platform old-platform \
  --yes
```

Without `--yes`, an interactive terminal asks first. Use `--dry-run` on add,
update, or remove to validate and show the planned change without writing.

[How-to index](index.md) · [Documentation index](../index.md)
