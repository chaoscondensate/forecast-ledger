package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/forecastcrypto"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

type SealedInitialBuild struct {
	Ledger  *ledger.Ledger
	KeyFile []byte
}
type SealedQuestionBuild struct {
	Mutation QuestionMutation
	KeyFile  []byte
}

func BuildInitialSealedLedger(ctx context.Context, root *ledger.Ledger, input InitialQuestionInput, effects Effects) (SealedInitialBuild, error) {
	if root == nil {
		return SealedInitialBuild{}, app.NewError(app.CodeInternal, "ledger root is nil", nil)
	}
	return BuildInitialSealedLedgerAt(ctx, root, input, root.CreatedAt, effects)
}

func BuildInitialSealedLedgerAt(ctx context.Context, root *ledger.Ledger, input InitialQuestionInput, observedAt ledger.Timestamp, effects Effects) (SealedInitialBuild, error) {
	var result SealedInitialBuild
	if err := effects.Validate(); err != nil {
		return result, app.NewError(app.CodeInternal, "sealing effects are not configured", err)
	}
	question, private, recordedAt, err := prepareInitialSealedLedger(root, input, observedAt)
	if err != nil {
		return result, err
	}
	revisionID := question.CurrentRevisionID
	sealed, err := forecastcrypto.Seal(ctx, question.ID, revisionID, private.ID, privateBundle(private, revisionID, recordedAt), "forecast-key:"+string(private.ID), effects.Random)
	if err != nil {
		return result, app.NewError(app.CodeIO, "sealed forecast could not be created", err)
	}
	question.Forecasts = []ledger.Forecast{sealedForecastRecord(private, revisionID, recordedAt, sealed.Commitment)}
	prospective, err := appendProspectiveQuestion(root, question)
	if err != nil {
		return result, err
	}
	result.Ledger, result.KeyFile = prospective, sealed.KeyFile
	return result, nil
}

func PlanInitialSealedLedger(root *ledger.Ledger, input InitialQuestionInput) (*ledger.Ledger, error) {
	if root == nil {
		return nil, app.NewError(app.CodeInternal, "ledger root is nil", nil)
	}
	return PlanInitialSealedLedgerAt(root, input, root.CreatedAt)
}

func PlanInitialSealedLedgerAt(root *ledger.Ledger, input InitialQuestionInput, observedAt ledger.Timestamp) (*ledger.Ledger, error) {
	question, private, recordedAt, err := prepareInitialSealedLedger(root, input, observedAt)
	if err != nil {
		return nil, err
	}
	question.Forecasts = []ledger.Forecast{sealedForecastRecord(private, question.CurrentRevisionID, recordedAt, placeholderCommitment(private.ID))}
	return appendProspectiveQuestion(root, question)
}

func BuildQuestionAddSealed(ctx context.Context, model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp, effects Effects) (SealedQuestionBuild, error) {
	var result SealedQuestionBuild
	if err := effects.Validate(); err != nil {
		return result, app.NewError(app.CodeInternal, "sealing effects are not configured", err)
	}
	question, private, recordedAt, err := prepareQuestionAddSealed(model, input, observedAt)
	if err != nil {
		return result, err
	}
	sealed, err := forecastcrypto.Seal(ctx, question.ID, question.CurrentRevisionID, private.ID, privateBundle(private, question.CurrentRevisionID, recordedAt), "forecast-key:"+string(private.ID), effects.Random)
	if err != nil {
		return result, app.NewError(app.CodeIO, "sealed forecast could not be created", err)
	}
	question.Forecasts = []ledger.Forecast{sealedForecastRecord(private, question.CurrentRevisionID, recordedAt, sealed.Commitment)}
	prospective, err := appendProspectiveQuestion(model, question)
	if err != nil {
		return result, err
	}
	value, err := jsonPatchValue(question)
	if err != nil {
		return result, err
	}
	result.Mutation = QuestionMutation{Ledger: prospective, Patches: []document.PatchOperation{{Kind: document.PatchAdd, Pointer: "/questions/-", Value: value}}}
	result.KeyFile = sealed.KeyFile
	return result, nil
}

func PlanQuestionAddSealed(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionMutation, error) {
	question, private, recordedAt, err := prepareQuestionAddSealed(model, input, observedAt)
	if err != nil {
		return QuestionMutation{}, err
	}
	question.Forecasts = []ledger.Forecast{sealedForecastRecord(private, question.CurrentRevisionID, recordedAt, placeholderCommitment(private.ID))}
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

func prepareQuestionAddSealed(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (ledger.Question, InitialForecastInput, ledger.Timestamp, error) {
	question, index, err := buildQuestionShell(model, input, observedAt)
	if err != nil {
		return ledger.Question{}, InitialForecastInput{}, "", err
	}
	if input.Input.InitialForecast == nil {
		return ledger.Question{}, InitialForecastInput{}, "", invalidField("initial_forecast", "this builder requires an initial forecast")
	}
	return validateInitialSealed(question, index, *input.Input.InitialForecast, observedAt)
}

func prepareInitialSealedLedger(root *ledger.Ledger, input InitialQuestionInput, observedAt ledger.Timestamp) (ledger.Question, InitialForecastInput, ledger.Timestamp, error) {
	if root == nil {
		return ledger.Question{}, InitialForecastInput{}, "", app.NewError(app.CodeInternal, "ledger root is nil", nil)
	}
	question, index, err := buildQuestionShell(root, NormalizeInitialQuestion(input), observedAt)
	if err != nil {
		return ledger.Question{}, InitialForecastInput{}, "", err
	}
	if input.InitialForecast == nil {
		return ledger.Question{}, InitialForecastInput{}, "", invalidField("initial_forecast", "this builder requires an initial forecast")
	}
	return validateInitialSealed(question, index, *input.InitialForecast, observedAt)
}

func validateInitialSealed(question ledger.Question, index *ledger.Index, private InitialForecastInput, observedAt ledger.Timestamp) (ledger.Question, InitialForecastInput, ledger.Timestamp, error) {
	if private.Visibility != ledger.VisibilitySealed {
		return ledger.Question{}, InitialForecastInput{}, "", invalidField("initial_forecast.visibility", "this builder requires a sealed initial forecast")
	}
	if err := ValidateSlug(private.ID, "initial_forecast.id"); err != nil {
		return ledger.Question{}, InitialForecastInput{}, "", err
	}
	if _, exists := index.Forecast(private.ID); exists {
		return ledger.Question{}, InitialForecastInput{}, "", app.NewError(app.CodeConflict, "forecast ID already exists", nil)
	}
	if private.SupersedesForecastID != nil {
		return ledger.Question{}, InitialForecastInput{}, "", invalidField("initial_forecast.supersedes_forecast_id", "a question's first forecast cannot supersede another forecast")
	}
	if len(private.Representations) == 0 || private.Rationale == nil || private.KeyFactors == nil || private.Comment == nil {
		return ledger.Question{}, InitialForecastInput{}, "", invalidField("initial_forecast", "a sealed forecast requires representations, rationale, key_factors, and comment")
	}
	for i, factor := range *private.KeyFactors {
		if strings.TrimSpace(factor) == "" {
			return ledger.Question{}, InitialForecastInput{}, "", invalidField(fmt.Sprintf("initial_forecast.key_factors.%d", i), "key factor must not be empty")
		}
	}
	forecastedAt, recordedAt := DefaultForecastTimes(private.ForecastedAt, private.RecordedAt, observedAt)
	private.ForecastedAt = forecastedAt
	if err := validateForecastChronology(&question.Revisions[0], forecastedAt, recordedAt); err != nil {
		return ledger.Question{}, InitialForecastInput{}, "", err
	}
	return question, private, recordedAt, nil
}

func privateBundle(input InitialForecastInput, revisionID ledger.Slug, recordedAt ledger.Timestamp) forecastcrypto.PrivateBundle {
	return forecastcrypto.PrivateBundle{QuestionRevisionID: revisionID, ForecastedAt: input.ForecastedAt, RecordedAt: recordedAt, Representations: append([]ledger.ForecastRepresentation(nil), input.Representations...), Rationale: *input.Rationale, KeyFactors: append([]string(nil), (*input.KeyFactors)...), Comment: *input.Comment}
}

func placeholderCommitment(id ledger.Slug) ledger.SealedCommitment {
	return ledger.SealedCommitment{Scheme: forecastcrypto.SealScheme, CommitmentHash: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(strings.Repeat("0", 64))}, Encryption: ledger.Encryption{Algorithm: forecastcrypto.EncryptionProfile, Nonce: "AAAAAAAAAAAAAAAA", Ciphertext: "AAAAAAAAAAAAAAAAAAAAAAAA"}, KeyHint: "forecast-key:" + string(id)}
}

func sealedForecastRecord(private InitialForecastInput, revisionID ledger.Slug, recordedAt ledger.Timestamp, commitment ledger.SealedCommitment) ledger.Forecast {
	return ledger.Forecast{ID: private.ID, QuestionRevisionID: revisionID, ForecastedAt: private.ForecastedAt, RecordedAt: recordedAt, Visibility: ledger.VisibilitySealed, PublicNote: cloneString(private.PublicNote), Provenance: private.Provenance, Commitment: &ledger.Commitment{Sealed: &commitment}, Integrity: ledger.Integrity{Unanchored: &ledger.UnanchoredIntegrity{Status: ledger.IntegrityUnanchored}}}
}
