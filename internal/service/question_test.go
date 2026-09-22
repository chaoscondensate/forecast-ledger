package service

import (
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func TestQuestionRevisionIsAppendOnlyAndForecastHistoryStaysBound(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	model, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		t.Fatal(err)
	}
	revision := binaryInitialQuestion().Revision
	revision.ID = "qr-two"
	revision.EffectiveAt = "2026-02-01T00:00:00Z"
	revision.Title = "Will it happen under the clarified rule?"
	mutation, err := BuildQuestionRevise(model, "q-one", revision, "2026-02-01T00:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	question := mutation.Ledger.Questions[0]
	if len(question.Revisions) != 2 || question.CurrentRevisionID != "qr-two" || question.Forecasts[0].QuestionRevisionID != "qr-one" {
		t.Fatalf("append-only revision result = %#v", question)
	}
	if len(mutation.Patches) != 2 || mutation.TargetCoveredChanged {
		t.Fatalf("revision patches = %#v", mutation)
	}
}

func TestQuestionRevisionOmittedTimesAdvancePastHistory(t *testing.T) {
	tests := []struct {
		name          string
		previousEff   ledger.Timestamp
		previousRec   ledger.Timestamp
		observed      ledger.Timestamp
		wantEffective ledger.Timestamp
		wantRecorded  ledger.Timestamp
	}{
		{name: "same tick", previousEff: "2026-01-01T00:00:00Z", previousRec: "2026-01-01T00:00:00Z", observed: "2026-01-01T00:00:00Z", wantEffective: "2026-01-01T00:00:00.000000001Z", wantRecorded: "2026-01-01T00:00:00.000000001Z"},
		{name: "regressing clock and fractional history", previousEff: "2026-01-01T00:00:00.123456789Z", previousRec: "2026-01-01T00:00:00.5Z", observed: "2025-12-31T23:59:59Z", wantEffective: "2026-01-01T00:00:00.12345679Z", wantRecorded: "2026-01-01T00:00:00.5Z"},
		{name: "later observation", previousEff: "2026-01-01T00:00:00.1+01:00", previousRec: "2026-01-01T00:00:00.2+01:00", observed: "2026-01-01T00:00:01+01:00", wantEffective: "2026-01-01T00:00:01+01:00", wantRecorded: "2026-01-01T00:00:01+01:00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := testPublicInitialLedger(t)
			model.Questions[0].Forecasts = []ledger.Forecast{}
			model.Questions[0].Revisions[0].EffectiveAt = test.previousEff
			model.Questions[0].Revisions[0].RecordedAt = test.previousRec
			input := binaryInitialQuestion().Revision
			input.ID = "qr-two"
			input.EffectiveAt = ""
			input.RecordedAt = nil
			mutation, err := BuildQuestionRevise(model, "q-one", input, test.observed)
			if err != nil {
				t.Fatal(err)
			}
			got := mutation.Ledger.Questions[0].Revisions[1]
			if got.EffectiveAt != test.wantEffective || got.RecordedAt != test.wantRecorded {
				t.Fatalf("derived times = %s / %s, want %s / %s", got.EffectiveAt, got.RecordedAt, test.wantEffective, test.wantRecorded)
			}
		})
	}
}

func TestQuestionRevisionExplicitChronologyIsNotRepaired(t *testing.T) {
	model := testPublicInitialLedger(t)
	previous := model.Questions[0].Revisions[0]
	for _, effective := range []ledger.Timestamp{previous.EffectiveAt, "2025-12-31T23:59:59Z"} {
		input := binaryInitialQuestion().Revision
		input.ID = "qr-two"
		input.EffectiveAt = effective
		if _, err := BuildQuestionRevise(model, "q-one", input, "2026-02-01T00:00:00Z"); app.ErrorCodeOf(err) != app.CodeInvalidData {
			t.Fatalf("explicit effective_at %s error = %v", effective, err)
		}
	}

	overflow := testPublicInitialLedger(t)
	overflow.Questions[0].Revisions[0].EffectiveAt = "9999-12-31T23:59:59.999999999Z"
	overflow.Questions[0].Revisions[0].RecordedAt = "9999-12-31T23:59:59.999999999Z"
	input := binaryInitialQuestion().Revision
	input.ID, input.EffectiveAt, input.RecordedAt = "qr-overflow", "", nil
	if _, err := BuildQuestionRevise(overflow, "q-one", input, "9999-12-31T23:59:59.999999999Z"); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("overflow default error = %v", err)
	}
}

func TestQuestionMetadataUpdateCannotRewriteMeaning(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	updated, err := BuildQuestionUpdate(model, "q-election-coalition", QuestionPatchInput{Notes: Optional[string]{Set: true, Value: "Watch the coalition talks."}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ledger.Questions[1].Revisions[1].Title != model.Questions[1].Revisions[1].Title || len(updated.Patches) != 1 {
		t.Fatalf("metadata update changed semantic revision: %#v", updated)
	}
}

func TestResolvedAndUnresolvedTerminalShapes(t *testing.T) {
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey"}, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	model, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		t.Fatal(err)
	}
	closed, err := BuildQuestionUpdate(model, "q-one", QuestionPatchInput{Status: Optional[ledger.QuestionStatus]{Set: true, Value: ledger.QuestionClosed}})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	resolved, err := BuildQuestionResolve(closed.Ledger, "q-one", ResolutionInput{
		QuestionRevisionID: "qr-one", Outcome: ledger.ScalarValue{Boolean: &yes}, OutcomeKnownAt: "2027-01-01T00:00:00Z",
		Sources: []EvidenceSourceInput{{Title: "Official result", URL: "https://example.org/result", RetrievedAt: "2027-01-01T00:01:00Z"}},
	}, "2027-01-01T00:02:00Z")
	if err != nil || resolved.Ledger.Questions[0].Resolution.Resolved == nil {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	disputed, err := BuildQuestionUnresolved(resolved.Ledger, "q-one", ledger.ResolutionDisputed, UnresolvedResolutionInput{Reason: "The source is under review."}, "2027-01-02T00:00:00Z")
	if err != nil || disputed.Ledger.Questions[0].Resolution.Unresolved == nil || disputed.Ledger.Questions[0].Status != ledger.QuestionDisputed {
		t.Fatalf("disputed=%#v err=%v", disputed, err)
	}
	if _, err := BuildQuestionRevise(disputed.Ledger, "q-one", binaryInitialQuestion().Revision, "2027-01-03T00:00:00Z"); app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("terminal revise error = %v", err)
	}
}
