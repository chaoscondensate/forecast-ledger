// Package target builds the protocol-defined canonical forecast target bytes.
package target

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

const (
	ForecastEnvelopeSchema = "forecast-envelope/v2"
	LifecycleSchema        = "forecast-lifecycle/v1"
	Canonicalization       = "RFC8785"
)

type CommitmentProjection struct {
	Scheme         string            `json:"scheme"`
	CommitmentHash ledger.Digest     `json:"commitment_hash"`
	Encryption     ledger.Encryption `json:"encryption"`
}

type ForecastProjection struct {
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
	Commitment           *CommitmentProjection            `json:"commitment,omitempty"`
}

type forecastEnvelope struct {
	Schema   string `json:"schema"`
	Question struct {
		ID       ledger.Slug             `json:"id"`
		Revision ledger.QuestionRevision `json:"revision"`
	} `json:"question"`
	Forecast ForecastProjection `json:"forecast"`
}

type lifecycleEnvelope struct {
	Schema                 string                  `json:"schema"`
	QuestionID             ledger.Slug             `json:"question_id"`
	ForecastID             ledger.Slug             `json:"forecast_id"`
	ForecastEnvelopeSHA256 ledger.Digest           `json:"forecast_envelope_sha256"`
	LifecycleEvents        []ledger.LifecycleEvent `json:"lifecycle_events"`
	HeadEventID            ledger.Slug             `json:"head_event_id"`
}

// Forecast builds canonical forecast-envelope/v2 bytes and their SHA-256.
func Forecast(question ledger.Question, forecast ledger.Forecast) ([]byte, string, error) {
	var revision *ledger.QuestionRevision
	for index := range question.Revisions {
		if question.Revisions[index].ID == forecast.QuestionRevisionID {
			revision = &question.Revisions[index]
			break
		}
	}
	if revision == nil {
		return nil, "", fmt.Errorf("forecast references a missing question revision")
	}
	projection := ForecastProjection{
		ID: forecast.ID, QuestionRevisionID: forecast.QuestionRevisionID,
		ForecastedAt: forecast.ForecastedAt, RecordedAt: forecast.RecordedAt,
		Visibility: forecast.Visibility, PublicNote: forecast.PublicNote,
		SupersedesForecastID: forecast.SupersedesForecastID, Provenance: forecast.Provenance,
	}
	switch forecast.Visibility {
	case ledger.VisibilityPublic:
		projection.Representations, projection.Rationale = forecast.Representations, forecast.Rationale
		projection.KeyFactors, projection.Comment = forecast.KeyFactors, forecast.Comment
	case ledger.VisibilitySealed, ledger.VisibilityRevealed:
		projection.Visibility = ledger.VisibilitySealed
		if forecast.Commitment == nil {
			return nil, "", fmt.Errorf("sealed forecast commitment is missing")
		}
		if forecast.Commitment.Sealed != nil {
			value := forecast.Commitment.Sealed
			projection.Commitment = &CommitmentProjection{Scheme: value.Scheme, CommitmentHash: value.CommitmentHash, Encryption: value.Encryption}
		} else if forecast.Commitment.Revealed != nil {
			value := forecast.Commitment.Revealed
			projection.Commitment = &CommitmentProjection{Scheme: value.Scheme, CommitmentHash: value.CommitmentHash, Encryption: value.Encryption}
		} else {
			return nil, "", fmt.Errorf("sealed forecast commitment is empty")
		}
	default:
		return nil, "", fmt.Errorf("forecast visibility is not supported for targets")
	}
	value := forecastEnvelope{Schema: ForecastEnvelopeSchema, Forecast: projection}
	value.Question.ID, value.Question.Revision = question.ID, *revision
	return canonicalBytes(value)
}

// Lifecycle builds canonical forecast-lifecycle/v1 bytes through one head.
func Lifecycle(question ledger.Question, forecast ledger.Forecast, headEventID ledger.Slug) ([]byte, string, error) {
	if forecast.LifecycleEvents == nil {
		return nil, "", fmt.Errorf("forecast has no lifecycle events")
	}
	head := -1
	for index := range *forecast.LifecycleEvents {
		if (*forecast.LifecycleEvents)[index].ID == headEventID {
			head = index
			break
		}
	}
	if head < 0 {
		return nil, "", fmt.Errorf("lifecycle head does not exist")
	}
	_, forecastDigest, err := Forecast(question, forecast)
	if err != nil {
		return nil, "", err
	}
	value := lifecycleEnvelope{
		Schema: LifecycleSchema, QuestionID: question.ID, ForecastID: forecast.ID,
		ForecastEnvelopeSHA256: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(forecastDigest)},
		LifecycleEvents:        append([]ledger.LifecycleEvent(nil), (*forecast.LifecycleEvents)[:head+1]...),
		HeadEventID:            headEventID,
	}
	return canonicalBytes(value)
}

func canonicalBytes(value any) ([]byte, string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, "", fmt.Errorf("encode target: %w", err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return nil, "", fmt.Errorf("normalize target: %w", err)
	}
	result, err := canonical.Marshal(parsed.Root.Any())
	if err != nil {
		return nil, "", fmt.Errorf("canonicalize target: %w", err)
	}
	digest := sha256.Sum256(result)
	return result, hex.EncodeToString(digest[:]), nil
}
