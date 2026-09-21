package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

func TestLedgerTransactionRunsCompleteProspectiveValidation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	original, err := fs.ReadFile(ledgerschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	validator := func(parsed *document.Document) error {
		return ValidateLedgerDocument(parsed, os.DirFS(directory))
	}
	err = storage.UpdateLedger(context.Background(), path, storage.TransactionOptions{
		Validate: validator,
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/default_timezone", Value: "Mars/Olympus"}})
		},
	})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("semantic-invalid prospective ledger error = %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatal("invalid prospective mutation changed ledger bytes")
	}
}
