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
	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestLoadRejectsMissingUnknownAndFutureSchemaVersionsFirst(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing", content: `{"ledger_id":"example"}`},
		{name: "wrong type", content: `{"schema_version":1}`},
		{name: "old", content: `{"schema_version":"1.0.0"}`},
		{name: "future", content: `{"schema_version":"3.0.0"}`},
		{name: "unknown", content: `{"schema_version":"preview"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.json")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadAndValidateLedger(context.Background(), path, nil)
			if app.ErrorCodeOf(err) != app.CodeUnsupportedSchemaVersion || app.ExitCodeOf(err) != 3 {
				t.Fatalf("error = %#v, code=%q exit=%d", err, app.ErrorCodeOf(err), app.ExitCodeOf(err))
			}
			applicationErr, ok := err.(*app.Error)
			if !ok || applicationErr.Details["supported_schema_version"] != "2.0.0" {
				t.Fatalf("details = %#v", applicationErr)
			}
		})
	}
}

func TestMutatingOperationRejectsUnsupportedVersionBeforeWriting(t *testing.T) {
	raw, err := fs.ReadFile(ledgerschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	unsupported := bytes.Replace(raw, []byte(`"schema_version": "2.0.0"`), []byte(`"schema_version": "0.0.0"`), 1)
	if bytes.Equal(unsupported, raw) {
		t.Fatal("unsupported-version fixture was not created")
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, unsupported, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = CommitPlatformAddFile(context.Background(), path, "new-platform", PlatformCreateInput{Name: "New", Kind: ledger.PlatformInformal})
	if app.ErrorCodeOf(err) != app.CodeUnsupportedSchemaVersion {
		t.Fatalf("error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, unsupported) {
		t.Fatal("unsupported ledger changed")
	}
}
