package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/publication"
)

func TestEvidenceReconciliationNoEvidenceAndForecastTargetStates(t *testing.T) {
	model := testPublicInitialLedger(t)
	path := filepath.Join(t.TempDir(), "ledger.json")
	writeLedgerModel(t, path, model)
	loaded, err := loadAndValidateLedgerForEvidence(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || empty.State != ReconciliationNoEvidence {
		t.Fatalf("empty reconciliation = %#v, %v", empty, err)
	}

	artifact, err := BuildForecastTarget(model, "q-one", "f-one")
	if err != nil {
		t.Fatal(err)
	}
	artifact.RelativePath = "proofs/targets/f-one.json"
	model.Questions[0].Forecasts[0].Integrity = ledger.Integrity{Retained: &ledger.RetainedIntegrity{Status: ledger.IntegrityRetained, Target: TargetMetadataFor(artifact)}}
	writeLedgerModel(t, path, model)
	loaded, err = loadAndValidateLedgerForEvidence(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	missingIndex, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || missingIndex.State != ReconciliationIncomplete || !hasReconciliationIssue(missingIndex, "evidence.index_missing") {
		t.Fatalf("missing index reconciliation = %#v, %v", missingIndex, err)
	}

	entry := publication.EvidenceEntry{
		Role: publication.RoleForecastTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)),
		Digest:   publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256},
		Forecast: &publication.ForecastTargetBinding{QuestionID: "q-one", ForecastID: "f-one", Scope: ForecastEnvelopeSchema},
	}
	writeEvidenceIndexAndArtifacts(t, filepath.Dir(path), model.LedgerID, []publication.EvidenceEntry{entry}, map[string][]byte{entry.Path: artifact.Bytes})
	passed, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || passed.State != ReconciliationPass {
		t.Fatalf("reconciled store = %#v, %v", passed, err)
	}

	if err := os.Remove(filepath.Join(filepath.Dir(path), filepath.FromSlash(entry.Path))); err != nil {
		t.Fatal(err)
	}
	missing, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || missing.State != ReconciliationIncomplete || !hasReconciliationIssue(missing, "evidence.indexed_artifact_missing") {
		t.Fatalf("missing artifact = %#v, %v", missing, err)
	}
}

func TestEvidenceReconciliationDetectsUnindexedAndDetachedLifecycleEvidence(t *testing.T) {
	model := testPublicInitialLedger(t)
	withdrawn, err := BuildForecastLifecycle(model, "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-withdrawn", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	model = withdrawn.Ledger
	artifact, err := BuildLifecycleTarget(model, "q-one", "f-one", "event-withdrawn")
	if err != nil {
		t.Fatal(err)
	}
	artifact.RelativePath = "proofs/targets/f-one.lifecycle.event-withdrawn.json"
	checkpoint := ledger.ActivityCheckpoint{ID: "checkpoint-withdrawn", HeadEventID: "event-withdrawn", RecordedAt: "2026-02-01T00:00:02Z", Integrity: ledger.LifecycleIntegrity{Retained: &ledger.RetainedLifecycleIntegrity{Status: ledger.IntegrityRetained, Target: LifecycleTargetMetadataFor(artifact)}}}
	model.Questions[0].Forecasts[0].ActivityCheckpoints = &[]ledger.ActivityCheckpoint{checkpoint}
	path := filepath.Join(t.TempDir(), "ledger.json")
	writeLedgerModel(t, path, model)
	entry := publication.EvidenceEntry{
		Role: publication.RoleLifecycleTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)),
		Digest:    publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256},
		Lifecycle: &publication.LifecycleTargetBinding{QuestionID: "q-one", ForecastID: "f-one", CheckpointID: "checkpoint-withdrawn", HeadEventID: "event-withdrawn", Scope: LifecycleTargetSchema},
	}
	writeEvidenceIndexAndArtifacts(t, filepath.Dir(path), model.LedgerID, []publication.EvidenceEntry{entry}, map[string][]byte{entry.Path: artifact.Bytes, "proofs/planted.bin": []byte("planted")})
	loaded, err := loadAndValidateLedgerForEvidence(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	unindexed, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || unindexed.State != ReconciliationIncomplete || !hasReconciliationIssue(unindexed, "evidence.unindexed_artifact") {
		t.Fatalf("unindexed result = %#v, %v", unindexed, err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(path), "proofs", "planted.bin")); err != nil {
		t.Fatal(err)
	}
	model.Questions[0].Forecasts[0].LifecycleEvents = nil
	model.Questions[0].Forecasts[0].ActivityCheckpoints = nil
	writeLedgerModel(t, path, model)
	loaded, err = loadAndValidateLedgerForEvidence(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	detached, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || detached.State != ReconciliationFail || !hasReconciliationIssue(detached, "activity.retained_evidence_unreferenced") {
		t.Fatalf("detached lifecycle = %#v, %v", detached, err)
	}
}

func TestEvidenceReconciliationAcceptsCustomSafePathAndRejectsUnrelatedBinding(t *testing.T) {
	model := testPublicInitialLedger(t)
	artifact, err := BuildForecastTarget(model, "q-one", "f-one")
	if err != nil {
		t.Fatal(err)
	}
	artifact.RelativePath = "proofs/custom/forecast-target.bin"
	model.Questions[0].Forecasts[0].Integrity = ledger.Integrity{Retained: &ledger.RetainedIntegrity{Status: ledger.IntegrityRetained, Target: TargetMetadataFor(artifact)}}
	path := filepath.Join(t.TempDir(), "ledger.json")
	writeLedgerModel(t, path, model)
	entry := publication.EvidenceEntry{
		Role: publication.RoleForecastTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)),
		Digest:   publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256},
		Forecast: &publication.ForecastTargetBinding{QuestionID: "q-one", ForecastID: "f-one", Scope: ForecastEnvelopeSchema},
	}
	writeEvidenceIndexAndArtifacts(t, filepath.Dir(path), model.LedgerID, []publication.EvidenceEntry{entry}, map[string][]byte{entry.Path: artifact.Bytes})
	loaded, err := loadAndValidateLedgerForEvidence(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || result.State != ReconciliationPass {
		t.Fatalf("custom safe path reconciliation = %#v, %v", result, err)
	}

	unrelatedPath := "proofs/custom/unrelated-target.bin"
	unrelated := entry
	unrelated.Path = unrelatedPath
	unrelated.Forecast = &publication.ForecastTargetBinding{QuestionID: "q-one", ForecastID: "f-unrelated", Scope: ForecastEnvelopeSchema}
	writeEvidenceIndexAndArtifacts(t, filepath.Dir(path), model.LedgerID, []publication.EvidenceEntry{entry, unrelated}, map[string][]byte{entry.Path: artifact.Bytes, unrelatedPath: artifact.Bytes})
	result, err = ReconcileEvidenceStore(t.Context(), loaded)
	if err != nil || result.State != ReconciliationFail || !hasReconciliationIssue(result, "evidence.index_entry_unreferenced") {
		t.Fatalf("unrelated binding reconciliation = %#v, %v", result, err)
	}
}

func TestVerificationAggregatePrecedence(t *testing.T) {
	layer := func(state LayerState, reasons ...string) VerificationLayer {
		return VerificationLayer{Name: "test", State: state, ReasonCodes: reasons}
	}
	for _, testCase := range []struct {
		name   string
		report VerificationReport
		want   VerificationOverall
		code   app.ErrorCode
	}{
		{name: "failure before incomplete and pending", report: VerificationReport{Reconciliation: layer(LayerNotChecked), Forecasts: []ForecastVerification{{Layers: []VerificationLayer{layer(LayerPending), layer(LayerFail)}}}}, want: VerificationFail, code: app.CodeVerification},
		{name: "incomplete before pending", report: VerificationReport{Forecasts: []ForecastVerification{{Layers: []VerificationLayer{layer(LayerPending), layer(LayerNotChecked)}}}}, want: VerificationIncomplete, code: app.CodeIncomplete},
		{name: "network incomplete", report: VerificationReport{Forecasts: []ForecastVerification{{Layers: []VerificationLayer{layer(LayerNotChecked, "outcome.source_unavailable")}}}}, want: VerificationIncomplete, code: app.CodeNetwork},
		{name: "pending", report: VerificationReport{Forecasts: []ForecastVerification{{Layers: []VerificationLayer{layer(LayerPending)}}}}, want: VerificationPending, code: app.CodePending},
		{name: "no evidence", report: VerificationReport{}, want: VerificationNoEvidence, code: app.CodeIncomplete},
		{name: "pass", report: VerificationReport{Forecasts: []ForecastVerification{{Layers: []VerificationLayer{layer(LayerPass)}}}}, want: VerificationPass},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, code := aggregateVerification(testCase.report)
			if got != testCase.want || code != testCase.code {
				t.Fatalf("aggregate = %s/%s, want %s/%s", got, code, testCase.want, testCase.code)
			}
		})
	}
}

func FuzzEvidenceReconciliationTargetBytes(f *testing.F) {
	f.Add([]byte("changed"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, replacement []byte) {
		if len(replacement) > 4096 {
			replacement = replacement[:4096]
		}
		model := testPublicInitialLedger(t)
		artifact, err := BuildForecastTarget(model, "q-one", "f-one")
		if err != nil {
			t.Fatal(err)
		}
		artifact.RelativePath = "proofs/targets/f-one.json"
		model.Questions[0].Forecasts[0].Integrity = ledger.Integrity{Retained: &ledger.RetainedIntegrity{Status: ledger.IntegrityRetained, Target: TargetMetadataFor(artifact)}}
		path := filepath.Join(t.TempDir(), "ledger.json")
		writeLedgerModel(t, path, model)
		entry := publication.EvidenceEntry{
			Role: publication.RoleForecastTarget, Path: string(artifact.RelativePath), Size: int64(len(artifact.Bytes)),
			Digest: publication.Digest{Algorithm: "sha-256", Value: artifact.SHA256}, Forecast: &publication.ForecastTargetBinding{QuestionID: "q-one", ForecastID: "f-one", Scope: ForecastEnvelopeSchema},
		}
		writeEvidenceIndexAndArtifacts(t, filepath.Dir(path), model.LedgerID, []publication.EvidenceEntry{entry}, map[string][]byte{entry.Path: replacement})
		loaded, err := loadAndValidateLedgerForEvidence(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := ReconcileEvidenceStore(t.Context(), loaded)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(replacement, artifact.Bytes) && result.State != ReconciliationPass {
			t.Fatalf("exact bytes did not reconcile: %#v", result)
		}
		if !bytes.Equal(replacement, artifact.Bytes) && result.State != ReconciliationFail {
			t.Fatalf("changed bytes were not rejected: %#v", result)
		}
	})
}

func writeEvidenceIndexAndArtifacts(t *testing.T, root string, ledgerID ledger.Slug, entries []publication.EvidenceEntry, artifacts map[string][]byte) {
	t.Helper()
	for relative, data := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	publication.SortEvidenceEntries(entries)
	index := publication.EvidenceIndex{Schema: publication.EvidenceIndexProfile, LedgerID: string(ledgerID), Contract: publication.CurrentContractIdentity(), Entries: entries}
	encoded, err := publication.EncodeEvidenceIndex(index, false)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, filepath.FromSlash(publication.EvidenceIndexPath))
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasReconciliationIssue(result EvidenceReconciliation, code string) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
