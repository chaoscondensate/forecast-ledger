# Generated operation contracts

<!-- doc-metadata
coverage: operation-contracts-v1
reviewed: 2026-08-31
owner: interface
generated: true
security-critical: true
prerequisites: index.md
next: ../index.md
source: go generate ./internal/service
-->

> Generated; do not edit by hand. Run `go generate ./internal/service`.

These declarations are shared request contracts for CLI reference and MCP discovery. A declaration does not make a hidden command available.

| Operation | CLI | MCP tool | Selection | Request contract | Outcomes | Dry-run | Confirmation | Network | Result notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `forecast.add` | `forecast-ledger forecast add` | `forecast_add` | `forecast` | [forecast-create](request-schemas/forecast-create.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `forecast.expire` | `forecast-ledger forecast expire` | `forecast_expire` | `forecast` | [lifecycle](request-schemas/lifecycle.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `forecast.key_hint.update` | `forecast-ledger forecast key-hint update` | `forecast_key_hint_update` | `forecast` | [key-hint-update](request-schemas/key-hint-update.schema.json) (direct) | `success`, `planned`, `unchanged` | true | false | `none` |  |
| `forecast.list` | `forecast-ledger forecast list` | `forecast_list` | `question` | — | `success` | false | false | `none` |  |
| `forecast.reaffirm` | `forecast-ledger forecast reaffirm` | `forecast_reaffirm` | `forecast` | [lifecycle](request-schemas/lifecycle.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `forecast.reveal` | `forecast-ledger forecast reveal` | `forecast_reveal` | `forecast` | — | `success`, `planned`, `unchanged` | true | true | `none` |  |
| `forecast.seal` | `forecast-ledger forecast seal` | `forecast_seal` | `forecast` | [forecast-seal-private](request-schemas/forecast-seal-private.schema.json) (secret) | `success`, `planned` | true | false | `none` |  |
| `forecast.show` | `forecast-ledger forecast show` | `forecast_show` | `forecast` | — | `success` | false | false | `none` |  |
| `forecast.withdraw` | `forecast-ledger forecast withdraw` | `forecast_withdraw` | `forecast` | [lifecycle](request-schemas/lifecycle.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `group.add` | `forecast-ledger group add` | `group_add` | `group` | [group-create](request-schemas/group-create.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `group.list` | `forecast-ledger group list` | `group_list` | `ledger` | — | `success` | false | false | `none` |  |
| `group.remove` | `forecast-ledger group remove` | `group_remove` | `group` | — | `success`, `planned` | true | true | `none` |  |
| `group.show` | `forecast-ledger group show` | `group_show` | `group` | — | `success` | false | false | `none` |  |
| `group.update` | `forecast-ledger group update` | `group_update` | `group` | [group-patch](request-schemas/group-patch.schema.json) (direct) | `success`, `planned`, `unchanged` | true | false | `none` |  |
| `ledger.init` | `forecast-ledger init` | `ledger_init` | `ledger` | [init](request-schemas/init.schema.json) (direct_with_initial_secret) | `success`, `planned` | true | false | `none` |  |
| `ledger.status` | `forecast-ledger status` | `ledger_status` | `ledger` | — | `success` | false | false | `none` |  |
| `ledger.update` | `forecast-ledger ledger update` | `ledger_update` | `ledger` | [root-metadata-patch](request-schemas/root-metadata-patch.schema.json) (direct) | `success`, `planned`, `unchanged` | true | false | `none` |  |
| `ledger.validate` | `forecast-ledger validate` | `ledger_validate` | `ledger` | — | `success` | false | false | `none` |  |
| `platform.add` | `forecast-ledger platform add` | `platform_add` | `platform` | [platform-create](request-schemas/platform-create.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `platform.list` | `forecast-ledger platform list` | `platform_list` | `ledger` | — | `success` | false | false | `none` |  |
| `platform.remove` | `forecast-ledger platform remove` | `platform_remove` | `platform` | — | `success`, `planned` | true | true | `none` |  |
| `platform.show` | `forecast-ledger platform show` | `platform_show` | `platform` | — | `success` | false | false | `none` |  |
| `platform.update` | `forecast-ledger platform update` | `platform_update` | `platform` | [platform-patch](request-schemas/platform-patch.schema.json) (direct) | `success`, `planned`, `unchanged` | true | false | `none` |  |
| `publication.build` | `forecast-ledger publish build` | `publication_build` | `ledger` | — | `success`, `planned` | true | false | `none` |  |
| `publication.verify` | `forecast-ledger publish verify` | `publication_verify` | `ledger` | — | `success`, `pending`, `partial_failure` | false | false | `none` | Pass requires at least one applicable forecast-evidence layer; an empty or all-not-applicable selection returns no_evidence. |
| `question.add` | `forecast-ledger question add` | `question_add` | `question` | [question-add](request-schemas/question-add.schema.json) (direct_with_initial_secret) | `success`, `planned` | true | false | `none` |  |
| `question.ambiguous` | `forecast-ledger question ambiguous` | `question_ambiguous` | `question` | [unresolved-resolution](request-schemas/unresolved-resolution.schema.json) (direct) | `success`, `planned` | true | true | `none` |  |
| `question.dispute` | `forecast-ledger question dispute` | `question_dispute` | `question` | [unresolved-resolution](request-schemas/unresolved-resolution.schema.json) (direct) | `success`, `planned` | true | true | `none` |  |
| `question.list` | `forecast-ledger question list` | `question_list` | `ledger` | — | `success` | false | false | `none` |  |
| `question.not_applicable` | `forecast-ledger question not-applicable` | `question_not_applicable` | `question` | [not-applicable](request-schemas/not-applicable.schema.json) (direct) | `success`, `planned` | true | true | `none` |  |
| `question.resolve` | `forecast-ledger question resolve` | `question_resolve` | `question` | [resolution](request-schemas/resolution.schema.json) (direct) | `success`, `planned` | true | true | `none` |  |
| `question.revise` | `forecast-ledger question revise` | `question_revise` | `question` | [question-revision](request-schemas/question-revision.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `question.show` | `forecast-ledger question show` | `question_show` | `question` | — | `success` | false | false | `none` |  |
| `question.update` | `forecast-ledger question update` | `question_update` | `question` | [question-patch](request-schemas/question-patch.schema.json) (direct) | `success`, `planned`, `unchanged` | true | false | `none` |  |
| `question.void` | `forecast-ledger question void` | `question_void` | `question` | [unresolved-resolution](request-schemas/unresolved-resolution.schema.json) (direct) | `success`, `planned` | true | true | `none` |  |
| `relationship.add` | `forecast-ledger relationship add` | `relationship_add` | `relationship` | [relationship](request-schemas/relationship.schema.json) (direct) | `success`, `planned` | true | false | `none` |  |
| `relationship.list` | `forecast-ledger relationship list` | `relationship_list` | `ledger` | — | `success` | false | false | `none` |  |
| `relationship.remove` | `forecast-ledger relationship remove` | `relationship_remove` | `relationship` | — | `success`, `planned` | true | true | `none` |  |
| `relationship.show` | `forecast-ledger relationship show` | `relationship_show` | `relationship` | — | `success` | false | false | `none` |  |
| `target.build` | `forecast-ledger target build` | `target_build` | `target` | — | `success`, `planned` | true | false | `none` |  |
| `target.check` | `forecast-ledger target check` | `target_check` | `target` | — | `success`, `partial_failure` | false | false | `none` |  |
| `timestamp.stamp` | `forecast-ledger timestamp stamp` | `timestamp_stamp` | `forecast` | — | `success`, `planned`, `pending`, `partial_failure` | true | false | `required` |  |
| `timestamp.status` | `forecast-ledger timestamp status` | `timestamp_status` | `forecast` | — | `success` | false | false | `none` |  |
| `timestamp.verify` | `forecast-ledger timestamp verify` | `timestamp_verify` | `forecast` | — | `success`, `planned`, `pending`, `partial_failure` | true | false | `none` | Returns a structured timing report for pending, not-checked, mismatch, and verified outcomes; source unavailability is not proof failure. |
| `verification.run` | `forecast-ledger verify` | `verification_run` | `ledger` | — | `success`, `pending`, `partial_failure` | false | false | `optional` | Pass requires at least one applicable forecast-evidence layer; an empty or all-not-applicable selection returns no_evidence. |

The common [operation result schema](result.schema.json) defines warning, side-effect, and recovery fields. The [MCP tool catalog](mcp-tool-schemas.json) contains closed request schemas.

[Reference index](../index.md)
