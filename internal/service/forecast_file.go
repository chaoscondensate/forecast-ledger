package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

type ForecastFileResult struct {
	LedgerID        ledger.Slug         `json:"ledger_id"`
	QuestionID      ledger.Slug         `json:"question_id"`
	ForecastID      ledger.Slug         `json:"forecast_id"`
	RecordedAt      ledger.Timestamp    `json:"recorded_at"`
	NormalizedTimes []TimeNormalization `json:"normalized_times,omitempty"`
	Changed         bool                `json:"changed"`
	ChangedPointers []string            `json:"changed_pointers"`
	BeforeSHA256    string              `json:"before_sha256"`
	AfterSHA256     string              `json:"after_sha256"`
	Effects         []SideEffect        `json:"effects,omitempty"`
	Recovery        Recovery            `json:"recovery,omitempty"`
}

func PlanPublicForecastAddFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, input ForecastCreateInput, observedAt ledger.Timestamp) (ForecastFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return ForecastFileResult{}, err
	}
	mutation, err := BuildPublicForecastAppend(loaded.Model, questionID, forecastID, input, observedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return ForecastFileResult{}, err
	}
	return buildForecastMutationFileResult(loaded.Document, loaded.Model, questionID, forecastID, mutation)
}

func CommitPublicForecastAddFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, input ForecastCreateInput, observedAt ledger.Timestamp) (ForecastFileResult, error) {
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return ForecastFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	var result ForecastFileResult
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for forecast mutation", decodeErr)
			}
			mutation, buildErr := BuildPublicForecastAppend(model, questionID, forecastID, input, observedAt)
			if buildErr != nil {
				return nil, buildErr
			}
			result, buildErr = buildForecastMutationFileResult(parsed, model, questionID, forecastID, mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return document.ApplyPatch(parsed, mutation.Patches)
		},
	})
	if err != nil {
		return ForecastFileResult{}, err
	}
	return result, nil
}

func PlanForecastLifecycleFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, eventType ledger.LifecycleEventType, input LifecycleInput, observedAt ledger.Timestamp) (ForecastFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return ForecastFileResult{}, err
	}
	mutation, err := BuildForecastLifecycle(loaded.Model, questionID, forecastID, eventType, input, observedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return ForecastFileResult{}, err
	}
	return buildForecastMutationFileResult(loaded.Document, loaded.Model, questionID, forecastID, mutation)
}

func CommitForecastLifecycleFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, eventType ledger.LifecycleEventType, input LifecycleInput, observedAt ledger.Timestamp) (ForecastFileResult, error) {
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return ForecastFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	var result ForecastFileResult
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for lifecycle mutation", decodeErr)
			}
			mutation, buildErr := BuildForecastLifecycle(model, questionID, forecastID, eventType, input, observedAt)
			if buildErr != nil {
				return nil, buildErr
			}
			result, buildErr = buildForecastMutationFileResult(parsed, model, questionID, forecastID, mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return document.ApplyPatch(parsed, mutation.Patches)
		},
	})
	return result, err
}

func buildForecastMutationFileResult(parsed *document.Document, model *ledger.Ledger, questionID, forecastID ledger.Slug, mutation ForecastMutation) (ForecastFileResult, error) {
	patched, err := document.ApplyPatch(parsed, mutation.Patches)
	if err != nil {
		return ForecastFileResult{}, app.NewError(app.CodeInternal, "forecast mutation cannot be applied to source document", err)
	}
	before := sha256.Sum256(parsed.Raw)
	after := sha256.Sum256(patched)
	_, question, err := selectQuestion(mutation.Ledger, questionID)
	if err != nil || len(question.Forecasts) == 0 {
		return ForecastFileResult{}, app.NewError(app.CodeInternal, "appended forecast cannot be selected", err)
	}
	var forecast ledger.Forecast
	found := false
	for _, candidate := range question.Forecasts {
		if candidate.ID == forecastID {
			forecast, found = candidate, true
			break
		}
	}
	if !found {
		return ForecastFileResult{}, app.NewError(app.CodeInternal, "mutated forecast cannot be selected", nil)
	}
	return ForecastFileResult{
		LedgerID: model.LedgerID, QuestionID: questionID, ForecastID: forecastID,
		RecordedAt: forecast.RecordedAt, Changed: len(mutation.Patches) > 0, ChangedPointers: ChangedPointers(mutation.Patches),
		BeforeSHA256: hex.EncodeToString(before[:]), AfterSHA256: hex.EncodeToString(after[:]),
	}, nil
}

func LoadForecastList(ctx context.Context, path string, stdin io.Reader, questionID ledger.Slug) (ledger.Slug, []ForecastSummary, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", nil, err
	}
	items, err := ListForecasts(loaded.Model, questionID)
	return loaded.Model.LedgerID, items, err
}

func LoadForecastShow(ctx context.Context, path string, stdin io.Reader, questionID, forecastID ledger.Slug) (ledger.Slug, ForecastView, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", ForecastView{}, err
	}
	result, err := ShowForecast(loaded.Model, questionID, forecastID)
	return loaded.Model.LedgerID, result, err
}
