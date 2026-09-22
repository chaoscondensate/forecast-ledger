package service

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestPublicForecastAppendRequiresExplicitOwnedRevision(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	input := coalitionForecastInput()
	input.SupersedesForecastID = slugPointer("f-election-coalition-002")
	mutation, err := BuildPublicForecastAppend(model, "q-election-coalition", "f-election-coalition-003", input, "2026-09-01T09:01:00+01:00")
	if err != nil {
		t.Fatal(err)
	}
	question, _, _ := selectQuestion(mutation.Ledger, "q-election-coalition")
	appended := mutation.Ledger.Questions[question].Forecasts[2]
	if appended.QuestionRevisionID != "qr-election-coalition-2" || appended.Visibility != ledger.VisibilityPublic || appended.Integrity.Unanchored == nil {
		t.Fatalf("appended forecast = %#v", appended)
	}
	input.QuestionRevisionID = "qr-central-bank-cut-1"
	if _, err := BuildPublicForecastAppend(model, "q-election-coalition", "f-election-coalition-003", input, "2026-09-01T09:01:00+01:00"); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("foreign revision error = %v", err)
	}
}

func TestForecastListAndShowExposeRepresentationKinds(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	items, err := ListForecasts(model, "q-quarterly-revenue")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].RepresentationKinds) != 5 || items[0].QuestionRevisionID != "qr-quarterly-revenue-1" {
		t.Fatalf("list = %#v", items)
	}
	view, err := ShowForecast(model, "q-quarterly-revenue", "f-quarterly-revenue-001")
	if err != nil || view.Representations == nil || len(*view.Representations) != 5 {
		t.Fatalf("show = %#v, %v", view, err)
	}
}

func TestDeriveActivitySeparatesCurrentStateFromCheckpointCoverage(t *testing.T) {
	events := []ledger.LifecycleEvent{
		{ID: "event-withdrawn", Type: ledger.LifecycleWithdrawn, EffectiveAt: "2026-02-01T00:00:00Z", RecordedAt: "2026-02-01T00:00:01Z"},
		{ID: "event-reaffirmed", Type: ledger.LifecycleReaffirmed, EffectiveAt: "2026-02-02T00:00:00Z", RecordedAt: "2026-02-02T00:00:01Z"},
	}
	forecast := ledger.Forecast{LifecycleEvents: &events}
	activity := deriveActivity(forecast)
	if !activity.Active || activity.EventCount != 2 || activity.LastEventID != "event-reaffirmed" || activity.Coverage != ActivityUnbound {
		t.Fatalf("unbound activity = %#v", activity)
	}
	checkpoints := []ledger.ActivityCheckpoint{{
		ID: "checkpoint-event-withdrawn", HeadEventID: "event-withdrawn", RecordedAt: "2026-02-01T00:00:02Z",
		Integrity: ledger.LifecycleIntegrity{Verified: &ledger.VerifiedLifecycleIntegrity{Status: ledger.IntegrityVerified}},
	}}
	forecast.ActivityCheckpoints = &checkpoints
	activity = deriveActivity(forecast)
	if activity.Coverage != ActivityPartial || activity.CoveredHead != "event-withdrawn" || !activity.Active {
		t.Fatalf("partial activity = %#v", activity)
	}
	checkpoints[0].HeadEventID = "event-reaffirmed"
	activity = deriveActivity(forecast)
	if activity.Coverage != ActivityVerified {
		t.Fatalf("verified activity = %#v", activity)
	}
	checkpoints[0].Integrity = ledger.LifecycleIntegrity{Pending: &ledger.PendingLifecycleIntegrity{Status: ledger.IntegrityPending}}
	if activity = deriveActivity(forecast); activity.Coverage != ActivityPending {
		t.Fatalf("pending activity = %#v", activity)
	}
	checkpoints[0].Integrity = ledger.LifecycleIntegrity{Failed: &ledger.FailedLifecycleIntegrity{Status: ledger.IntegrityFailed}}
	if activity = deriveActivity(forecast); activity.Coverage != ActivityFailed {
		t.Fatalf("failed activity = %#v", activity)
	}
}

func TestActivityClassificationDoesNotLetOlderVerifiedEvidenceOverrideCurrentPendingHead(t *testing.T) {
	state, reasons := classifyActivityEvidence(ActivityResult{Coverage: ActivityPending}, true, true, false)
	if state != LayerPending || len(reasons) != 1 || reasons[0] != "activity.evidence_pending" {
		t.Fatalf("current pending head classified as %s %v", state, reasons)
	}
	state, reasons = classifyActivityEvidence(ActivityResult{Coverage: ActivityPartial}, true, false, false)
	if state != LayerPass || len(reasons) != 1 || reasons[0] != "activity.prefix_verified_current_head_uncovered" {
		t.Fatalf("verified older prefix classified as %s %v", state, reasons)
	}
}

func TestForecastFileMutationIsMinimalAndStdinReadsWork(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	input := coalitionForecastInput()
	input.RecordedAt = timestampPointer("2026-09-01T09:01:00+01:00")
	result, err := CommitPublicForecastAddFile(context.Background(), path, "q-election-coalition", "f-election-coalition-003", input, "2026-09-01T09:01:00+01:00")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.ChangedPointers) != 1 || result.ChangedPointers[0] != "/questions/1/forecasts/-" {
		t.Fatalf("result = %#v", result)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ledgerID, items, err := LoadForecastList(context.Background(), "-", bytes.NewReader(updated), "q-election-coalition")
	if err != nil || ledgerID == "" || len(items) != 3 {
		t.Fatalf("stdin list = %q %#v, %v", ledgerID, items, err)
	}
}

func coalitionForecastInput() ForecastCreateInput {
	return ForecastCreateInput{
		QuestionRevisionID: "qr-election-coalition-2",
		ForecastedAt:       "2026-09-01T09:00:00+01:00",
		Representations: []ledger.ForecastRepresentation{{PMF: &ledger.PMFRepresentation{
			Kind: ledger.RepresentationPMF, OptionSetRef: ledger.OptionSetRef{ID: "coalitions", Version: 2},
			Entries: []ledger.PMFEntry{{OptionID: "centre-left", Probability: "0.5"}, {OptionID: "centre-right", Probability: "0.3"}, {OptionID: "unity", Probability: "0.1"}, {OptionID: "other", Probability: "0.1"}},
		}}},
	}
}

func slugPointer(value ledger.Slug) *ledger.Slug                { return &value }
func timestampPointer(value ledger.Timestamp) *ledger.Timestamp { return &value }
