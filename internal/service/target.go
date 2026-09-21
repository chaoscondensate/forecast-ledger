package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

const (
	ForecastEnvelopeSchema = "forecast-envelope/v2"
	TargetCanonicalization = "RFC8785"
)

type TargetArtifact struct {
	QuestionID   ledger.Slug         `json:"question_id"`
	ForecastID   ledger.Slug         `json:"forecast_id"`
	RelativePath ledger.RelativePath `json:"path"`
	SHA256       string              `json:"sha256"`
	Size         int                 `json:"size"`
	Bytes        []byte              `json:"-"`
}

type targetCommitment struct {
	Scheme         string            `json:"scheme"`
	CommitmentHash ledger.Digest     `json:"commitment_hash"`
	Encryption     ledger.Encryption `json:"encryption"`
}

// targetForecast is an explicit projection. Integrity, key_hint, revealed_at,
// and revealed_key cannot accidentally enter the timestamp claim.
type targetForecast struct {
	ID                   ledger.Slug                      `json:"id"`
	QuestionRevisionID   ledger.Slug                      `json:"question_revision_id"`
	ForecastedAt         ledger.Timestamp                 `json:"forecasted_at"`
	RecordedAt           ledger.Timestamp                 `json:"recorded_at"`
	Visibility           ledger.ForecastVisibility        `json:"visibility"`
	Representations      *[]ledger.ForecastRepresentation `json:"representations,omitempty"`
	Rationale            *string                          `json:"rationale,omitempty"`
	KeyFactors           *[]string                        `json:"key_factors,omitempty"`
	Comment              *string                          `json:"comment,omitempty"`
	PublicNote           *string                          `json:"public_note,omitempty"`
	SupersedesForecastID *ledger.Slug                     `json:"supersedes_forecast_id,omitempty"`
	Provenance           *ledger.Provenance               `json:"provenance,omitempty"`
	LifecycleEvents      *[]ledger.LifecycleEvent         `json:"lifecycle_events,omitempty"`
	Commitment           *targetCommitment                `json:"commitment,omitempty"`
}

type targetQuestion struct {
	ID       ledger.Slug             `json:"id"`
	Revision ledger.QuestionRevision `json:"revision"`
}
type forecastEnvelope struct {
	Schema   string         `json:"schema"`
	Question targetQuestion `json:"question"`
	Forecast targetForecast `json:"forecast"`
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
	envelope, err := buildForecastEnvelope(question, *forecast)
	if err != nil {
		return TargetArtifact{}, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return TargetArtifact{}, app.NewError(app.CodeInternal, "forecast target cannot be encoded", err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return TargetArtifact{}, app.NewError(app.CodeInternal, "forecast target cannot be normalized", err)
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil {
		return TargetArtifact{}, app.NewError(app.CodeInvalidData, "forecast target cannot be canonicalized", err)
	}
	digest := sha256.Sum256(canonicalBytes)
	relative := ledger.RelativePath(storage.DeterministicRelativePath("proofs/targets", string(forecastID)+".json"))
	return TargetArtifact{QuestionID: questionID, ForecastID: forecastID, RelativePath: relative, SHA256: hex.EncodeToString(digest[:]), Size: len(canonicalBytes), Bytes: canonicalBytes}, nil
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

func buildForecastEnvelope(question ledger.Question, forecast ledger.Forecast) (forecastEnvelope, error) {
	var revision *ledger.QuestionRevision
	for i := range question.Revisions {
		if question.Revisions[i].ID == forecast.QuestionRevisionID {
			revision = &question.Revisions[i]
			break
		}
	}
	if revision == nil {
		return forecastEnvelope{}, app.NewError(app.CodeInvalidData, "forecast references a missing question revision", nil)
	}
	target := targetForecast{ID: forecast.ID, QuestionRevisionID: forecast.QuestionRevisionID, ForecastedAt: forecast.ForecastedAt, RecordedAt: forecast.RecordedAt, Visibility: forecast.Visibility, PublicNote: cloneString(forecast.PublicNote), SupersedesForecastID: cloneSlug(forecast.SupersedesForecastID), Provenance: forecast.Provenance, LifecycleEvents: forecast.LifecycleEvents}
	switch forecast.Visibility {
	case ledger.VisibilityPublic:
		target.Representations = cloneRepresentations(forecast.Representations)
		target.Rationale = cloneString(forecast.Rationale)
		target.KeyFactors = cloneStrings(forecast.KeyFactors)
		target.Comment = cloneString(forecast.Comment)
	case ledger.VisibilitySealed, ledger.VisibilityRevealed:
		target.Visibility = ledger.VisibilitySealed
		sealed, _, err := originalSealedCommitment(forecast)
		if err != nil {
			return forecastEnvelope{}, err
		}
		target.Commitment = &targetCommitment{Scheme: sealed.Scheme, CommitmentHash: sealed.CommitmentHash, Encryption: sealed.Encryption}
	default:
		return forecastEnvelope{}, app.NewError(app.CodeInvalidData, "forecast visibility is not supported for targets", nil)
	}
	return forecastEnvelope{Schema: ForecastEnvelopeSchema, Question: targetQuestion{ID: question.ID, Revision: *revision}, Forecast: target}, nil
}

func TargetMetadataFor(artifact TargetArtifact) ledger.ForecastTarget {
	return ledger.ForecastTarget{Scope: ForecastEnvelopeSchema, Canonicalization: TargetCanonicalization, ArtifactPath: artifact.RelativePath, Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}}
}
