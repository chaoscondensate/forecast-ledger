package service

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/forecastcrypto"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

var keyHintPattern = regexp.MustCompile(`^([a-z][a-z0-9+.-]*):([A-Za-z0-9._~+-]+)$`)

func ValidateKeyHint(forecastID ledger.Slug, keyHint string) error {
	match := keyHintPattern.FindStringSubmatch(keyHint)
	if len(match) != 3 || match[1] == "file" || match[1] == "forecast-key" && match[2] != string(forecastID) {
		return invalidField("key_hint", "key hint must be a non-location scheme:opaque value; forecast-key must name the selected forecast")
	}
	return nil
}

type SealedForecastBuild struct {
	Mutation ForecastMutation
	KeyFile  []byte
}

func BuildSealedForecastAppend(ctx context.Context, model *ledger.Ledger, questionID, forecastID ledger.Slug, input SealedForecastInput, observedAt ledger.Timestamp, effects Effects) (SealedForecastBuild, error) {
	var result SealedForecastBuild
	if err := effects.Validate(); err != nil {
		return result, app.NewError(app.CodeInternal, "sealing effects are not configured", err)
	}
	position, question, revision, recordedAt, normalized, err := prepareSealedForecastAppend(model, questionID, forecastID, input, observedAt)
	if err != nil {
		return result, err
	}
	bundle := forecastcrypto.PrivateBundle{Representations: append([]ledger.ForecastRepresentation(nil), normalized.Representations...), Rationale: cloneString(normalized.Rationale), KeyFactors: cloneStrings(normalized.KeyFactors), Comment: cloneString(normalized.Comment)}
	sealed, err := forecastcrypto.Seal(ctx, questionID, revision.ID, forecastID, bundle, "forecast-key:"+string(forecastID), effects.Random)
	if err != nil {
		return result, app.NewError(app.CodeIO, "sealed forecast could not be created", err)
	}
	mutation, err := appendSealedForecastMutation(model, position, question, sealedAppendRecord(forecastID, normalized, revision.ID, recordedAt, sealed.Commitment))
	if err != nil {
		return result, err
	}
	result.Mutation, result.KeyFile = mutation, sealed.KeyFile
	return result, nil
}

func PlanSealedForecastAppend(model *ledger.Ledger, questionID, forecastID ledger.Slug, input SealedForecastInput, observedAt ledger.Timestamp) (ForecastMutation, error) {
	position, question, revision, recordedAt, normalized, err := prepareSealedForecastAppend(model, questionID, forecastID, input, observedAt)
	if err != nil {
		return ForecastMutation{}, err
	}
	return appendSealedForecastMutation(model, position, question, sealedAppendRecord(forecastID, normalized, revision.ID, recordedAt, placeholderCommitment(forecastID)))
}

func prepareSealedForecastAppend(model *ledger.Ledger, questionID, forecastID ledger.Slug, input SealedForecastInput, observedAt ledger.Timestamp) (int, ledger.Question, ledger.QuestionRevision, ledger.Timestamp, SealedForecastInput, error) {
	forecastedAt, recordedAt := DefaultForecastTimes(input.ForecastedAt, input.RecordedAt, observedAt)
	input.ForecastedAt, input.RecordedAt = forecastedAt, &recordedAt
	position, question, revision, err := prepareForecastAppend(model, questionID, input.QuestionRevisionID, forecastID, forecastedAt, recordedAt, input.SupersedesForecastID)
	if err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, "", input, err
	}
	if len(input.Representations) == 0 {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, "", input, invalidField("representations", "at least one forecast representation is required")
	}
	if err := validateOptionalKeyFactors(input.KeyFactors); err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, "", input, err
	}
	if err := validateForecastChronology(&revision, forecastedAt, recordedAt); err != nil {
		return 0, ledger.Question{}, ledger.QuestionRevision{}, "", input, err
	}
	return position, question, revision, recordedAt, input, nil
}

func sealedAppendRecord(id ledger.Slug, input SealedForecastInput, revisionID ledger.Slug, recordedAt ledger.Timestamp, commitment ledger.SealedCommitment) ledger.Forecast {
	return ledger.Forecast{ID: id, QuestionRevisionID: revisionID, ForecastedAt: input.ForecastedAt, RecordedAt: recordedAt, Visibility: ledger.VisibilitySealed, PublicNote: cloneString(input.PublicNote), Provenance: input.Provenance, SupersedesForecastID: cloneSlug(input.SupersedesForecastID), Commitment: &ledger.Commitment{Sealed: &commitment}, Integrity: ledger.Integrity{Unanchored: &ledger.UnanchoredIntegrity{Status: ledger.IntegrityUnanchored}}}
}

func appendSealedForecastMutation(model *ledger.Ledger, questionPosition int, _ ledger.Question, forecast ledger.Forecast) (ForecastMutation, error) {
	prospective, err := cloneLedger(model)
	if err != nil {
		return ForecastMutation{}, err
	}
	prospective.Questions[questionPosition].Forecasts = append(prospective.Questions[questionPosition].Forecasts, forecast)
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return ForecastMutation{}, err
	}
	value, err := jsonPatchValue(forecast)
	if err != nil {
		return ForecastMutation{}, err
	}
	return ForecastMutation{Ledger: prospective, Patches: []document.PatchOperation{{Kind: document.PatchAdd, Pointer: questionForecastAppendPointer(questionPosition), Value: value}}}, nil
}

func BuildForecastReveal(model *ledger.Ledger, questionID, forecastID ledger.Slug, keyFile []byte, revealedAt ledger.Timestamp) (ForecastMutation, error) {
	if _, err := ParseTimestamp(revealedAt, "revealed_at"); err != nil {
		return ForecastMutation{}, err
	}
	questionPosition, question, forecastPosition, forecast, err := selectForecast(model, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	sealed, alreadyRevealed, err := originalSealedCommitment(forecast)
	if err != nil {
		return ForecastMutation{}, err
	}
	opened, err := forecastcrypto.Open(keyFile, questionID, forecast.QuestionRevisionID, forecastID, sealed)
	if err != nil {
		return ForecastMutation{}, app.NewError(app.CodeVerification, "sealed forecast authentication failed", err)
	}
	if err := validateRevealedBundle(question, forecast, opened.Bundle); err != nil {
		return ForecastMutation{}, app.NewError(app.CodeVerification, "authenticated bundle does not match the selected forecast", err)
	}
	if alreadyRevealed {
		if forecast.Commitment.Revealed.RevealedKey != opened.KeyHex || !revealedMirrorMatches(forecast, opened.Bundle) {
			return ForecastMutation{}, app.NewError(app.CodeVerification, "revealed forecast mirror or disclosed key does not match the authenticated seal", nil)
		}
		return ForecastMutation{Ledger: model}, nil
	}
	originalTarget, err := BuildForecastTarget(model, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return ForecastMutation{}, err
	}
	updated := &prospective.Questions[questionPosition].Forecasts[forecastPosition]
	representations := append([]ledger.ForecastRepresentation(nil), opened.Bundle.Representations...)
	updated.Visibility, updated.Representations = ledger.VisibilityRevealed, &representations
	updated.Rationale, updated.KeyFactors, updated.Comment = cloneString(opened.Bundle.Rationale), cloneStrings(opened.Bundle.KeyFactors), cloneString(opened.Bundle.Comment)
	updated.Commitment = &ledger.Commitment{Revealed: &ledger.RevealedCommitment{Scheme: sealed.Scheme, CommitmentHash: sealed.CommitmentHash, Encryption: sealed.Encryption, KeyHint: sealed.KeyHint, RevealedAt: revealedAt, RevealedKey: opened.KeyHex}}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return ForecastMutation{}, err
	}
	revealedTarget, err := BuildForecastTarget(prospective, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	if !bytes.Equal(originalTarget.Bytes, revealedTarget.Bytes) {
		return ForecastMutation{}, app.NewError(app.CodeVerification, "reveal would change the original sealed forecast target", nil)
	}
	base := "/questions/" + strconv.Itoa(questionPosition) + "/forecasts/" + strconv.Itoa(forecastPosition)
	patchValue := func(value any) any { normalized, _ := jsonPatchValue(value); return normalized }
	patches := []document.PatchOperation{
		replacePatch(base+"/visibility", ledger.VisibilityRevealed),
		{Kind: document.PatchAdd, Pointer: base + "/representations", Value: patchValue(representations)},
	}
	if opened.Bundle.Rationale != nil {
		patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/rationale", Value: *opened.Bundle.Rationale})
	}
	if opened.Bundle.KeyFactors != nil {
		patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/key_factors", Value: patchValue(*opened.Bundle.KeyFactors)})
	}
	if opened.Bundle.Comment != nil {
		patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/comment", Value: *opened.Bundle.Comment})
	}
	patches = append(patches,
		document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/commitment/revealed_at", Value: string(revealedAt)},
		document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/commitment/revealed_key", Value: string(opened.KeyHex)},
	)
	return ForecastMutation{Ledger: prospective, Patches: patches}, nil
}

func BuildForecastKeyHintUpdate(model *ledger.Ledger, questionID, forecastID ledger.Slug, keyHint string) (ForecastMutation, error) {
	questionPosition, _, forecastPosition, forecast, err := selectForecast(model, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	if err := ValidateKeyHint(forecastID, keyHint); err != nil {
		return ForecastMutation{}, err
	}
	if forecast.Commitment == nil || forecast.Commitment.Sealed == nil && forecast.Commitment.Revealed == nil {
		return ForecastMutation{}, app.NewError(app.CodeConflict, "key hint can be changed only on a sealed or revealed forecast", nil)
	}
	current := ""
	if forecast.Commitment.Sealed != nil {
		current = forecast.Commitment.Sealed.KeyHint
	} else {
		current = forecast.Commitment.Revealed.KeyHint
	}
	if current == keyHint {
		return ForecastMutation{Ledger: model}, nil
	}
	before, err := BuildForecastTarget(model, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return ForecastMutation{}, err
	}
	updated := &prospective.Questions[questionPosition].Forecasts[forecastPosition]
	if updated.Commitment.Sealed != nil {
		updated.Commitment.Sealed.KeyHint = keyHint
	} else {
		updated.Commitment.Revealed.KeyHint = keyHint
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return ForecastMutation{}, err
	}
	after, err := BuildForecastTarget(prospective, questionID, forecastID)
	if err != nil || !bytes.Equal(before.Bytes, after.Bytes) {
		return ForecastMutation{}, app.NewError(app.CodeInternal, "key hint update changed forecast target bytes", err)
	}
	base := "/questions/" + strconv.Itoa(questionPosition) + "/forecasts/" + strconv.Itoa(forecastPosition)
	return ForecastMutation{Ledger: prospective, Patches: []document.PatchOperation{replacePatch(base+"/commitment/key_hint", keyHint)}}, nil
}

func selectForecast(model *ledger.Ledger, questionID, forecastID ledger.Slug) (int, ledger.Question, int, ledger.Forecast, error) {
	questionPosition, question, err := selectQuestion(model, questionID)
	if err != nil {
		return 0, ledger.Question{}, 0, ledger.Forecast{}, err
	}
	for i, forecast := range question.Forecasts {
		if forecast.ID == forecastID {
			return questionPosition, question, i, forecast, nil
		}
	}
	return 0, ledger.Question{}, 0, ledger.Forecast{}, app.WithDetails(app.NewError(app.CodeNotFound, "forecast was not found in the selected question", nil), map[string]any{"question_id": questionID, "forecast_id": forecastID})
}

func originalSealedCommitment(forecast ledger.Forecast) (ledger.SealedCommitment, bool, error) {
	if forecast.Commitment == nil {
		return ledger.SealedCommitment{}, false, app.NewError(app.CodeConflict, "forecast has no sealed commitment", nil)
	}
	if forecast.Visibility == ledger.VisibilitySealed && forecast.Commitment.Sealed != nil {
		return *forecast.Commitment.Sealed, false, nil
	}
	if forecast.Visibility == ledger.VisibilityRevealed && forecast.Commitment.Revealed != nil {
		v := forecast.Commitment.Revealed
		return ledger.SealedCommitment{Scheme: v.Scheme, CommitmentHash: v.CommitmentHash, Encryption: v.Encryption, KeyHint: v.KeyHint}, true, nil
	}
	return ledger.SealedCommitment{}, false, app.NewError(app.CodeConflict, "forecast is not sealed or revealed", nil)
}

func validateRevealedBundle(question ledger.Question, forecast ledger.Forecast, bundle forecastcrypto.PrivateBundle) error {
	var revision *ledger.QuestionRevision
	for i := range question.Revisions {
		if question.Revisions[i].ID == forecast.QuestionRevisionID {
			revision = &question.Revisions[i]
			break
		}
	}
	if revision == nil {
		return app.NewError(app.CodeVerification, "bound question revision is missing", nil)
	}
	if len(bundle.Representations) == 0 {
		return app.NewError(app.CodeVerification, "authenticated forecast has no representations", nil)
	}
	if err := validateOptionalKeyFactors(bundle.KeyFactors); err != nil {
		return err
	}
	return validateForecastChronology(revision, forecast.ForecastedAt, forecast.RecordedAt)
}

func revealedMirrorMatches(forecast ledger.Forecast, bundle forecastcrypto.PrivateBundle) bool {
	if forecast.Representations == nil {
		return false
	}
	left, _ := json.Marshal(forecastcrypto.PrivateBundle{Representations: *forecast.Representations, Rationale: forecast.Rationale, KeyFactors: forecast.KeyFactors, Comment: forecast.Comment})
	right, _ := json.Marshal(bundle)
	return reflect.DeepEqual(left, right)
}

var _ = strings.Builder{}
