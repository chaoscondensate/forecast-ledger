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

func TestEmptyAndQuestionOnlyCreationShapesV2(t *testing.T) {
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "empty", Timezone: "UTC", ForecasterID: "owner", ForecasterName: "Owner"}, "2026-08-29T10:00:00Z")
	if err != nil {
		t.Fatalf("%#v", err)
	}
	shape, err := ClassifyInitInput(InitInput{})
	if err != nil || shape != CreationLedgerOnly {
		t.Fatalf("shape=%q err=%v", shape, err)
	}
	question := binaryInitialQuestion()
	question.InitialForecast = nil
	withQuestion, err := BuildInitialQuestionLedgerAt(root, question, "2026-08-29T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(withQuestion.Questions) != 1 || len(withQuestion.Questions[0].Revisions) != 1 || len(withQuestion.Questions[0].Forecasts) != 0 {
		t.Fatalf("question-only ledger = %#v", withQuestion)
	}
}

func TestBacklogQuestionAcceptsFirstV2Forecast(t *testing.T) {
	_, model := rootUpdateFixture(t, "question-without-forecasts.yaml")
	question := model.Questions[0]
	input := ForecastCreateInput{
		QuestionRevisionID: question.CurrentRevisionID,
		ForecastedAt:       "2026-09-22T09:00:00Z",
		Representations: []ledger.ForecastRepresentation{{Probability: &ledger.ProbabilityRepresentation{
			Kind: ledger.RepresentationProbability, Outcome: true, Probability: "0.5",
		}}},
	}
	mutation, err := BuildPublicForecastAppend(model, question.ID, "f-first", input, "2026-09-22T09:01:00Z")
	if err != nil {
		t.Fatalf("%#v", err)
	}
	forecast := mutation.Ledger.Questions[0].Forecasts[0]
	if forecast.QuestionRevisionID != question.CurrentRevisionID || forecast.SupersedesForecastID != nil {
		t.Fatalf("first forecast = %#v", forecast)
	}
}

func TestUnsupportedSchemaIsRejectedBeforeMutationSideEffects(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	unsupported := bytes.Replace(raw, []byte(`"schema_version": "2.2.0"`), []byte(`"schema_version": "2.1.0"`), 1)
	if bytes.Equal(unsupported, raw) {
		t.Fatal("unsupported-version fixture was not created")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, unsupported, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CommitPlatformAddFile(context.Background(), path, "local", PlatformCreateInput{Name: "Local", Kind: ledger.PlatformInformal})
	if app.ErrorCodeOf(err) != app.CodeUnsupportedSchemaVersion {
		t.Fatalf("error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, unsupported) {
		t.Fatalf("unsupported ledger changed: %v", err)
	}
	afterEntries, err := os.ReadDir(directory)
	if err != nil || len(afterEntries) != len(beforeEntries) {
		t.Fatalf("side-effect files were created: before=%v after=%v err=%v", beforeEntries, afterEntries, err)
	}
}
