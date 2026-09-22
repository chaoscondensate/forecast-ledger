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
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

func TestLoadRejectsMissingUnknownAndFutureSchemaVersionsFirst(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing", content: `{"ledger_id":"example"}`},
		{name: "wrong type", content: `{"schema_version":1}`},
		{name: "old", content: `{"schema_version":"1.0.0"}`},
		{name: "superseded v2", content: `{"schema_version":"2.0.1"}`},
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
			if !ok || applicationErr.Details["supported_schema_version"] != "2.1.0" {
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
	unsupported := bytes.Replace(raw, []byte(`"schema_version": "2.1.0"`), []byte(`"schema_version": "2.0.1"`), 1)
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

func TestSupersededV2LifecycleEvidenceRejectsBeforeEffects(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	old := []byte(`{
  "schema_version": "2.0.1",
  "ledger_id": "old-evidence",
  "questions": [{
    "id": "q-old",
    "forecasts": [{
      "id": "f-old",
      "lifecycle_events": [{"id":"event-old","type":"withdrawn","effective_at":"2026-01-02T00:00:00Z","recorded_at":"2026-01-02T00:00:01Z"}],
      "integrity": {"status":"verified","target":{"scope":"forecast-envelope/v2","canonicalization":"RFC8785","artifact_path":"proofs/targets/f-old.json","digest":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"timestamps":[]}
    }]
  }]
}`)
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}

	assertUnsupported := func(name string, err error) {
		t.Helper()
		if app.ErrorCodeOf(err) != app.CodeUnsupportedSchemaVersion {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	_, err := CommitTargetBuild(t.Context(), path, false, "q-old", "f-old")
	assertUnsupported("target build", err)
	keyPath := filepath.Join(directory, "new.key")
	_, err = CommitForecastSealFile(t.Context(), path, keyPath, "q-old", "f-new", SealedForecastInput{}, "2026-01-03T00:00:00Z", Effects{Random: failingRandom{}})
	assertUnsupported("forecast seal", err)
	transport := &countingRoundTripper{}
	_, err = CommitTimestampStamp(t.Context(), path, "q-old", "f-old", TimestampStampOptions{TSAURL: "https://tsa.example.test", CABundlePath: "missing.pem", Effects: Effects{Random: failingRandom{}}, HTTPClient: testTimestampHTTPClient(transport)})
	assertUnsupported("timestamp stamp", err)
	output := filepath.Join(directory, "package")
	_, err = CommitPublicationBuild(t.Context(), path, output, false)
	assertUnsupported("publication build", err)

	if transport.requests != 0 {
		t.Fatalf("unsupported v2.0.1 contacted the network %d times", transport.requests)
	}
	for _, forbidden := range []string{storage.LedgerLockPath(path), keyPath, filepath.Join(directory, "proofs"), output} {
		if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
			t.Fatalf("unsupported v2.0.1 created %s: %v", filepath.Base(forbidden), err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, old) {
		t.Fatalf("unsupported v2.0.1 ledger changed: %v", err)
	}
}
