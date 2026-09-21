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
