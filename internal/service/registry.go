package service

import "sort"

type SelectionKind string

const (
	SelectionLedger           SelectionKind = "ledger"
	SelectionPlatform         SelectionKind = "platform"
	SelectionQuestion         SelectionKind = "question"
	SelectionForecast         SelectionKind = "forecast"
	SelectionQuestionForecast SelectionKind = "question_forecast"
	SelectionTarget           SelectionKind = "target"
	SelectionGroup            SelectionKind = "group"
	SelectionRelationship     SelectionKind = "relationship"
)

type RequestMode string

const (
	RequestNone                    RequestMode = "none"
	RequestDirect                  RequestMode = "direct"
	RequestDirectWithInitialSecret RequestMode = "direct_with_initial_secret"
	RequestSecret                  RequestMode = "secret"
)

type RequestField struct {
	Name        string   `json:"name"`
	Required    bool     `json:"required"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

type OperationDefinition struct {
	Name            OperationName   `json:"operation"`
	CLI             string          `json:"cli"`
	MCPTool         string          `json:"mcp_tool"`
	Selection       SelectionKind   `json:"selection"`
	RequestSchema   InputSchemaName `json:"request_schema,omitempty"`
	RequestMode     RequestMode     `json:"request_mode"`
	Fields          []RequestField  `json:"fields,omitempty"`
	Policy          OperationPolicy `json:"policy"`
	ResultNotes     string          `json:"result_notes,omitempty"`
	OutcomeStates   []OutcomeState  `json:"outcome_states"`
	classifyOutcome outcomeClassifier
}

func OperationDefinitions() []OperationDefinition {
	definitions := []OperationDefinition{
		definition(OperationLedgerInit, "init", "ledger_init", SelectionLedger, InputSchemaInit, RequestDirectWithInitialSecret,
			requiredString("ledger_id", "Stable ledger ID"), requiredString("timezone", "IANA timezone name"),
			requiredString("forecaster_id", "Stable forecaster ID"), requiredString("forecaster_name", "Forecaster display name"),
			RequestField{Name: "forecaster_kind", Type: "string", Description: "Forecaster kind", Enum: []string{"individual", "team"}},
			RequestField{Name: "initial_secret_input_file", Type: "string", Description: "Protected private bundle for a sealed first forecast"},
			RequestField{Name: "key_file", Type: "string", Description: "New protected key reference for a sealed first forecast"}),
		definition(OperationLedgerUpdate, "ledger update", "ledger_update", SelectionLedger, InputSchemaRootMetadata, RequestDirect),
		definition(OperationLedgerValidate, "validate", "ledger_validate", SelectionLedger, "", RequestNone),
		definition(OperationLedgerStatus, "status", "ledger_status", SelectionLedger, "", RequestNone),
		definition(OperationPlatformAdd, "platform add", "platform_add", SelectionPlatform, InputSchemaPlatformCreate, RequestDirect),
		definition(OperationPlatformUpdate, "platform update", "platform_update", SelectionPlatform, InputSchemaPlatformPatch, RequestDirect),
		definition(OperationPlatformList, "platform list", "platform_list", SelectionLedger, "", RequestNone),
		definition(OperationPlatformShow, "platform show", "platform_show", SelectionPlatform, "", RequestNone),
		definition(OperationPlatformRemove, "platform remove", "platform_remove", SelectionPlatform, "", RequestNone),
		definition(OperationQuestionAdd, "question add", "question_add", SelectionQuestion, InputSchemaQuestionAdd, RequestDirectWithInitialSecret,
			RequestField{Name: "initial_secret_input_file", Type: "string", Description: "Protected private bundle for a sealed first forecast"},
			RequestField{Name: "key_file", Type: "string", Description: "New protected key reference for a sealed first forecast"}),
		definition(OperationQuestionUpdate, "question update", "question_update", SelectionQuestion, InputSchemaQuestionPatch, RequestDirect),
		definition(OperationQuestionRevise, "question revise", "question_revise", SelectionQuestion, InputSchemaQuestionRevision, RequestDirect),
		definition(OperationQuestionList, "question list", "question_list", SelectionLedger, "", RequestNone),
		definition(OperationQuestionShow, "question show", "question_show", SelectionQuestion, "", RequestNone),
		definition(OperationQuestionResolve, "question resolve", "question_resolve", SelectionQuestion, InputSchemaResolution, RequestDirect),
		definition(OperationQuestionAmbiguous, "question ambiguous", "question_ambiguous", SelectionQuestion, InputSchemaUnresolvedResolution, RequestDirect),
		definition(OperationQuestionVoid, "question void", "question_void", SelectionQuestion, InputSchemaUnresolvedResolution, RequestDirect),
		definition(OperationQuestionDispute, "question dispute", "question_dispute", SelectionQuestion, InputSchemaUnresolvedResolution, RequestDirect),
		definition(OperationQuestionNotApplicable, "question not-applicable", "question_not_applicable", SelectionQuestion, InputSchemaNotApplicable, RequestDirect),
		definition(OperationGroupAdd, "group add", "group_add", SelectionGroup, InputSchemaGroupCreate, RequestDirect),
		definition(OperationGroupUpdate, "group update", "group_update", SelectionGroup, InputSchemaGroupPatch, RequestDirect),
		definition(OperationGroupList, "group list", "group_list", SelectionLedger, "", RequestNone),
		definition(OperationGroupShow, "group show", "group_show", SelectionGroup, "", RequestNone),
		definition(OperationGroupRemove, "group remove", "group_remove", SelectionGroup, "", RequestNone),
		definition(OperationRelationshipAdd, "relationship add", "relationship_add", SelectionRelationship, InputSchemaRelationship, RequestDirect),
		definition(OperationRelationshipList, "relationship list", "relationship_list", SelectionLedger, "", RequestNone),
		definition(OperationRelationshipShow, "relationship show", "relationship_show", SelectionRelationship, "", RequestNone),
		definition(OperationRelationshipRemove, "relationship remove", "relationship_remove", SelectionRelationship, "", RequestNone),
		definition(OperationForecastAdd, "forecast add", "forecast_add", SelectionForecast, InputSchemaForecastCreate, RequestDirect),
		definition(OperationForecastList, "forecast list", "forecast_list", SelectionQuestion, "", RequestNone),
		definition(OperationForecastShow, "forecast show", "forecast_show", SelectionForecast, "", RequestNone),
		definition(OperationForecastSeal, "forecast seal", "forecast_seal", SelectionForecast, InputSchemaForecastSealPrivate, RequestSecret,
			RequestField{Name: "secret_input_file", Required: true, Type: "string", Description: "Protected private forecast bundle"},
			requiredString("question_revision_id", "Bound question revision ID"),
			RequestField{Name: "forecasted_at", Type: "string", Description: "Optional exact RFC 3339 forecast time"},
			RequestField{Name: "recorded_at", Type: "string", Description: "Optional exact RFC 3339 record time"},
			RequestField{Name: "public_note", Type: "string", Description: "Optional public note"},
			RequestField{Name: "supersedes_forecast_id", Type: "string", Description: "Optional superseded forecast ID"},
			requiredString("key_file", "New protected key reference")),
		definition(OperationForecastReveal, "forecast reveal", "forecast_reveal", SelectionForecast, "", RequestNone,
			requiredString("key_file", "Protected key reference"),
			RequestField{Name: "revealed_at", Type: "string", Description: "Optional exact RFC 3339 reveal time"}),
		definition(OperationForecastWithdraw, "forecast withdraw", "forecast_withdraw", SelectionForecast, InputSchemaLifecycle, RequestDirect),
		definition(OperationForecastExpire, "forecast expire", "forecast_expire", SelectionForecast, InputSchemaLifecycle, RequestDirect),
		definition(OperationForecastReaffirm, "forecast reaffirm", "forecast_reaffirm", SelectionForecast, InputSchemaLifecycle, RequestDirect),
		definition(OperationForecastKeyHintUpdate, "forecast key-hint update", "forecast_key_hint_update", SelectionForecast, InputSchemaKeyHintUpdate, RequestDirect),
		definition(OperationTargetBuild, "target build", "target_build", SelectionTarget, "", RequestNone),
		definition(OperationTargetCheck, "target check", "target_check", SelectionTarget, "", RequestNone),
		definition(OperationTimestampStamp, "timestamp stamp", "timestamp_stamp", SelectionForecast, "", RequestNone,
			RequestField{Name: "tsa_provider", Type: "string", Description: "Built-in timestamp provider; defaults to auto", Enum: []string{"auto", "freetsa"}},
			RequestField{Name: "tsa_url", Type: "string", Description: "Custom public HTTPS timestamp authority URL; requires ca_bundle"},
			RequestField{Name: "ca_bundle", Type: "string", Description: "Custom retained ledger-relative PEM CA bundle; requires tsa_url"}),
		definition(OperationTimestampStatus, "timestamp status", "timestamp_status", SelectionForecast, "", RequestNone),
		definition(OperationTimestampVerify, "timestamp verify", "timestamp_verify", SelectionForecast, "", RequestNone),
		definition(OperationVerificationRun, "verify", "verification_run", SelectionLedger, "", RequestNone,
			RequestField{Name: "question", Type: "string", Description: "Optional question ID"},
			RequestField{Name: "forecast", Type: "string", Description: "Optional forecast ID; requires question"},
			RequestField{Name: "check_sources", Type: "boolean", Description: "Check outcome source reachability"}),
		definition(OperationPublicationBuild, "publish build", "publication_build", SelectionLedger, "", RequestNone,
			requiredString("output", "New package path inside an output root")),
		definition(OperationPublicationVerify, "publish verify", "publication_verify", SelectionLedger, "", RequestNone,
			requiredString("manifest", "Manifest path inside the package root")),
	}
	return definitions
}

func SortedOperationDefinitions() []OperationDefinition {
	definitions := OperationDefinitions()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions
}

func definition(name OperationName, cli, mcp string, selection SelectionKind, request InputSchemaName, mode RequestMode, fields ...RequestField) OperationDefinition {
	policy, ok := PolicyForOperation(name)
	if !ok {
		panic("operation definition has no policy: " + string(name))
	}
	outcomes := outcomeContractFor(name)
	if err := validateOutcomeContract(name, outcomes); err != nil {
		panic(err)
	}
	return OperationDefinition{
		Name: name, CLI: cli, MCPTool: mcp, Selection: selection,
		RequestSchema: request, RequestMode: mode, Fields: fields, Policy: policy, ResultNotes: operationResultNotes(name),
		OutcomeStates: append([]OutcomeState(nil), outcomes.States...), classifyOutcome: outcomes.Classify,
	}
}

func operationResultNotes(name OperationName) string {
	switch name {
	case OperationTimestampVerify:
		return "Returns a structured timing report for pending, not-checked, mismatch, and verified outcomes; source unavailability is not proof failure."
	case OperationVerificationRun, OperationPublicationVerify:
		return "Pass requires at least one applicable forecast-evidence layer; an empty or all-not-applicable selection returns no_evidence."
	default:
		return ""
	}
}

func requiredString(name, description string) RequestField {
	return RequestField{Name: name, Required: true, Type: "string", Description: description}
}
