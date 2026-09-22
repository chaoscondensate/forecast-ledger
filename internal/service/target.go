package service

import (
	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	targetbytes "github.com/chaoscondensate/forecast-ledger/internal/target"
)

const (
	ForecastEnvelopeSchema = targetbytes.ForecastEnvelopeSchema
	LifecycleTargetSchema  = targetbytes.LifecycleSchema
	TargetCanonicalization = targetbytes.Canonicalization
)

type TargetArtifact struct {
	QuestionID   ledger.Slug         `json:"question_id"`
	ForecastID   ledger.Slug         `json:"forecast_id"`
	HeadEventID  ledger.Slug         `json:"head_event_id,omitempty"`
	Scope        string              `json:"scope"`
	RelativePath ledger.RelativePath `json:"path"`
	SHA256       string              `json:"sha256"`
	Size         int                 `json:"size"`
	Bytes        []byte              `json:"-"`
}

func BuildForecastTarget(model *ledger.Ledger, questionID, forecastID ledger.Slug) (TargetArtifact, error) {
	if model == nil {
		return TargetArtifact{}, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return TargetArtifact{}, err
	}
	var forecast *ledger.Forecast
	for i := range question.Forecasts {
		if question.Forecasts[i].ID == forecastID {
			forecast = &question.Forecasts[i]
			break
		}
	}
	if forecast == nil {
		return TargetArtifact{}, app.WithDetails(app.NewError(app.CodeNotFound, "forecast was not found in the selected question", nil), map[string]any{"question_id": questionID, "forecast_id": forecastID})
	}
	canonicalBytes, digest, err := targetbytes.Forecast(question, *forecast)
	if err != nil {
		return TargetArtifact{}, app.NewError(app.CodeInvalidData, "forecast target cannot be canonicalized", err)
	}
	relative := ledger.RelativePath(storage.DeterministicRelativePath("proofs/targets", string(forecastID)+".json"))
	return TargetArtifact{QuestionID: questionID, ForecastID: forecastID, Scope: ForecastEnvelopeSchema, RelativePath: relative, SHA256: digest, Size: len(canonicalBytes), Bytes: canonicalBytes}, nil
}

func BuildLifecycleTarget(model *ledger.Ledger, questionID, forecastID, headEventID ledger.Slug) (TargetArtifact, error) {
	if model == nil {
		return TargetArtifact{}, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return TargetArtifact{}, err
	}
	var forecast *ledger.Forecast
	for i := range question.Forecasts {
		if question.Forecasts[i].ID == forecastID {
			forecast = &question.Forecasts[i]
			break
		}
	}
	if forecast == nil {
		return TargetArtifact{}, app.WithDetails(app.NewError(app.CodeNotFound, "forecast was not found in the selected question", nil), map[string]any{"question_id": questionID, "forecast_id": forecastID})
	}
	if forecast.LifecycleEvents == nil {
		return TargetArtifact{}, app.NewError(app.CodeNotFound, "forecast has no lifecycle events", nil)
	}
	head := -1
	for i := range *forecast.LifecycleEvents {
		if (*forecast.LifecycleEvents)[i].ID == headEventID {
			head = i
			break
		}
	}
	if head < 0 {
		return TargetArtifact{}, app.WithDetails(app.NewError(app.CodeNotFound, "lifecycle head was not found in the selected forecast", nil), map[string]any{"head_event_id": headEventID})
	}
	canonicalBytes, digest, err := targetbytes.Lifecycle(question, *forecast, headEventID)
	if err != nil {
		return TargetArtifact{}, app.NewError(app.CodeInvalidData, "lifecycle target cannot be canonicalized", err)
	}
	name := string(forecastID) + ".lifecycle." + string(headEventID) + ".json"
	relative := ledger.RelativePath(storage.DeterministicRelativePath("proofs/targets", name))
	return TargetArtifact{QuestionID: questionID, ForecastID: forecastID, HeadEventID: headEventID, Scope: LifecycleTargetSchema, RelativePath: relative, SHA256: digest, Size: len(canonicalBytes), Bytes: canonicalBytes}, nil
}

func BuildAllForecastTargets(model *ledger.Ledger) ([]TargetArtifact, error) {
	if model == nil {
		return nil, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	result := []TargetArtifact{}
	for _, question := range model.Questions {
		for _, forecast := range question.Forecasts {
			artifact, err := BuildForecastTarget(model, question.ID, forecast.ID)
			if err != nil {
				return nil, err
			}
			result = append(result, artifact)
		}
	}
	paths := make([]string, len(result))
	for i := range result {
		paths[i] = string(result[i].RelativePath)
	}
	if err := storage.DetectPortablePathCollisions(paths); err != nil {
		return nil, err
	}
	return result, nil
}

func TargetMetadataFor(artifact TargetArtifact) ledger.ForecastTarget {
	return ledger.ForecastTarget{Scope: ForecastEnvelopeSchema, Canonicalization: TargetCanonicalization, ArtifactPath: artifact.RelativePath, Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}}
}

func LifecycleTargetMetadataFor(artifact TargetArtifact) ledger.LifecycleTarget {
	return ledger.LifecycleTarget{Scope: LifecycleTargetSchema, Canonicalization: TargetCanonicalization, ArtifactPath: artifact.RelativePath, Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}}
}
