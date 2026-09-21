package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func BuildInitialPublicLedger(root *ledger.Ledger, input InitialQuestionInput) (*ledger.Ledger, error) {
	return BuildInitialPublicLedgerAt(root, input, root.CreatedAt)
}

func BuildInitialPublicLedgerAt(root *ledger.Ledger, input InitialQuestionInput, observedAt ledger.Timestamp) (*ledger.Ledger, error) {
	if root == nil {
		return nil, app.NewError(app.CodeInternal, "ledger root is nil", nil)
	}
	return BuildQuestionWithInitialPublicForecast(root, NormalizeInitialQuestion(input), observedAt)
}

func BuildInitialQuestionLedgerAt(root *ledger.Ledger, input InitialQuestionInput, observedAt ledger.Timestamp) (*ledger.Ledger, error) {
	if root == nil {
		return nil, app.NewError(app.CodeInternal, "ledger root is nil", nil)
	}
	question, _, err := buildQuestionShell(root, NormalizeInitialQuestion(input), observedAt)
	if err != nil {
		return nil, err
	}
	return appendProspectiveQuestion(root, question)
}

func BuildQuestionWithoutForecast(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionMutation, error) {
	question, _, err := buildQuestionShell(model, input, observedAt)
	if err != nil {
		return QuestionMutation{}, err
	}
	prospective, err := appendProspectiveQuestion(model, question)
	if err != nil {
		return QuestionMutation{}, err
	}
	value, err := jsonPatchValue(question)
	if err != nil {
		return QuestionMutation{}, err
	}
	return QuestionMutation{Ledger: prospective, Patches: []document.PatchOperation{{Kind: document.PatchAdd, Pointer: "/questions/-", Value: value}}}, nil
}

func BuildQuestionWithInitialPublicForecast(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (*ledger.Ledger, error) {
	question, index, err := buildQuestionShell(model, input, observedAt)
	if err != nil {
		return nil, err
	}
	if input.Input.InitialForecast == nil {
		return nil, invalidField("initial_forecast", "this builder requires an initial forecast")
	}
	forecast, err := buildInitialPublicForecast(*input.Input.InitialForecast, question.Revisions[0], observedAt)
	if err != nil {
		return nil, err
	}
	if input.Input.InitialForecast.SupersedesForecastID != nil {
		return nil, invalidField("initial_forecast.supersedes_forecast_id", "a question's first forecast cannot supersede another forecast")
	}
	if _, exists := index.Forecast(forecast.ID); exists {
		return nil, app.WithDetails(app.NewError(app.CodeConflict, "forecast ID already exists", nil), map[string]any{"forecast_id": forecast.ID})
	}
	question.Forecasts = []ledger.Forecast{forecast}
	return appendProspectiveQuestion(model, question)
}

func buildQuestionShell(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (ledger.Question, *ledger.Index, error) {
	if model == nil {
		return ledger.Question{}, nil, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	if err := ValidateSlug(input.ID, "question"); err != nil {
		return ledger.Question{}, nil, err
	}
	createdAt := observedAt
	if input.Input.CreatedAt != nil {
		createdAt = *input.Input.CreatedAt
	}
	if _, err := ParseTimestamp(createdAt, "question.created_at"); err != nil {
		return ledger.Question{}, nil, err
	}
	revision, err := buildRevision(input.Input.Revision, observedAt)
	if err != nil {
		return ledger.Question{}, nil, err
	}
	if strings.TrimSpace(revision.Title) == "" || strings.TrimSpace(revision.ResolutionCriteria) == "" {
		return ledger.Question{}, nil, invalidField("revision", "revision title and resolution criteria must not be empty")
	}
	index, err := ledger.BuildIndex(model)
	if err != nil {
		return ledger.Question{}, nil, app.NewError(app.CodeInvalidData, "existing ledger indexes are invalid", err)
	}
	if _, exists := index.Question(input.ID); exists {
		return ledger.Question{}, nil, app.WithDetails(app.NewError(app.CodeConflict, "question ID already exists", nil), map[string]any{"question_id": input.ID})
	}
	question := ledger.Question{
		ID: input.ID, Status: ledger.QuestionOpen, CreatedAt: createdAt,
		CurrentRevisionID: revision.ID, Revisions: []ledger.QuestionRevision{revision},
		Tags: cloneSlugs(input.Input.Tags), Notes: cloneString(input.Input.Notes), Forecasts: []ledger.Forecast{},
	}
	return question, index, nil
}

func buildRevision(input RevisionInput, observedAt ledger.Timestamp) (ledger.QuestionRevision, error) {
	if err := ValidateSlug(input.ID, "revision.id"); err != nil {
		return ledger.QuestionRevision{}, err
	}
	effective := input.EffectiveAt
	if effective == "" {
		effective = observedAt
	}
	recorded := observedAt
	if input.RecordedAt != nil {
		recorded = *input.RecordedAt
	}
	if err := ValidateChronology(effective, "revision.effective_at", recorded, "revision.recorded_at", true); err != nil {
		return ledger.QuestionRevision{}, err
	}
	if _, err := ParseTimestamp(input.ExpectedResolutionAt, "revision.expected_resolution_at"); err != nil {
		return ledger.QuestionRevision{}, err
	}
	return ledger.QuestionRevision{
		ID: input.ID, EffectiveAt: effective, RecordedAt: recorded, Title: input.Title,
		ResolutionCriteria: input.ResolutionCriteria, ForecastingOpensAt: input.ForecastingOpensAt,
		ExpectedResolutionAt: input.ExpectedResolutionAt, OutcomeSpace: input.OutcomeSpace,
		Domain: input.Domain, Provenance: input.Provenance,
	}, nil
}

func appendProspectiveQuestion(model *ledger.Ledger, question ledger.Question) (*ledger.Ledger, error) {
	prospective, err := cloneLedger(model)
	if err != nil {
		return nil, err
	}
	prospective.Questions = append(prospective.Questions, question)
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return nil, err
	}
	return prospective, nil
}

func buildInitialPublicForecast(input InitialForecastInput, revision ledger.QuestionRevision, observedAt ledger.Timestamp) (ledger.Forecast, error) {
	if input.Visibility != ledger.VisibilityPublic {
		return ledger.Forecast{}, invalidField("initial_forecast.visibility", "this builder requires a public initial forecast")
	}
	if err := ValidateSlug(input.ID, "initial_forecast.id"); err != nil {
		return ledger.Forecast{}, err
	}
	forecastedAt, recordedAt := DefaultForecastTimes(input.ForecastedAt, input.RecordedAt, observedAt)
	if err := validateForecastChronology(&revision, forecastedAt, recordedAt); err != nil {
		return ledger.Forecast{}, err
	}
	representations := append([]ledger.ForecastRepresentation(nil), input.Representations...)
	return ledger.Forecast{
		ID: input.ID, QuestionRevisionID: revision.ID, ForecastedAt: forecastedAt, RecordedAt: recordedAt,
		Visibility: ledger.VisibilityPublic, Representations: &representations,
		Rationale: cloneString(input.Rationale), KeyFactors: cloneStrings(input.KeyFactors), Comment: cloneString(input.Comment),
		PublicNote: cloneString(input.PublicNote), Provenance: input.Provenance,
		Integrity: ledger.Integrity{Unanchored: &ledger.UnanchoredIntegrity{Status: ledger.IntegrityUnanchored}},
	}, nil
}

func validateForecastChronology(revision *ledger.QuestionRevision, forecastedAt, recordedAt ledger.Timestamp) error {
	if err := ValidateChronology(forecastedAt, "forecasted_at", recordedAt, "recorded_at", true); err != nil {
		return err
	}
	if revision != nil && revision.ForecastingOpensAt != nil {
		return ValidateChronology(*revision.ForecastingOpensAt, "forecasting_opens_at", forecastedAt, "forecasted_at", true)
	}
	return nil
}

func ValidateProspectiveLedgerModel(model *ledger.Ledger) error {
	encoded, err := json.Marshal(model)
	if err != nil {
		return app.NewError(app.CodeInternal, "prospective ledger cannot be encoded", err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return app.NewError(app.CodeInvalidData, "prospective ledger cannot be parsed", err)
	}
	if err := ValidateLedgerDocument(parsed, nil); err != nil {
		return preserveValidationDetails(app.CodeInvalidData, "prospective ledger is not valid", err)
	}
	return nil
}

func cloneLedger(model *ledger.Ledger) (*ledger.Ledger, error) {
	encoded, err := json.Marshal(model)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "ledger cannot be copied", err)
	}
	var result ledger.Ledger
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, app.NewError(app.CodeInternal, "ledger copy cannot be decoded", err)
	}
	return &result, nil
}

func cloneSlugs(value *[]ledger.Slug) *[]ledger.Slug {
	if value == nil {
		return nil
	}
	copyValue := append([]ledger.Slug(nil), (*value)...)
	return &copyValue
}

func cloneStrings(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	copyValue := append([]string(nil), (*value)...)
	return &copyValue
}
