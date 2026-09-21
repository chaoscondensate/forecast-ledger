package service

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

func TestCommitRootMetadataFileUpdatePreservesUnrelatedYAMLBytes(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "team-ledger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	input := RootMetadataPatchInput{
		Title: Optional[string]{Set: true, Value: "Updated team ledger"},
		Forecaster: Optional[ForecasterMetadataPatchInput]{Set: true, Value: ForecasterMetadataPatchInput{
			Name: Optional[string]{Set: true, Value: "Current Team Name"},
		}},
	}
	result, err := CommitRootMetadataFileUpdate(context.Background(), path, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.ChangedPointers) != 2 || result.BeforeSHA256 == result.AfterSHA256 {
		t.Fatalf("result = %#v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeQuestions := raw[bytes.Index(raw, []byte("questions:\n")):]
	afterQuestions := after[bytes.Index(after, []byte("questions:\n")):]
	if !bytes.Equal(beforeQuestions, afterQuestions) {
		t.Fatal("question bytes changed during YAML metadata update")
	}
	if !strings.HasPrefix(string(after), "# yaml-language-server:") || !strings.Contains(string(after), "title: Updated team ledger") || !strings.Contains(string(after), "name: Current Team Name") {
		t.Fatalf("YAML metadata update missing or presentation changed:\n%s", after)
	}
	loaded, err := LoadAndValidateLedger(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model.Forecaster.ID != ledger.Slug("example-research-team") || loaded.Model.Publication != nil && loaded.Model.Publication.History == "" {
		t.Fatalf("immutable metadata changed: %#v", loaded.Model)
	}
}

func TestServiceMutationAutomaticallyRecoversInterruptedLedgerWrite(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	err = storage.UpdateLedger(t.Context(), path, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, os.DirFS(directory)) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/title", Value: "Interrupted title"}})
		},
		Fault: func(stage storage.TransactionStage) error {
			if stage == storage.StageJournalSynced {
				return errors.New("simulated stop")
			}
			return nil
		},
	})
	if app.ErrorCodeOf(err) != app.CodeIO {
		t.Fatalf("fault error=%v", err)
	}
	result, err := CommitRootMetadataFileUpdate(t.Context(), path, RootMetadataPatchInput{Title: Optional[string]{Set: true, Value: "Retried title"}})
	if err != nil || !result.Changed {
		t.Fatalf("retry result=%+v err=%v", result, err)
	}
	loaded, err := LoadAndValidateLedger(t.Context(), path, nil)
	if err != nil || loaded.Model.Title == nil || *loaded.Model.Title != "Retried title" {
		t.Fatalf("recovered ledger title=%v err=%v", loaded.Model.Title, err)
	}
	if _, statErr := os.Stat(storage.JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("retry retained journal: %v", statErr)
	}
}

func TestPlanRootMetadataFileUpdateDoesNotWrite(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := PlanRootMetadataFileUpdate(context.Background(), path, RootMetadataPatchInput{DefaultTimezone: Optional[string]{Set: true, Value: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("planned change reported unchanged")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, raw) {
		t.Fatal("plan changed ledger bytes")
	}
}
