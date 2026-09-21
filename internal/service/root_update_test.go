package service

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

func TestBuildRootMetadataUpdateUsesMinimalPatchesAndPreservesImmutableCollections(t *testing.T) {
	doc, model := rootUpdateFixture(t, "individual-ledger.json")
	website := "https://example.net/forecasts"
	contact := ledger.Contact{Website: &website}
	input := RootMetadataPatchInput{
		Title:           Optional[string]{Set: true, Value: "Updated title"},
		Description:     Optional[string]{Set: true, Null: true},
		DefaultTimezone: Optional[string]{Set: true, Value: "UTC"},
		Forecaster: Optional[ForecasterMetadataPatchInput]{Set: true, Value: ForecasterMetadataPatchInput{
			Contact: Optional[ledger.Contact]{Set: true, Value: contact},
		}},
	}
	update, err := BuildRootMetadataUpdate(model, input)
	if err != nil {
		t.Fatal(err)
	}
	if update.Ledger.LedgerID != model.LedgerID || len(update.Ledger.Questions) != len(model.Questions) || update.Ledger.Publication.RepositoryURL != model.Publication.RepositoryURL {
		t.Fatalf("immutable data changed: %#v", update.Ledger)
	}
	patched, err := document.ApplyPatch(doc, update.Patches)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patched), `"description":`) || !strings.Contains(string(patched), `"title": "Updated title"`) {
		t.Fatalf("metadata patches missing:\n%s", patched)
	}
	originalQuestions := doc.Raw[bytes.Index(doc.Raw, []byte(`  "questions":`)):]
	patchedQuestions := patched[bytes.Index(patched, []byte(`  "questions":`)):]
	if !bytes.Equal(originalQuestions, patchedQuestions) {
		t.Fatal("question and forecast bytes changed during root metadata update")
	}
	parsed, err := document.ParseJSON(bytes.NewReader(patched), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLedgerDocument(parsed, nil); err != nil {
		t.Fatal(err)
	}
	if len(update.Warnings) != 2 {
		t.Fatalf("warnings = %#v", update.Warnings)
	}
}

func TestBuildRootMetadataUpdateRequiresAtomicForecasterShapeTransitions(t *testing.T) {
	_, individual := rootUpdateFixture(t, "individual-ledger.json")
	_, team := rootUpdateFixture(t, "team-ledger.yaml")
	members := []ledger.Member{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}
	validToTeam := RootMetadataPatchInput{Forecaster: Optional[ForecasterMetadataPatchInput]{Set: true, Value: ForecasterMetadataPatchInput{
		Kind:    Optional[ledger.ForecasterKind]{Set: true, Value: ledger.ForecasterTeam},
		Members: Optional[[]ledger.Member]{Set: true, Value: members},
	}}}
	updated, err := BuildRootMetadataUpdate(individual, validToTeam)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ledger.Forecaster.Kind != ledger.ForecasterTeam || updated.Ledger.Forecaster.Members == nil || len(*updated.Ledger.Forecaster.Members) != 2 {
		t.Fatalf("team transition = %#v", updated.Ledger.Forecaster)
	}
	invalidToTeam := validToTeam
	invalidToTeam.Forecaster.Value.Members = Optional[[]ledger.Member]{}
	if _, err := BuildRootMetadataUpdate(individual, invalidToTeam); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("non-atomic team transition error = %v", err)
	}
	validToIndividual := RootMetadataPatchInput{Forecaster: Optional[ForecasterMetadataPatchInput]{Set: true, Value: ForecasterMetadataPatchInput{
		Kind:    Optional[ledger.ForecasterKind]{Set: true, Value: ledger.ForecasterIndividual},
		Members: Optional[[]ledger.Member]{Set: true, Null: true},
	}}}
	updated, err = BuildRootMetadataUpdate(team, validToIndividual)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ledger.Forecaster.Kind != ledger.ForecasterIndividual || updated.Ledger.Forecaster.Members != nil {
		t.Fatalf("individual transition = %#v", updated.Ledger.Forecaster)
	}
}

func TestRootMetadataInputSchemaRejectsImmutableFields(t *testing.T) {
	var input RootMetadataPatchInput
	err := DecodeOperationInput(nil, "-", strings.NewReader(`{"ledger_id":"other"}`), InputSchemaRootMetadata, &input)
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("immutable field error = %v", err)
	}
}

func rootUpdateFixture(t *testing.T, name string) (*document.Document, *ledger.Ledger) {
	t.Helper()
	raw, err := fs.ReadFile(contractschema.ValidExamples(), name)
	if err != nil {
		t.Fatal(err)
	}
	var parsed *document.Document
	if strings.HasSuffix(name, ".json") {
		parsed, err = document.ParseJSON(bytes.NewReader(raw), document.DefaultLimits)
	} else {
		parsed, err = document.ParseYAML(bytes.NewReader(raw), document.DefaultLimits)
	}
	if err != nil {
		t.Fatal(err)
	}
	model, err := validation.DecodeLedger(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return parsed, model
}
