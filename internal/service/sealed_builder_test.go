package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/forecastcrypto"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func TestBuildInitialSealedLedgerProducesValidRedactedForecastAndBoundKey(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{
		LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey",
	}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	input.InitialForecast.ForecastedAt = ""
	rationale, comment := "private rationale", "private comment"
	factors := []string{"base rate"}
	input.InitialForecast.Rationale = &rationale
	input.InitialForecast.KeyFactors = &factors
	input.InitialForecast.Comment = &comment
	entropy := bytes.Repeat([]byte{0x42}, 32+32+12)
	built, err := BuildInitialSealedLedger(context.Background(), root, input, Effects{
		Clock:  fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		Random: deterministicTestRandom{reader: bytes.NewReader(entropy)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProspectiveLedgerModel(built.Ledger); err != nil {
		t.Fatal(err)
	}
	forecast := built.Ledger.Questions[0].Forecasts[0]
	if forecast.Visibility != ledger.VisibilitySealed || forecast.Commitment == nil || forecast.Commitment.Sealed == nil {
		t.Fatalf("sealed forecast = %#v", forecast)
	}
	if forecast.Representations != nil || forecast.Rationale != nil || forecast.KeyFactors != nil || forecast.Comment != nil {
		t.Fatalf("sealed forecast leaked private mirror: %#v", forecast)
	}
	if forecast.ForecastedAt != root.CreatedAt || forecast.RecordedAt != root.CreatedAt {
		t.Fatalf("default times = %q, %q; want %q", forecast.ForecastedAt, forecast.RecordedAt, root.CreatedAt)
	}
	if forecast.Commitment.Sealed.KeyHint != "forecast-key:f-one" {
		t.Fatalf("key hint = %q", forecast.Commitment.Sealed.KeyHint)
	}
	if _, err := forecastcrypto.DecodeKeyFile(built.KeyFile, "q-one", "qr-one", "f-one", forecast.Commitment.Sealed.CommitmentHash.Value); err != nil {
		t.Fatal(err)
	}
}

func TestBuildInitialSealedLedgerRejectsMissingRepresentationsBeforeEntropy(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	input.InitialForecast.Representations = nil
	counting := &countingTestRandom{}
	_, err = BuildInitialSealedLedger(context.Background(), root, input, Effects{Clock: fixedTestClock{}, Random: counting})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("missing representations error = %v", err)
	}
	if counting.calls != 0 {
		t.Fatalf("entropy calls = %d", counting.calls)
	}
}

func TestOmittedSealedForecastTimeBeforeOpeningFailsBeforeEntropy(t *testing.T) {
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey"}, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	opensAt := ledger.Timestamp("2026-02-01T00:00:00Z")
	input.Revision.ForecastingOpensAt = &opensAt
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	input.InitialForecast.ForecastedAt = ""
	rationale, comment := "private rationale", "private comment"
	factors := []string{"base rate"}
	input.InitialForecast.Rationale = &rationale
	input.InitialForecast.KeyFactors = &factors
	input.InitialForecast.Comment = &comment
	counting := &countingTestRandom{}
	_, err = BuildInitialSealedLedgerAt(context.Background(), root, input, "2026-01-15T00:00:00Z", Effects{Clock: fixedTestClock{}, Random: counting})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("before-open error = %v", err)
	}
	if counting.calls != 0 {
		t.Fatalf("entropy calls = %d", counting.calls)
	}
}

type countingTestRandom struct{ calls int }

func (random *countingTestRandom) ReadFull(_ context.Context, destination []byte) error {
	random.calls++
	for index := range destination {
		destination[index] = byte(index)
	}
	return nil
}
