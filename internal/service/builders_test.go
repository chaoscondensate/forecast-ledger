package service

import (
	"strings"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func TestBuildLedgerRootCapturesOneClockAndBuildsValidIdentity(t *testing.T) {
	clock := fixedTestClock{value: time.Date(2026, 8, 26, 15, 4, 5, 900, time.FixedZone("BST", 3600))}
	root, err := BuildLedgerRoot(InitRootRequest{
		LedgerID: "research", Timezone: "Europe/London", ForecasterID: "andrey",
		ForecasterName: "Andrey", ForecasterKind: ledger.ForecasterIndividual,
		Input: InitInput{Platforms: map[ledger.Slug]ledger.Platform{"local": {Name: "Local", Kind: ledger.PlatformInformal}}},
	}, clock)
	if err != nil {
		t.Fatalf("%#v", err)
	}
	if root.SchemaVersion != "2.0.0" || root.CreatedAt != "2026-08-26T15:04:05+01:00" || root.Platforms["local"].Name != "Local" {
		t.Fatalf("root = %#v", root)
	}
	if root.Questions == nil || len(root.Questions) != 0 || root.Publication != nil {
		t.Fatalf("root collections/publication = %#v / %#v", root.Questions, root.Publication)
	}
}

func TestBuildLedgerRootRejectsInvalidTeamIdentityTimeAndPlatform(t *testing.T) {
	members := []ledger.Member{{ID: "same", Name: "One"}, {ID: "same", Name: "Two"}}
	tests := []InitRootRequest{
		{LedgerID: "Bad ID", Timezone: "Europe/London", ForecasterID: "id", ForecasterName: "Name"},
		{LedgerID: "ok", Timezone: "Mars/Olympus", ForecasterID: "id", ForecasterName: "Name"},
		{LedgerID: "ok", Timezone: "UTC", ForecasterID: "id", ForecasterName: "Name", ForecasterKind: ledger.ForecasterTeam},
		{LedgerID: "ok", Timezone: "UTC", ForecasterID: "id", ForecasterName: "Name", ForecasterKind: ledger.ForecasterTeam, Input: InitInput{Members: &members}},
		{LedgerID: "ok", Timezone: "UTC", ForecasterID: "id", ForecasterName: "Name", Input: InitInput{Platforms: map[ledger.Slug]ledger.Platform{"p": {Name: "", Kind: ledger.PlatformInformal}}}},
	}
	for index, request := range tests {
		if _, err := BuildLedgerRoot(request, fixedTestClock{value: time.Now()}); app.ErrorCodeOf(err) != app.CodeInvalidData {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestTimestampAndChronologyRequireExactRFC3339(t *testing.T) {
	for _, value := range []ledger.Timestamp{"2026-08-26", "2026-08-26T12:00Z", "2026-02-30T12:00:00Z", "2026-08-26T12:00:00"} {
		if _, err := ParseTimestamp(value, "time"); app.ErrorCodeOf(err) != app.CodeInvalidData {
			t.Fatalf("timestamp %q error = %v", value, err)
		}
	}
	if err := ValidateChronology("2026-08-27T00:00:00Z", "later", "2026-08-26T00:00:00Z", "earlier", true); app.ErrorCodeOf(err) != app.CodeInvalidData || !strings.Contains(err.Error(), "must not be before") {
		t.Fatalf("reverse chronology error = %v", err)
	}
	if err := ValidateChronology("2026-08-26T00:00:00Z", "earlier", "2026-08-26T00:00:00Z", "later", true); err != nil {
		t.Fatalf("inclusive equality failed: %v", err)
	}
}

func TestInitDefaultsRecordedAtFromOperationClockNotExplicitCreatedAt(t *testing.T) {
	createdAt := ledger.Timestamp("2020-01-01T00:00:00Z")
	operationAt := ledger.Timestamp("2026-08-26T12:34:56Z")
	request := InitRootRequest{
		LedgerID: "research", Timezone: "UTC", ForecasterID: "andrey", ForecasterName: "Andrey",
		Input: InitInput{CreatedAt: &createdAt},
	}
	root, err := BuildLedgerRootAt(request, operationAt)
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	questionCreated := ledger.Timestamp("2020-01-01T00:00:00Z")
	input.CreatedAt = &questionCreated
	input.Revision.ExpectedResolutionAt = "2027-01-02T00:00:00Z"
	input.InitialForecast.ForecastedAt = operationAt
	input.InitialForecast.RecordedAt = nil
	model, err := BuildInitialPublicLedgerAt(root, input, operationAt)
	if err != nil {
		t.Fatalf("%#v", err)
	}
	if model.CreatedAt != createdAt || model.Questions[0].Forecasts[0].RecordedAt != operationAt {
		t.Fatalf("created_at=%s recorded_at=%s", model.CreatedAt, model.Questions[0].Forecasts[0].RecordedAt)
	}
}
