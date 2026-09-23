package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/publication"
	"github.com/chaoscondensate/forecast-ledger/internal/schema"
	targetbytes "github.com/chaoscondensate/forecast-ledger/internal/target"
)

type ReconciliationState string

const (
	ReconciliationPass       ReconciliationState = "pass"
	ReconciliationFail       ReconciliationState = "fail"
	ReconciliationIncomplete ReconciliationState = "incomplete"
	ReconciliationNoEvidence ReconciliationState = "no_evidence"
)

type ReconciliationIssue struct {
	Code  string              `json:"code"`
	Path  string              `json:"path,omitempty"`
	State ReconciliationState `json:"state"`
}

type EvidenceReconciliation struct {
	State        ReconciliationState        `json:"state"`
	IndexPresent bool                       `json:"index_present"`
	Declarations int                        `json:"declarations"`
	Indexed      int                        `json:"indexed"`
	Files        int                        `json:"files"`
	Issues       []ReconciliationIssue      `json:"issues,omitempty"`
	Index        *publication.EvidenceIndex `json:"-"`
	Store        publication.ManagedStore   `json:"-"`
}

type expectedEvidence struct {
	role           string
	targetPath     string
	requestPath    string
	trustPath      string
	tsaURL         string
	questionID     ledger.Slug
	forecastID     ledger.Slug
	checkpointID   ledger.Slug
	headEventID    ledger.Slug
	scope          string
	digest         string
	messageImprint string
	bytes          []byte
}

func ReconcileEvidenceStore(ctx context.Context, loaded *LoadedLedger) (EvidenceReconciliation, error) {
	if loaded == nil || loaded.Model == nil || loaded.Path == "" {
		return EvidenceReconciliation{}, app.NewError(app.CodeInternal, "evidence reconciliation has no loaded ledger", nil)
	}
	if ctx != nil && ctx.Err() != nil {
		return EvidenceReconciliation{}, app.NewError(app.CodeInterrupted, "evidence reconciliation was interrupted", ctx.Err())
	}
	store, err := publication.InspectManagedStore(filepath.Dir(loaded.Path), publication.DefaultManagedStoreLimits())
	if err != nil {
		return EvidenceReconciliation{}, app.NewError(app.CodeVerification, "managed evidence store is unsafe", err)
	}
	expected, err := expectedEvidenceDeclarations(loaded.Model)
	if err != nil {
		return EvidenceReconciliation{}, err
	}
	result := EvidenceReconciliation{State: ReconciliationPass, IndexPresent: store.IndexBytes != nil, Declarations: len(expected), Files: len(store.Artifacts), Store: store}
	add := func(code, path string, state ReconciliationState) {
		result.Issues = append(result.Issues, ReconciliationIssue{Code: code, Path: path, State: state})
		if state == ReconciliationFail || result.State == ReconciliationPass || result.State == ReconciliationNoEvidence {
			result.State = state
		}
	}
	if store.IndexBytes == nil {
		if len(expected) == 0 && len(store.Artifacts) == 0 {
			result.State = ReconciliationNoEvidence
			return result, nil
		}
		if len(expected) > 0 {
			add("evidence.index_missing", publication.EvidenceIndexPath, ReconciliationIncomplete)
		}
		for path := range store.Artifacts {
			add("evidence.unindexed_artifact", path, ReconciliationIncomplete)
		}
		return result, nil
	}
	index, err := publication.DecodeEvidenceIndex(store.IndexBytes, false)
	if err != nil {
		add("evidence.index_invalid", publication.EvidenceIndexPath, ReconciliationFail)
		return result, nil
	}
	if index.LedgerID != string(loaded.Model.LedgerID) {
		add("evidence.index_ledger_mismatch", publication.EvidenceIndexPath, ReconciliationFail)
	}
	result.Index, result.Indexed = &index, len(index.Entries)
	indexed := make(map[string]publication.EvidenceEntry, len(index.Entries))
	for _, entry := range index.Entries {
		indexed[entry.Path] = entry
		artifact, exists := store.Artifacts[entry.Path]
		if !exists {
			add("evidence.indexed_artifact_missing", entry.Path, ReconciliationIncomplete)
			continue
		}
		if artifact.Size != entry.Size || artifact.SHA256 != entry.Digest.Value {
			add("evidence.artifact_mismatch", entry.Path, ReconciliationFail)
		}
	}
	for path := range store.Artifacts {
		if _, exists := indexed[path]; !exists {
			add("evidence.unindexed_artifact", path, ReconciliationIncomplete)
		}
	}
	for path, declaration := range expected {
		entry, exists := indexed[path]
		if !exists {
			add("evidence.declaration_missing_index", path, ReconciliationIncomplete)
			continue
		}
		if !entryMatchesDeclaration(entry, declaration) {
			add("evidence.binding_mismatch", path, ReconciliationFail)
			continue
		}
		if declaration.bytes != nil {
			artifact, exists := store.Artifacts[path]
			if exists && !bytes.Equal(artifact.Bytes, declaration.bytes) {
				add("evidence.canonical_target_mismatch", path, ReconciliationFail)
			}
		}
	}
	for _, entry := range index.Entries {
		if _, declared := expected[entry.Path]; declared {
			continue
		}
		artifact, exists := store.Artifacts[entry.Path]
		if entry.Role == publication.RoleLifecycleTarget {
			if exists && validDetachedLifecycleTarget(loaded.Model, entry, artifact.Bytes) {
				add("activity.retained_evidence_unreferenced", entry.Path, ReconciliationFail)
			} else {
				add("evidence.index_entry_unreferenced", entry.Path, ReconciliationIncomplete)
			}
			continue
		}
		add("evidence.index_entry_unreferenced", entry.Path, ReconciliationFail)
	}
	return result, nil
}

func expectedEvidenceDeclarations(model *ledger.Ledger) (map[string]expectedEvidence, error) {
	result := make(map[string]expectedEvidence)
	add := func(path string, value expectedEvidence) error {
		if existing, exists := result[path]; exists && !reflect.DeepEqual(existing, value) {
			return app.NewError(app.CodeVerification, "ledger evidence declarations conflict on one path", nil)
		}
		result[path] = value
		return nil
	}
	for _, question := range model.Questions {
		for _, forecast := range question.Forecasts {
			if target := recordedForecastTarget(model, question.ID, forecast.ID); target != nil {
				artifact, err := BuildForecastTarget(model, question.ID, forecast.ID)
				if err != nil {
					return nil, err
				}
				declaration := expectedEvidence{role: publication.RoleForecastTarget, questionID: question.ID, forecastID: forecast.ID, scope: target.Scope, digest: string(target.Digest.Value), bytes: artifact.Bytes}
				if err := add(string(target.ArtifactPath), declaration); err != nil {
					return nil, err
				}
				for _, timestamp := range integrityTimestamps(forecast.Integrity) {
					if err := addTimestampDeclarations(add, target.ArtifactPath, target.Digest.Value, timestamp); err != nil {
						return nil, err
					}
				}
			}
			if forecast.ActivityCheckpoints == nil {
				continue
			}
			for _, checkpoint := range *forecast.ActivityCheckpoints {
				target := lifecycleIntegrityTarget(checkpoint.Integrity)
				if target == nil {
					continue
				}
				artifact, err := BuildLifecycleTarget(model, question.ID, forecast.ID, checkpoint.HeadEventID)
				if err != nil {
					return nil, err
				}
				declaration := expectedEvidence{role: publication.RoleLifecycleTarget, questionID: question.ID, forecastID: forecast.ID, checkpointID: checkpoint.ID, headEventID: checkpoint.HeadEventID, scope: target.Scope, digest: string(target.Digest.Value), bytes: artifact.Bytes}
				if err := add(string(target.ArtifactPath), declaration); err != nil {
					return nil, err
				}
				for _, timestamp := range lifecycleIntegrityTimestamps(checkpoint.Integrity) {
					if err := addTimestampDeclarations(add, target.ArtifactPath, target.Digest.Value, timestamp); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return result, nil
}

func addTimestampDeclarations(add func(string, expectedEvidence) error, targetPath ledger.RelativePath, targetDigest ledger.Hex32, timestamp ledger.RFC3161Timestamp) error {
	request := expectedEvidence{role: publication.RoleRFC3161Request, targetPath: string(targetPath), messageImprint: string(targetDigest), scope: "sha256", requestPath: string(timestamp.RequestPath)}
	if err := add(string(timestamp.RequestPath), request); err != nil {
		return err
	}
	response := expectedEvidence{role: publication.RoleRFC3161Response, targetPath: string(targetPath), requestPath: string(timestamp.RequestPath), tsaURL: timestamp.TSAURL}
	if timestamp.CABundlePath != nil {
		response.trustPath = string(*timestamp.CABundlePath)
		if err := add(response.trustPath, expectedEvidence{role: publication.RoleX509CABundle}); err != nil {
			return err
		}
	}
	return add(string(timestamp.ResponsePath), response)
}

func entryMatchesDeclaration(entry publication.EvidenceEntry, expected expectedEvidence) bool {
	switch expected.role {
	case publication.RoleForecastTarget:
		return entry.Role == expected.role && entry.Forecast != nil && entry.Forecast.QuestionID == string(expected.questionID) && entry.Forecast.ForecastID == string(expected.forecastID) && entry.Forecast.Scope == expected.scope && entry.Digest.Value == expected.digest
	case publication.RoleLifecycleTarget:
		return entry.Role == expected.role && entry.Lifecycle != nil && entry.Lifecycle.QuestionID == string(expected.questionID) && entry.Lifecycle.ForecastID == string(expected.forecastID) && entry.Lifecycle.CheckpointID == string(expected.checkpointID) && entry.Lifecycle.HeadEventID == string(expected.headEventID) && entry.Lifecycle.Scope == expected.scope && entry.Digest.Value == expected.digest
	case publication.RoleRFC3161Request:
		return entry.Role == expected.role && entry.TargetRef != nil && entry.TargetRef.TargetPath == expected.targetPath && entry.Request != nil && entry.Request.HashAlgorithm == "sha256" && entry.Request.MessageImprintSHA256 == expected.messageImprint
	case publication.RoleRFC3161Response:
		trust := ""
		if entry.Response != nil && entry.Response.TrustPath != nil {
			trust = *entry.Response.TrustPath
		}
		return entry.Role == expected.role && entry.Response != nil && entry.Response.TargetPath == expected.targetPath && entry.Response.RequestPath == expected.requestPath && trust == expected.trustPath && entry.TSA != nil && entry.TSA.TSAURL == expected.tsaURL
	case publication.RoleX509CABundle:
		return entry.Role == expected.role && entry.Format != nil && *entry.Format == publication.PEMCertificateBundle
	default:
		return false
	}
}

type detachedLifecycleTarget struct {
	Schema                 string                  `json:"schema"`
	QuestionID             ledger.Slug             `json:"question_id"`
	ForecastID             ledger.Slug             `json:"forecast_id"`
	ForecastEnvelopeSHA256 ledger.Digest           `json:"forecast_envelope_sha256"`
	LifecycleEvents        []ledger.LifecycleEvent `json:"lifecycle_events"`
	HeadEventID            ledger.Slug             `json:"head_event_id"`
}

func validDetachedLifecycleTarget(model *ledger.Ledger, entry publication.EvidenceEntry, data []byte) bool {
	if entry.Lifecycle == nil || len(data) == 0 {
		return false
	}
	parsed, err := document.ParseJSON(bytes.NewReader(data), document.Limits{MaxBytes: maxTargetBytes, MaxDepth: 32, MaxNodes: 10000, MaxScalarBytes: 1 << 20})
	if err != nil {
		return false
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil || !bytes.Equal(canonicalBytes, data) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var target detachedLifecycleTarget
	if err := decoder.Decode(&target); err != nil || target.Schema != schema.LifecycleTargetProfile || target.QuestionID != ledger.Slug(entry.Lifecycle.QuestionID) || target.ForecastID != ledger.Slug(entry.Lifecycle.ForecastID) || target.HeadEventID != ledger.Slug(entry.Lifecycle.HeadEventID) || len(target.LifecycleEvents) == 0 || target.LifecycleEvents[len(target.LifecycleEvents)-1].ID != target.HeadEventID {
		return false
	}
	_, question, _, forecast, err := selectForecast(model, target.QuestionID, target.ForecastID)
	if err != nil {
		return false
	}
	_, envelopeDigest, err := targetbytes.Forecast(question, forecast)
	return err == nil && target.ForecastEnvelopeSHA256.Algorithm == "sha-256" && string(target.ForecastEnvelopeSHA256.Value) == envelopeDigest
}

func reconciliationError(result EvidenceReconciliation) error {
	if result.State == ReconciliationPass || result.State == ReconciliationNoEvidence {
		return nil
	}
	code := app.CodeVerification
	if result.State == ReconciliationIncomplete {
		code = app.CodeInvalidData
	}
	return app.WithDetails(app.NewError(code, "managed evidence store is not reconciled", errors.New(string(result.State))), map[string]any{"reconciliation": result})
}
