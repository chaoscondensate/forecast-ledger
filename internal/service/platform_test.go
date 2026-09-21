package service

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestPlatformValidationIsSharedAcrossCreateAndUpdate(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	badURL := "relative/path"
	badAccount := ledger.PlatformAccount{}
	for index, input := range []PlatformCreateInput{
		{Name: "", Kind: ledger.PlatformInformal},
		{Name: "Bad kind", Kind: "other"},
		{Name: "Bad URL", Kind: ledger.PlatformInformal, URL: &badURL},
		{Name: "Empty account", Kind: ledger.PlatformInformal, Account: &badAccount},
	} {
		if _, err := BuildPlatformAdd(model, ledger.Slug("new-platform"), input); app.ErrorCodeOf(err) != app.CodeInvalidData {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
	if _, err := BuildPlatformAdd(model, "Bad ID", PlatformCreateInput{Name: "Valid", Kind: ledger.PlatformInformal}); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("bad ID error = %v", err)
	}
}

func TestPlatformAddUpdateListShowAndRemoveBusinessRules(t *testing.T) {
	doc, model := rootUpdateFixture(t, "individual-ledger.json")
	url := "https://example.net/forecasts"
	added, err := BuildPlatformAdd(model, "new-platform", PlatformCreateInput{Name: "New", Kind: ledger.PlatformInformal, URL: &url})
	if err != nil {
		t.Fatal(err)
	}
	patched, err := document.ApplyPatch(doc, added.Patches)
	if err != nil {
		t.Fatal(err)
	}
	beforeQuestions := doc.Raw[bytes.Index(doc.Raw, []byte(`  "questions":`)):]
	afterQuestions := patched[bytes.Index(patched, []byte(`  "questions":`)):]
	if !bytes.Equal(beforeQuestions, afterQuestions) {
		t.Fatal("platform add rewrote questions")
	}
	if _, err := BuildPlatformAdd(model, "metaculus", PlatformCreateInput{Name: "Duplicate", Kind: ledger.PlatformInformal}); app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("duplicate platform error = %v", err)
	}

	newName := Optional[string]{Set: true, Value: "Updated Metaculus"}
	updated, err := BuildPlatformUpdate(model, "metaculus", PlatformPatchInput{Name: newName, URL: Optional[string]{Set: true, Null: true}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Ledger.Platforms["metaculus"].Name != "Updated Metaculus" || updated.Ledger.Platforms["metaculus"].URL != nil {
		t.Fatalf("updated platform = %#v", updated.Ledger.Platforms["metaculus"])
	}
	if _, err := BuildPlatformUpdate(model, "missing", PlatformPatchInput{Name: newName}); app.ErrorCodeOf(err) != app.CodeNotFound {
		t.Fatalf("missing platform update error = %v", err)
	}

	items, err := ListPlatforms(model)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "metaculus" || items[0].ReferenceCount == 0 {
		t.Fatalf("platform list = %#v", items)
	}
	shown, err := ShowPlatform(model, "metaculus")
	if err != nil {
		t.Fatal(err)
	}
	if len(shown.ReferencingQuestionIDs) == 0 || !reflect.DeepEqual(shown.ReferencingQuestionIDs, append([]ledger.Slug(nil), shown.ReferencingQuestionIDs...)) {
		t.Fatalf("platform show = %#v", shown)
	}
	if _, err := BuildPlatformRemove(model, "metaculus"); app.ErrorCodeOf(err) != app.CodeConflict || !strings.Contains(err.Error(), "referenced") {
		t.Fatalf("referenced remove error = %v", err)
	}
	removed, err := BuildPlatformRemove(added.Ledger, "new-platform")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := removed.Ledger.Platforms["new-platform"]; exists {
		t.Fatal("unreferenced platform was not removed")
	}
}

func TestPlatformFileMutationsAndStdinReads(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CommitPlatformAddFile(context.Background(), path, "new-platform", PlatformCreateInput{Name: "New", Kind: ledger.PlatformInternal})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !reflect.DeepEqual(result.ChangedPointers, []string{"/platforms/new-platform"}) {
		t.Fatalf("add result = %#v", result)
	}
	updatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ledgerID, list, err := LoadPlatformList(context.Background(), "-", bytes.NewReader(updatedBytes))
	if err != nil {
		t.Fatal(err)
	}
	if ledgerID == "" || len(list) != 2 || list[0].ID != "metaculus" || list[1].ID != "new-platform" {
		t.Fatalf("stdin list = %q %#v", ledgerID, list)
	}
	_, shown, err := LoadPlatformShow(context.Background(), "-", bytes.NewReader(updatedBytes), "new-platform")
	if err != nil || shown.Platform.Name != "New" {
		t.Fatalf("stdin show = %#v, %v", shown, err)
	}
	plan, err := PlanPlatformRemoveFile(context.Background(), path, "new-platform")
	if err != nil || !plan.Changed {
		t.Fatalf("remove plan = %#v, %v", plan, err)
	}
	afterPlan, _ := os.ReadFile(path)
	if !bytes.Equal(afterPlan, updatedBytes) {
		t.Fatal("platform remove plan changed file")
	}
}
