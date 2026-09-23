package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/publication"
)

type targetRetentionPlan struct {
	LedgerBytes []byte
	IndexBytes  []byte
	Index       publication.EvidenceIndex
	Changed     bool
}

func buildTargetRetentionPlan(loaded *LoadedLedger, artifacts []TargetArtifact, scope TargetScope, checkpointID ledger.Slug, recordedAt ledger.Timestamp, existing *publication.EvidenceIndex) (targetRetentionPlan, error) {
	if loaded == nil || loaded.Model == nil || loaded.Document == nil {
		return targetRetentionPlan{}, app.NewError(app.CodeInternal, "target retention has no loaded ledger", nil)
	}
	if scope == TargetScopeLifecycle {
		if checkpointID == "" || recordedAt == "" {
			return targetRetentionPlan{}, app.NewError(app.CodeUsage, "lifecycle target build requires checkpoint and recorded_at", nil)
		}
		if _, err := ParseTimestamp(recordedAt, "recorded_at"); err != nil {
			return targetRetentionPlan{}, err
		}
	} else if checkpointID != "" || recordedAt != "" {
		return targetRetentionPlan{}, app.NewError(app.CodeUsage, "checkpoint and recorded_at can be used only with lifecycle target scope", nil)
	}
	patches := make([]document.PatchOperation, 0, len(artifacts))
	entries := make([]publication.EvidenceEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		questionPosition, _, forecastPosition, forecast, err := selectForecast(loaded.Model, artifact.QuestionID, artifact.ForecastID)
		if err != nil {
			return targetRetentionPlan{}, err
		}
		base := fmt.Sprintf("/questions/%d/forecasts/%d", questionPosition, forecastPosition)
		if artifact.Scope == ForecastEnvelopeSchema {
			if forecast.Integrity.Unanchored == nil {
				target := recordedForecastTarget(loaded.Model, artifact.QuestionID, artifact.ForecastID)
				if target == nil || target.ArtifactPath != artifact.RelativePath || string(target.Digest.Value) != artifact.SHA256 {
					return targetRetentionPlan{}, app.NewError(app.CodeConflict, "forecast already has different retained evidence", nil)
				}
			} else {
				value, err := jsonPatchValue(ledger.Integrity{Retained: &ledger.RetainedIntegrity{Status: ledger.IntegrityRetained, Target: TargetMetadataFor(artifact)}})
				if err != nil {
					return targetRetentionPlan{}, err
				}
				patches = append(patches, document.PatchOperation{Kind: document.PatchReplace, Pointer: base + "/integrity", Value: value})
			}
			entries = append(entries, publication.EvidenceEntry{Role: publication.RoleForecastTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)), Digest: publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256}, Forecast: &publication.ForecastTargetBinding{QuestionID: string(artifact.QuestionID), ForecastID: string(artifact.ForecastID), Scope: artifact.Scope}})
			continue
		}
		if forecast.ActivityCheckpoints != nil {
			for _, checkpoint := range *forecast.ActivityCheckpoints {
				if checkpoint.ID == checkpointID || checkpoint.HeadEventID == artifact.HeadEventID {
					return targetRetentionPlan{}, app.NewError(app.CodeConflict, "lifecycle checkpoint ID or head is already retained", nil)
				}
			}
		}
		checkpoint := ledger.ActivityCheckpoint{ID: checkpointID, HeadEventID: artifact.HeadEventID, RecordedAt: recordedAt, Integrity: ledger.LifecycleIntegrity{Retained: &ledger.RetainedLifecycleIntegrity{Status: ledger.IntegrityRetained, Target: LifecycleTargetMetadataFor(artifact)}}}
		value, err := jsonPatchValue(checkpoint)
		if err != nil {
			return targetRetentionPlan{}, err
		}
		pointer := base + "/activity_checkpoints"
		if forecast.ActivityCheckpoints == nil {
			patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: pointer, Value: []any{value}})
		} else {
			patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: pointer + "/-", Value: value})
		}
		entries = append(entries, publication.EvidenceEntry{Role: publication.RoleLifecycleTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)), Digest: publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256}, Lifecycle: &publication.LifecycleTargetBinding{QuestionID: string(artifact.QuestionID), ForecastID: string(artifact.ForecastID), CheckpointID: string(checkpointID), HeadEventID: string(artifact.HeadEventID), Scope: artifact.Scope}})
	}
	ledgerBytes := bytes.Clone(loaded.Document.Raw)
	if len(patches) > 0 {
		var err error
		ledgerBytes, err = document.ApplyPatch(loaded.Document, patches)
		if err != nil {
			return targetRetentionPlan{}, err
		}
		parsed, err := parsePatchedLedger(loaded.Document.Format, ledgerBytes)
		if err != nil {
			return targetRetentionPlan{}, err
		}
		if err := ValidateLedgerDocument(parsed, nil); err != nil {
			return targetRetentionPlan{}, preserveValidationDetails(app.CodeInvalidData, "prospective retained target ledger is invalid", err)
		}
	}
	index := publication.EvidenceIndex{Schema: publication.EvidenceIndexProfile, LedgerID: string(loaded.Model.LedgerID), Contract: publication.CurrentContractIdentity(), Entries: []publication.EvidenceEntry{}}
	if existing != nil {
		index = *existing
		index.Entries = append([]publication.EvidenceEntry(nil), existing.Entries...)
	}
	byPath := make(map[string]publication.EvidenceEntry, len(index.Entries))
	for _, entry := range index.Entries {
		byPath[entry.Path] = entry
	}
	for _, entry := range entries {
		if current, exists := byPath[entry.Path]; exists {
			left, _ := json.Marshal(current)
			right, _ := json.Marshal(entry)
			if !bytes.Equal(left, right) {
				return targetRetentionPlan{}, app.NewError(app.CodeConflict, "evidence index path already has different metadata", nil)
			}
			continue
		}
		index.Entries = append(index.Entries, entry)
	}
	publication.SortEvidenceEntries(index.Entries)
	indexBytes, err := publication.EncodeEvidenceIndex(index, false)
	if err != nil {
		return targetRetentionPlan{}, err
	}
	changed := len(patches) > 0
	if existing == nil {
		changed = true
	}
	return targetRetentionPlan{LedgerBytes: ledgerBytes, IndexBytes: indexBytes, Index: index, Changed: changed}, nil
}

func parsePatchedLedger(format document.Format, data []byte) (*document.Document, error) {
	switch format {
	case document.FormatJSON:
		return document.ParseJSON(bytes.NewReader(data), document.DefaultLimits)
	case document.FormatYAML:
		return document.ParseYAML(bytes.NewReader(data), document.DefaultLimits)
	default:
		return nil, errors.New("unsupported ledger format")
	}
}
