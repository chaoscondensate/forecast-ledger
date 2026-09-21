package service

import (
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func TestBuildInitialPublicLedgerBindsForecastToFirstRevision(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{
		LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey",
	}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		t.Fatal(err)
	}
	question := result.Questions[0]
	forecast := question.Forecasts[0]
	if question.CurrentRevisionID != "qr-one" || len(question.Revisions) != 1 || forecast.QuestionRevisionID != "qr-one" {
		t.Fatalf("question/forecast revision binding = %#v / %#v", question, forecast)
	}
	if forecast.Visibility != ledger.VisibilityPublic || forecast.RecordedAt != root.CreatedAt || forecast.Integrity.Unanchored == nil {
		t.Fatalf("initial forecast = %#v", forecast)
	}
	if err := ValidateProspectiveLedgerModel(result); err != nil {
		t.Fatal(err)
	}
}

func TestInitialQuestionRejectsInvalidRevisionAndDuplicateGlobalForecastID(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	invalid := binaryInitialQuestion()
	invalid.Revision.OutcomeSpace.Kind = ledger.OutcomeNumeric
	if _, err := BuildInitialPublicLedger(root, invalid); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("mismatched outcome/domain error = %v", err)
	}
	valid, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		t.Fatal(err)
	}
	duplicate := binaryInitialQuestion()
	duplicate.ID = "q-two"
	duplicate.Revision.ID = "qr-two"
	if _, err := BuildQuestionWithInitialPublicForecast(valid, NormalizeInitialQuestion(duplicate), valid.CreatedAt); app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("duplicate global forecast error = %v", err)
	}
}

func binaryInitialQuestion() InitialQuestionInput {
	return InitialQuestionInput{
		ID: "q-one",
		Revision: RevisionInput{
			ID: "qr-one", Title: "Will it happen?", ResolutionCriteria: "Resolve from the named public source.",
			ExpectedResolutionAt: "2027-01-01T00:00:00Z",
			OutcomeSpace:         ledger.OutcomeSpace{Kind: ledger.OutcomeBinary},
			Domain:               ledger.Domain{Binary: &ledger.BinaryDomain{Kind: ledger.OutcomeBinary}},
		},
		InitialForecast: &InitialForecastInput{
			ID: "f-one", Visibility: ledger.VisibilityPublic, ForecastedAt: "2026-01-01T00:00:00Z",
			Representations: []ledger.ForecastRepresentation{{Probability: &ledger.ProbabilityRepresentation{
				Kind: ledger.RepresentationProbability, Outcome: true, Probability: "0.5",
			}}},
		},
	}
}
