package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

type ForecastMutation struct {
	Ledger  *ledger.Ledger
	Patches []document.PatchOperation
}

type ForecastSummary struct {
	ID                   ledger.Slug                 `json:"id"`
	QuestionRevisionID   ledger.Slug                 `json:"question_revision_id"`
	ForecastedAt         ledger.Timestamp            `json:"forecasted_at"`
	RecordedAt           ledger.Timestamp            `json:"recorded_at"`
	Visibility           ledger.ForecastVisibility   `json:"visibility"`
	RepresentationKinds  []ledger.RepresentationKind `json:"representation_kinds,omitempty"`
	Active               bool                        `json:"active"`
	SupersedesForecastID *ledger.Slug                `json:"supersedes_forecast_id,omitempty"`
	IntegrityStatus      ledger.IntegrityStatus      `json:"integrity_status"`
}

type CommitmentView struct {
	Scheme              string            `json:"scheme"`
	CommitmentHash      ledger.Digest     `json:"commitment_hash"`
	Encryption          ledger.Encryption `json:"encryption"`
	KeyHint             string            `json:"key_hint"`
	RevealedAt          *ledger.Timestamp `json:"revealed_at,omitempty"`
	RevealedKeyRedacted bool              `json:"revealed_key_redacted,omitempty"`
}

type ForecastView struct {
	Summary         ForecastSummary                  `json:"summary"`
	Representations *[]ledger.ForecastRepresentation `json:"representations,omitempty"`
	Rationale       *string                          `json:"rationale,omitempty"`
	KeyFactors      *[]string                        `json:"key_factors,omitempty"`
	Comment         *string                          `json:"comment,omitempty"`
	PublicNote      *string                          `json:"public_note,omitempty"`
	Provenance      *ledger.Provenance               `json:"provenance,omitempty"`
	LifecycleEvents *[]ledger.LifecycleEvent         `json:"lifecycle_events,omitempty"`
	Commitment      *CommitmentView                  `json:"commitment,omitempty"`
	Integrity       ForecastIntegrityView            `json:"integrity"`
}

type ForecastIntegrityView struct {
	Status        ledger.IntegrityStatus    `json:"status"`
	Target        *ledger.ForecastTarget    `json:"target,omitempty"`
	Timestamps    []ledger.RFC3161Timestamp `json:"timestamps,omitempty"`
	VerifiedAt    *ledger.Timestamp         `json:"verified_at,omitempty"`
	FailureReason string                    `json:"failure_reason,omitempty"`
	StoredOnly    bool                      `json:"stored_only,omitempty"`
}

func BuildPublicForecastAppend(model *ledger.Ledger, questionID, forecastID ledger.Slug, input ForecastCreateInput, observedAt ledger.Timestamp) (ForecastMutation, error) {
	var result ForecastMutation
	forecastedAt, recordedAt := DefaultForecastTimes(input.ForecastedAt, input.RecordedAt, observedAt)
	questionPosition, question, revision, err := prepareForecastAppend(model, questionID, input.QuestionRevisionID, forecastID, forecastedAt, recordedAt, input.SupersedesForecastID)
	if err != nil {
		return result, err
	}
	if len(input.Representations) == 0 {
		return result, invalidField("representations", "at least one forecast representation is required")
	}
	if err := validateOptionalKeyFactors(input.KeyFactors); err != nil {
		return result, err
	}
	if err := validateForecastChronology(&revision, forecastedAt, recordedAt); err != nil {
		return result, err
	}
	representations := append([]ledger.ForecastRepresentation(nil), input.Representations...)
	forecast := ledger.Forecast{
		ID: forecastID, QuestionRevisionID: revision.ID, ForecastedAt: forecastedAt, RecordedAt: recordedAt,
		Visibility: ledger.VisibilityPublic, Representations: &representations,
		Rationale: cloneString(input.Rationale), KeyFactors: cloneStrings(input.KeyFactors), Comment: cloneString(input.Comment),
		PublicNote: cloneString(input.PublicNote), Provenance: input.Provenance,
		SupersedesForecastID: cloneSlug(input.SupersedesForecastID),
		Integrity:            ledger.Integrity{Unanchored: &ledger.UnanchoredIntegrity{Status: ledger.IntegrityUnanchored}},
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return result, err
	}
	prospective.Questions[questionPosition].Forecasts = append(prospective.Questions[questionPosition].Forecasts, forecast)
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return result, err
	}
	valuePatch, err := jsonPatchValue(forecast)
	if err != nil {
		return result, err
	}
	result.Ledger = prospective
	result.Patches = []document.PatchOperation{{Kind: document.PatchAdd, Pointer: questionForecastAppendPointer(questionPosition), Value: valuePatch}}
	_ = question
	return result, nil
}

func DefaultForecastTimes(forecastedAt ledger.Timestamp, explicitRecordedAt *ledger.Timestamp, observedAt ledger.Timestamp) (ledger.Timestamp, ledger.Timestamp) {
	if forecastedAt == "" {
		forecastedAt = observedAt
	}
	recordedAt := observedAt
	if explicitRecordedAt != nil {
		recordedAt = *explicitRecordedAt
	}
	return forecastedAt, recordedAt
}

func prepareForecastAppend(model *ledger.Ledger, questionID, revisionID, forecastID ledger.Slug, forecastedAt, recordedAt ledger.Timestamp, supersedes *ledger.Slug) (int, ledger.Question, ledger.QuestionRevision, error) {
	if model == nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	if err := ValidateSlug(questionID, "question"); err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, err
	}
	if err := ValidateSlug(revisionID, "question_revision_id"); err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, err
	}
	if err := ValidateSlug(forecastID, "forecast"); err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, err
	}
	index, err := ledger.BuildIndex(model)
	if err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, app.NewError(app.CodeInvalidData, "ledger indexes are invalid", err)
	}
	questionPosition, exists := index.Question(questionID)
	if !exists {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, app.WithDetails(app.NewError(app.CodeNotFound, "question was not found", nil), map[string]any{"question_id": questionID})
	}
	question := model.Questions[questionPosition]
	if question.Status != ledger.QuestionOpen {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, app.WithDetails(app.NewError(app.CodeConflict, "forecasts can be added only to an open question", nil), map[string]any{"question_id": questionID, "status": question.Status})
	}
	var revision *ledger.QuestionRevision
	for i := range question.Revisions {
		if question.Revisions[i].ID == revisionID {
			revision = &question.Revisions[i]
			break
		}
	}
	if revision == nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, invalidField("question_revision_id", "revision does not belong to the selected question")
	}
	if _, exists := index.Forecast(forecastID); exists {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, app.WithDetails(app.NewError(app.CodeConflict, "forecast ID already exists", nil), map[string]any{"forecast_id": forecastID})
	}
	if len(question.Forecasts) > 0 {
		last := question.Forecasts[len(question.Forecasts)-1]
		if err := ValidateChronology(last.RecordedAt, "previous.recorded_at", recordedAt, "recorded_at", true); err != nil {
			return 0, ledger.Question{}, ledger.QuestionRevision{}, invalidField("recorded_at", "forecast records must remain ordered by recorded time")
		}
	}
	if supersedes != nil {
		location, exists := index.Forecast(*supersedes)
		if !exists || location.QuestionID != questionID {
			return 0, ledger.Question{}, ledger.QuestionRevision{}, invalidField("supersedes_forecast_id", "supersedes must identify an earlier forecast in the same question")
		}
	}
	return questionPosition, question, *revision, nil
}

func ListForecasts(model *ledger.Ledger, questionID ledger.Slug) ([]ForecastSummary, error) {
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return nil, err
	}
	result := make([]ForecastSummary, len(question.Forecasts))
	for i, forecast := range question.Forecasts {
		result[i] = summarizeForecast(forecast)
	}
	return result, nil
}

func ShowForecast(model *ledger.Ledger, questionID, forecastID ledger.Slug) (ForecastView, error) {
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return ForecastView{}, err
	}
	var selected *ledger.Forecast
	for i := range question.Forecasts {
		if question.Forecasts[i].ID == forecastID {
			selected = &question.Forecasts[i]
			break
		}
	}
	if selected == nil {
		return ForecastView{}, app.WithDetails(app.NewError(app.CodeNotFound, "forecast was not found in the selected question", nil), map[string]any{"question_id": questionID, "forecast_id": forecastID})
	}
	view := ForecastView{Summary: summarizeForecast(*selected), PublicNote: cloneString(selected.PublicNote), Provenance: selected.Provenance, LifecycleEvents: selected.LifecycleEvents, Integrity: forecastIntegrityView(selected.Integrity)}
	if selected.Visibility != ledger.VisibilitySealed {
		view.Representations = cloneRepresentations(selected.Representations)
		view.Rationale, view.KeyFactors, view.Comment = cloneString(selected.Rationale), cloneStrings(selected.KeyFactors), cloneString(selected.Comment)
	}
	view.Commitment = commitmentView(selected.Commitment)
	if selected.Visibility == ledger.VisibilitySealed && view.Commitment != nil {
		view.Commitment.Encryption.Ciphertext, view.Commitment.Encryption.Nonce = "", ""
	}
	return view, nil
}

func forecastIntegrityView(value ledger.Integrity) ForecastIntegrityView {
	view := ForecastIntegrityView{Status: integrityStatus(value)}
	switch {
	case value.Pending != nil:
		target := value.Pending.Target
		view.Target = &target
		view.Timestamps = append([]ledger.RFC3161Timestamp(nil), value.Pending.Timestamps...)
	case value.Verified != nil:
		target, at := value.Verified.Target, value.Verified.VerifiedAt
		view.Target = &target
		view.Timestamps = append([]ledger.RFC3161Timestamp(nil), value.Verified.Timestamps...)
		view.VerifiedAt = &at
		view.StoredOnly = true
	case value.Failed != nil:
		view.FailureReason = value.Failed.FailureReason
		if value.Failed.Target != nil {
			target := *value.Failed.Target
			view.Target = &target
		}
		if value.Failed.Timestamps != nil {
			view.Timestamps = append([]ledger.RFC3161Timestamp(nil), (*value.Failed.Timestamps)...)
		}
	}
	return view
}

func selectQuestion(model *ledger.Ledger, id ledger.Slug) (int, ledger.Question, error) {
	if model == nil {
		return 0, ledger.Question{}, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	index, err := ledger.BuildIndex(model)
	if err != nil {
		return 0, ledger.Question{}, app.NewError(app.CodeInvalidData, "ledger indexes are invalid", err)
	}
	position, exists := index.Question(id)
	if !exists {
		return 0, ledger.Question{}, app.WithDetails(app.NewError(app.CodeNotFound, "question was not found", nil), map[string]any{"question_id": id})
	}
	return position, model.Questions[position], nil
}

func summarizeForecast(forecast ledger.Forecast) ForecastSummary {
	kinds := []ledger.RepresentationKind{}
	if forecast.Representations != nil {
		for _, representation := range *forecast.Representations {
			kinds = append(kinds, representationKind(representation))
		}
	}
	return ForecastSummary{ID: forecast.ID, QuestionRevisionID: forecast.QuestionRevisionID, ForecastedAt: forecast.ForecastedAt, RecordedAt: forecast.RecordedAt, Visibility: forecast.Visibility, RepresentationKinds: kinds, Active: forecastActive(forecast), SupersedesForecastID: cloneSlug(forecast.SupersedesForecastID), IntegrityStatus: integrityStatus(forecast.Integrity)}
}

func representationKind(value ledger.ForecastRepresentation) ledger.RepresentationKind {
	switch {
	case value.Probability != nil:
		return value.Probability.Kind
	case value.PMF != nil:
		return value.PMF.Kind
	case value.BinnedPMF != nil:
		return value.BinnedPMF.Kind
	case value.Quantiles != nil:
		return value.Quantiles.Kind
	case value.CDF != nil:
		return value.CDF.Kind
	case value.Point != nil:
		return value.Point.Kind
	case value.CredibleIntervals != nil:
		return value.CredibleIntervals.Kind
	}
	return ""
}

func forecastActive(value ledger.Forecast) bool {
	if value.LifecycleEvents == nil || len(*value.LifecycleEvents) == 0 {
		return true
	}
	last := (*value.LifecycleEvents)[len(*value.LifecycleEvents)-1]
	return last.Type == ledger.LifecycleReaffirmed
}

func integrityStatus(value ledger.Integrity) ledger.IntegrityStatus {
	switch {
	case value.Unanchored != nil:
		return value.Unanchored.Status
	case value.Pending != nil:
		return value.Pending.Status
	case value.Verified != nil:
		return value.Verified.Status
	case value.Failed != nil:
		return value.Failed.Status
	}
	return ""
}

func commitmentView(value *ledger.Commitment) *CommitmentView {
	if value == nil {
		return nil
	}
	if value.Sealed != nil {
		v := value.Sealed
		return &CommitmentView{Scheme: v.Scheme, CommitmentHash: v.CommitmentHash, Encryption: v.Encryption, KeyHint: v.KeyHint}
	}
	if value.Revealed != nil {
		v := value.Revealed
		at := v.RevealedAt
		return &CommitmentView{Scheme: v.Scheme, CommitmentHash: v.CommitmentHash, Encryption: v.Encryption, KeyHint: v.KeyHint, RevealedAt: &at, RevealedKeyRedacted: true}
	}
	return nil
}

func cloneRepresentations(value *[]ledger.ForecastRepresentation) *[]ledger.ForecastRepresentation {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	var result []ledger.ForecastRepresentation
	_ = json.Unmarshal(encoded, &result)
	return &result
}

func cloneSlug(value *ledger.Slug) *ledger.Slug {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func validateOptionalKeyFactors(value *[]string) error {
	if value == nil {
		return nil
	}
	for _, factor := range *value {
		if strings.TrimSpace(factor) == "" {
			return invalidField("key_factors", "key factors must not contain an empty item")
		}
	}
	return nil
}

func questionForecastAppendPointer(questionPosition int) string {
	return "/questions/" + strconv.Itoa(questionPosition) + "/forecasts/-"
}
