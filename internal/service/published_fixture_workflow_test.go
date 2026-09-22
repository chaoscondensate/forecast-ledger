package service

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestEveryPublishedV2LedgerFixtureRunsThroughApplicableWorkflows(t *testing.T) {
	for _, fixture := range []struct {
		relative               string
		publicationFailureCode app.ErrorCode
	}{
		{relative: "examples/valid/empty-ledger.json"},
		{relative: "examples/valid/individual-ledger.json"},
		{relative: "examples/valid/question-without-forecasts.yaml"},
		{relative: "examples/valid/team-ledger.yaml", publicationFailureCode: app.CodeConflict},
		{relative: "tests/conformance/valid/relationships-and-datetime.json"},
		{relative: "tests/conformance/valid/revealed-representation-only.json", publicationFailureCode: app.CodeConflict},
	} {
		t.Run(filepath.Base(fixture.relative), func(t *testing.T) {
			raw, err := fs.ReadFile(contractschema.Conformance(), fixture.relative)
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			ledgerPath := filepath.Join(directory, "ledger"+filepath.Ext(fixture.relative))
			if err := os.WriteFile(ledgerPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadAndValidateLedger(t.Context(), ledgerPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			targets, err := BuildAllForecastTargets(loaded.Model)
			if err != nil {
				t.Fatal(err)
			}
			if len(targets) != ledger.SummaryCounts(loaded.Model).Forecasts {
				t.Fatalf("target count = %d, forecast count = %d", len(targets), ledger.SummaryCounts(loaded.Model).Forecasts)
			}
			packageRoot := filepath.Join(directory, "package")
			if _, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false); err != nil {
				if app.ErrorCodeOf(err) == fixture.publicationFailureCode {
					return
				}
				t.Fatal(err)
			}
			if fixture.publicationFailureCode != "" {
				t.Fatalf("publication succeeded; want %s", fixture.publicationFailureCode)
			}
			if _, err := VerifyPublicationPackage(t.Context(), filepath.Join(packageRoot, "ledger", filepath.Base(ledgerPath)), filepath.Join(packageRoot, "manifest.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
