package service

import (
	"fmt"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

// OutcomeState is the closed transport-neutral state of a completed operation.
// Fatal failures that produced no safe result remain ordinary application
// errors and are not classified as outcomes.
type OutcomeState string

const (
	OutcomeSuccess        OutcomeState = "success"
	OutcomePlanned        OutcomeState = "planned"
	OutcomeUnchanged      OutcomeState = "unchanged"
	OutcomePending        OutcomeState = "pending"
	OutcomePartialFailure OutcomeState = "partial_failure"
)

// OutcomeInput contains only the facts a service classifier may use. Adapters
// provide the same dry-run bit, typed data, and service error.
type OutcomeInput struct {
	DryRun bool
	Data   any
	Err    error
}

// Outcome is the shared public result decision used by every adapter.
type Outcome struct {
	State       OutcomeState
	Code        string
	Message     string
	FailureCode app.ErrorCode
	HasData     bool
}

type outcomeClassifier func(OutcomeInput) Outcome

type outcomeContract struct {
	States   []OutcomeState
	Classify outcomeClassifier
}

type ordinaryOutcome struct {
	code           string
	message        string
	plannedCode    string
	plannedMessage string
	unchangedCode  string
	unchangedMsg   string
}

// ClassifyOutcome returns the single service-owned public code and message for
// an operation. It fails closed if the registry is incomplete.
func (definition OperationDefinition) ClassifyOutcome(input OutcomeInput) Outcome {
	if definition.classifyOutcome == nil {
		panic("operation definition has no outcome classifier: " + string(definition.Name))
	}
	return definition.classifyOutcome(input)
}

// ClassifyOperationOutcome is the adapter entry point when an operation
// definition is not already in hand.
func ClassifyOperationOutcome(name OperationName, input OutcomeInput) Outcome {
	contract := outcomeContractFor(name)
	if err := validateOutcomeContract(name, contract); err != nil {
		panic(err)
	}
	return contract.Classify(input)
}

func outcomeContractFor(name OperationName) outcomeContract {
	if special := specialOutcomeContract(name); special.Classify != nil {
		return special
	}
	spec, ok := ordinaryOutcomes[name]
	if !ok {
		return outcomeContract{}
	}
	states := []OutcomeState{OutcomeSuccess}
	if spec.plannedCode != "" {
		states = append(states, OutcomePlanned)
	}
	if spec.unchangedCode != "" {
		states = append(states, OutcomeUnchanged)
	}
	return outcomeContract{States: states, Classify: func(input OutcomeInput) Outcome {
		if input.Err != nil {
			return Outcome{FailureCode: app.ErrorCodeOf(input.Err)}
		}
		if input.DryRun && spec.plannedCode != "" {
			return Outcome{State: OutcomePlanned, Code: spec.plannedCode, Message: spec.plannedMessage, HasData: true}
		}
		if spec.unchangedCode != "" && resultUnchanged(input.Data) {
			return Outcome{State: OutcomeUnchanged, Code: spec.unchangedCode, Message: spec.unchangedMsg, HasData: true}
		}
		return Outcome{State: OutcomeSuccess, Code: spec.code, Message: spec.message, HasData: true}
	}}
}

func resultUnchanged(data any) bool {
	switch value := data.(type) {
	case RootMetadataFileResult:
		return !value.Changed
	case PlatformFileResult:
		return !value.Changed
	case QuestionFileResult:
		return !value.Changed
	case ForecastFileResult:
		return !value.Changed
	case CollectionFileResult:
		return !value.Changed
	default:
		return false
	}
}

func specialOutcomeContract(name OperationName) outcomeContract {
	switch name {
	case OperationTargetCheck:
		return outcomeContract{States: []OutcomeState{OutcomeSuccess, OutcomePartialFailure}, Classify: classifyTargetCheckOutcome}
	case OperationTimestampStamp:
		return outcomeContract{States: []OutcomeState{OutcomeSuccess, OutcomePlanned, OutcomePending, OutcomePartialFailure}, Classify: classifyTimestampStampOutcome}
	case OperationTimestampVerify:
		return outcomeContract{States: []OutcomeState{OutcomeSuccess, OutcomePlanned, OutcomePending, OutcomePartialFailure}, Classify: classifyTimestampVerifyOutcome}
	case OperationVerificationRun:
		return outcomeContract{States: []OutcomeState{OutcomeSuccess, OutcomePending, OutcomePartialFailure}, Classify: classifyVerificationOutcome}
	case OperationPublicationVerify:
		return outcomeContract{States: []OutcomeState{OutcomeSuccess, OutcomePending, OutcomePartialFailure}, Classify: classifyPublicationVerifyOutcome}
	default:
		return outcomeContract{}
	}
}

func classifyTargetCheckOutcome(input OutcomeInput) Outcome {
	result, ok := input.Data.(TargetOperationResult)
	if !ok || (input.Err != nil && result.FailureCode == "") {
		return fatalOutcome(input.Err)
	}
	if result.FailureCode != "" {
		return Outcome{State: OutcomePartialFailure, Code: "target.failed", Message: "Target inspection completed with failures", FailureCode: result.FailureCode, HasData: true}
	}
	for _, target := range result.Targets {
		if string(target.State) == string(LayerNotApplicable) {
			return Outcome{State: OutcomeSuccess, Code: "target.checked", Message: "Target inspection completed; some forecasts have no retained target", HasData: true}
		}
	}
	return Outcome{State: OutcomeSuccess, Code: "target.valid", Message: "Forecast target artifacts match the ledger", HasData: true}
}

func classifyTimestampStampOutcome(input OutcomeInput) Outcome {
	result, ok := input.Data.(TimestampArtifactResult)
	if !ok || (input.Err != nil && result.FailureCode == "") {
		return fatalOutcome(input.Err)
	}
	if input.DryRun {
		return Outcome{State: OutcomePlanned, Code: "timestamp.stamp.planned", Message: "Timestamp stamp is valid; no entropy, network request, or file write occurred", HasData: true}
	}
	switch result.FailureCode {
	case app.CodeNetwork:
		return Outcome{State: OutcomePartialFailure, Code: "timestamp.not_checked", Message: "The timestamp authority request did not complete", FailureCode: result.FailureCode, HasData: true}
	case app.CodeVerification:
		return Outcome{State: OutcomePartialFailure, Code: "timestamp.invalid_response", Message: "No timestamp authority response passed local verification", FailureCode: result.FailureCode, HasData: true}
	}
	if result.State == TimestampPending || result.FailureCode == app.CodePending {
		return Outcome{State: OutcomePending, Code: "timestamp.pending", Message: "RFC 3161 response was retained as pending", FailureCode: result.FailureCode, HasData: true}
	}
	return Outcome{State: OutcomeSuccess, Code: "timestamp.verified", Message: "RFC 3161 response was verified and retained", HasData: true}
}

func classifyTimestampVerifyOutcome(input OutcomeInput) Outcome {
	result, ok := input.Data.(TimestampVerifyResult)
	if !ok || (input.Err != nil && result.FailureCode == "") {
		return fatalOutcome(input.Err)
	}
	if input.DryRun {
		return Outcome{State: OutcomePlanned, Code: "timestamp.verify.planned", Message: "Timestamp verification is valid; the ledger update was deferred", HasData: true}
	}
	if result.Verification.State == LayerPass {
		return Outcome{State: OutcomeSuccess, Code: "timestamp.verified", Message: "RFC 3161 evidence was verified locally", HasData: true}
	}
	state := OutcomePartialFailure
	if result.Verification.State == LayerPending || result.FailureCode == app.CodePending {
		state = OutcomePending
	}
	return Outcome{State: state, Code: "timestamp.verification." + string(result.Verification.State), Message: "Timestamp verification completed with status " + string(result.Verification.State), FailureCode: result.FailureCode, HasData: true}
}

func classifyVerificationOutcome(input OutcomeInput) Outcome {
	result, ok := input.Data.(VerificationReport)
	if !ok || input.Err != nil {
		return fatalOutcome(input.Err)
	}
	state := OutcomeSuccess
	if result.Overall == VerificationPending {
		state = OutcomePending
	} else if result.FailureCode != "" {
		state = OutcomePartialFailure
	}
	return Outcome{State: state, Code: "verification." + string(result.Overall), Message: "Verification completed with status " + string(result.Overall), FailureCode: result.FailureCode, HasData: true}
}

func classifyPublicationVerifyOutcome(input OutcomeInput) Outcome {
	result, ok := input.Data.(PublicationVerifyResult)
	if !ok || input.Err != nil {
		return fatalOutcome(input.Err)
	}
	state := OutcomeSuccess
	if result.Overall == VerificationPending {
		state = OutcomePending
	} else if result.FailureCode != "" {
		state = OutcomePartialFailure
	}
	return Outcome{State: state, Code: "publication.verification." + string(result.Overall), Message: "Package verification completed with status " + string(result.Overall), FailureCode: result.FailureCode, HasData: true}
}

func fatalOutcome(err error) Outcome {
	if err == nil {
		err = app.NewError(app.CodeInternal, "operation returned an unexpected result type", nil)
	}
	return Outcome{FailureCode: app.ErrorCodeOf(err)}
}

var ordinaryOutcomes = map[OperationName]ordinaryOutcome{
	OperationLedgerInit:            {"ledger.initialized", "Ledger was created", "ledger.init.planned", "Ledger initialization is valid; no files were written", "", ""},
	OperationLedgerUpdate:          {"ledger.updated", "Ledger metadata was updated", "ledger.update.planned", "Ledger metadata update is valid; no file was changed", "ledger.unchanged", "Ledger metadata is already up to date"},
	OperationLedgerValidate:        {code: "ledger.valid", message: "Ledger is valid"},
	OperationLedgerStatus:          {code: "ledger.status", message: "Ledger status was read"},
	OperationPlatformAdd:           {"platform.added", "Platform was added", "platform.add.planned", "Platform addition is valid; no file was changed", "", ""},
	OperationPlatformUpdate:        {"platform.updated", "Platform was updated", "platform.update.planned", "Platform update is valid; no file was changed", "platform.unchanged", "Platform is already up to date"},
	OperationPlatformList:          {code: "platform.list", message: "Platforms were read"},
	OperationPlatformShow:          {code: "platform.show", message: "Platform was read"},
	OperationPlatformRemove:        {"platform.removed", "Platform was removed", "platform.remove.planned", "Platform removal is valid; no file was changed", "", ""},
	OperationQuestionAdd:           {"question.added", "Question was added", "question.add.planned", "Question addition is valid; no file was changed", "", ""},
	OperationQuestionUpdate:        {"question.updated", "Question was updated", "question.update.planned", "Question update is valid; no file was changed", "question.unchanged", "Question is already up to date"},
	OperationQuestionRevise:        {"question.revised", "Question revision was appended", "question.revise.planned", "Question revision is valid; no file was changed", "", ""},
	OperationQuestionList:          {code: "question.list", message: "Questions were read"},
	OperationQuestionShow:          {code: "question.show", message: "Question was read"},
	OperationQuestionResolve:       {"question.resolved", "Question was resolved", "question.resolve.planned", "Question lifecycle change is valid; no file was changed", "", ""},
	OperationQuestionAmbiguous:     {"question.ambiguous", "Question was marked ambiguous", "question.ambiguous.planned", "Question resolution is valid; no file was changed", "", ""},
	OperationQuestionVoid:          {"question.void", "Question was marked void", "question.void.planned", "Question resolution is valid; no file was changed", "", ""},
	OperationQuestionDispute:       {"question.disputed", "Question was disputed", "question.dispute.planned", "Question lifecycle change is valid; no file was changed", "", ""},
	OperationQuestionNotApplicable: {"question.not_applicable", "Question was marked not applicable", "question.not_applicable.planned", "Question resolution is valid; no file was changed", "", ""},
	OperationGroupAdd:              {"group.added", "Group was added", "group.add.planned", "Group addition is valid; no file was changed", "", ""},
	OperationGroupUpdate:           {"group.updated", "Group was updated", "group.update.planned", "Group update is valid; no file was changed", "group.unchanged", "Group is already up to date"},
	OperationGroupList:             {code: "group.list", message: "Groups were read"},
	OperationGroupShow:             {code: "group.show", message: "Group was read"},
	OperationGroupRemove:           {"group.removed", "Group was removed", "group.remove.planned", "Group removal is valid; no file was changed", "", ""},
	OperationRelationshipAdd:       {"relationship.added", "Relationship was added", "relationship.add.planned", "Relationship addition is valid; no file was changed", "", ""},
	OperationRelationshipList:      {code: "relationship.list", message: "Relationships were read"},
	OperationRelationshipShow:      {code: "relationship.show", message: "Relationship was read"},
	OperationRelationshipRemove:    {"relationship.removed", "Relationship was removed", "relationship.remove.planned", "Relationship removal is valid; no file was changed", "", ""},
	OperationForecastAdd:           {"forecast.added", "Forecast was added", "forecast.add.planned", "Forecast addition is valid; no file was changed", "", ""},
	OperationForecastList:          {code: "forecast.list", message: "Forecasts were read"},
	OperationForecastShow:          {code: "forecast.show", message: "Forecast was read"},
	OperationForecastSeal:          {"forecast.sealed", "Sealed forecast and protected key were created", "forecast.seal.planned", "Sealed forecast creation is valid; no key or ledger file was changed", "", ""},
	OperationForecastReveal:        {"forecast.revealed", "Forecast was authenticated and revealed", "forecast.reveal.planned", "Forecast reveal is valid; no file was changed", "forecast.reveal.unchanged", "Forecast was already revealed with this authenticated key"},
	OperationForecastWithdraw:      {"forecast.withdrawn", "Forecast was withdrawn", "forecast.withdraw.planned", "Forecast withdrawal is valid; no file was changed", "", ""},
	OperationForecastExpire:        {"forecast.expired", "Forecast was expired", "forecast.expire.planned", "Forecast expiration is valid; no file was changed", "", ""},
	OperationForecastReaffirm:      {"forecast.reaffirmed", "Forecast was reaffirmed", "forecast.reaffirm.planned", "Forecast reaffirmation is valid; no file was changed", "", ""},
	OperationForecastKeyHintUpdate: {"forecast.key_hint.updated", "Forecast key hint was updated", "forecast.key_hint.update.planned", "Key hint update is valid; no file was changed", "forecast.key_hint.unchanged", "Forecast key hint is already up to date"},
	OperationTargetBuild:           {"target.built", "Forecast target artifacts were built", "target.build.planned", "Target build is valid; no files were written", "", ""},
	OperationTimestampStatus:       {code: "timestamp.status", message: "RFC 3161 local status was read"},
	OperationPublicationBuild:      {"publication.built", "Evidence package was built", "publication.build.planned", "Evidence package build is valid; no files were written", "", ""},
}

func validateOutcomeContract(name OperationName, contract outcomeContract) error {
	if contract.Classify == nil || len(contract.States) == 0 {
		return fmt.Errorf("operation %s has no outcome contract", name)
	}
	return nil
}
