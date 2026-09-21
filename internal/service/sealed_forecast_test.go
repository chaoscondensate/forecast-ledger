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

func TestForecastSealRevealAndKeyHintPreserveTargetBytes(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	input := sealedInitialQuestion()
	built, err := BuildInitialSealedLedger(context.Background(), root, input, Effects{Clock: fixedTestClock{}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := BuildForecastTarget(built.Ledger, "q-one", "f-one")
	if err != nil {
		t.Fatal(err)
	}
	revealed, err := BuildForecastReveal(built.Ledger, "q-one", "f-one", built.KeyFile, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	forecast := revealed.Ledger.Questions[0].Forecasts[0]
	if forecast.Visibility != ledger.VisibilityRevealed || forecast.Representations == nil || forecast.Rationale == nil || forecast.Commitment.Revealed == nil {
		t.Fatalf("revealed forecast = %#v", forecast)
	}
	after, err := BuildForecastTarget(revealed.Ledger, "q-one", "f-one")
	if err != nil || !bytes.Equal(before.Bytes, after.Bytes) {
		t.Fatalf("reveal changed target: %v", err)
	}
	hint, err := BuildForecastKeyHintUpdate(revealed.Ledger, "q-one", "f-one", "vault:item-42")
	if err != nil {
		t.Fatal(err)
	}
	afterHint, err := BuildForecastTarget(hint.Ledger, "q-one", "f-one")
	if err != nil || !bytes.Equal(before.Bytes, afterHint.Bytes) {
		t.Fatalf("key hint changed target: %v", err)
	}
}

func TestForecastRevealRejectsWrongRevisionBoundKeyWithoutMutation(t *testing.T) {
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	built, err := BuildInitialSealedLedger(context.Background(), root, sealedInitialQuestion(), Effects{Clock: fixedTestClock{}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))}})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := forecastcrypto.EncodeKeyFile("q-one", "qr-wrong", "f-one", bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildForecastReveal(built.Ledger, "q-one", "f-one", wrong, "2026-02-01T00:00:00Z"); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("wrong key error = %v", err)
	}
	if built.Ledger.Questions[0].Forecasts[0].Visibility != ledger.VisibilitySealed {
		t.Fatal("failed reveal mutated source ledger")
	}
}

func sealedInitialQuestion() InitialQuestionInput {
	input := binaryInitialQuestion()
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	rationale, comment := "private rationale", "private comment"
	factors := []string{"base rate"}
	input.InitialForecast.Rationale = &rationale
	input.InitialForecast.KeyFactors = &factors
	input.InitialForecast.Comment = &comment
	return input
}
