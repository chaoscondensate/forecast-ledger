package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

type fixedCLIClock struct{ value time.Time }

func (clock fixedCLIClock) Now() time.Time { return clock.value }

func TestNormalizeCLIAuthoredTimeAcceptedForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, raw, zone, want string
		policy                dateOnlyPolicy
	}{
		{"rfc3339 offset", "2030-08-10T14:05:06.123+05:30", "Europe/London", "2030-08-10T14:05:06.123+05:30", dateOnlyRejected},
		{"rfc3339 negative offset", "2030-08-10T14:05:06-07:00", "Europe/London", "2030-08-10T14:05:06-07:00", dateOnlyRejected},
		{"ISO local minute", "2030-08-10 14:05", "Europe/London", "2030-08-10T14:05:00+01:00", dateOnlyRejected},
		{"English day first", "10 Aug 2030 14:05", "Europe/London", "2030-08-10T14:05:00+01:00", dateOnlyRejected},
		{"English month first", "August 10 2030 14:05:06", "Europe/London", "2030-08-10T14:05:06+01:00", dateOnlyRejected},
		{"date start", "10 Aug 2030", "Europe/London", "2030-08-10T00:00:00+01:00", dateOnlyStart},
		{"date end", "10 Aug 2030", "Europe/London", "2030-08-10T23:59:59+01:00", dateOnlyEnd},
		{"leap day", "29 Feb 2032", "UTC", "2032-02-29T00:00:00Z", dateOnlyStart},
		{"DST start date end", "29 Mar 2026", "Europe/London", "2026-03-29T23:59:59+01:00", dateOnlyEnd},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeCLIAuthoredTime(test.raw, test.zone, "forecasted_at", test.policy)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestHumanTimeNormalizationAppearsInStructuredSuccessAndDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "init", "--file", path, "--ledger-id", "normalized-times", "--timezone", "Europe/London", "--forecaster-id", "owner", "--forecaster-name", "Owner", "--created-at", "30 Aug 2026 12:00")
	if code != 0 {
		t.Fatalf("init code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{`"normalized_times"`, `"field":"created_at"`, `"raw":"30 Aug 2026 12:00"`, `"normalized":"2026-08-30T12:00:00+01:00"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("success output lacks %s: %s", want, stdout)
		}
	}

	code, stdout, stderr = runCLI("forecast-ledger", "--json", "question", "add", "--dry-run", "--file", path, "--question", "q-dry", "--revision-id", "qr-dry", "--title", "Question", "--resolution-criteria", "Public result", "--expected-resolution-at", "10 Aug 2030", "--outcome-kind", "binary")
	if code != 0 {
		t.Fatalf("dry-run code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{`"field":"expected_resolution_at"`, `"raw":"10 Aug 2030"`, `"normalized":"2030-08-10T23:59:59+01:00"`, `"date_policy":"end_of_day"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("dry-run output lacks %s: %s", want, stdout)
		}
	}
}

func TestCLIForecastDefaultsUseOneLedgerTimezoneObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	effects := service.ProductionEffects()
	effects.Clock = fixedCLIClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	commands := [][]string{
		{"forecast-ledger", "init", "--file", path, "--ledger-id", "clock", "--timezone", "Europe/London", "--forecaster-id", "owner", "--forecaster-name", "Owner"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-clock", "--revision-id", "qr-clock", "--title", "Question", "--resolution-criteria", "Public result", "--expected-resolution-at", "10 Aug 2030", "--outcome-kind", "binary"},
		{"forecast-ledger", "forecast", "add", "--file", path, "--question", "q-clock", "--forecast", "f-clock", "--question-revision", "qr-clock", "--probability", "0.5", "--probability-outcome"},
	}
	for _, arguments := range commands {
		code, _, stderr := runCLIWithEffects(effects, arguments...)
		if code != 0 {
			t.Fatalf("%v failed with %d: %s", arguments[1:3], code, stderr)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(raw), `"2026-08-30T18:00:00+01:00"`); count < 2 {
		t.Fatalf("forecast defaults did not share the fixed ledger-timezone observation:\n%s", raw)
	}
}

func TestCLIQuestionRevisionDefaultsAdvanceOnSameClockTick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	effects := service.ProductionEffects()
	effects.Clock = fixedCLIClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	commands := [][]string{
		{"forecast-ledger", "init", "--file", path, "--ledger-id", "revision-clock", "--timezone", "Europe/London", "--forecaster-id", "owner", "--forecaster-name", "Owner"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-clock", "--revision-id", "qr-one", "--title", "Question", "--resolution-criteria", "Public result", "--expected-resolution-at", "2030-08-10T23:59:59+01:00", "--outcome-kind", "binary"},
		{"forecast-ledger", "question", "revise", "--file", path, "--question", "q-clock", "--revision-id", "qr-two", "--title", "Revised question", "--resolution-criteria", "Public result", "--expected-resolution-at", "2030-08-10T23:59:59+01:00", "--outcome-kind", "binary"},
	}
	for _, arguments := range commands {
		code, _, stderr := runCLIWithEffects(effects, arguments...)
		if code != 0 {
			t.Fatalf("%v failed with %d: %s", arguments[1:3], code, stderr)
		}
	}
	loaded, err := service.LoadAndValidateLedger(t.Context(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	revisions := loaded.Model.Questions[0].Revisions
	if len(revisions) != 2 || revisions[0].EffectiveAt != "2026-08-30T18:00:00+01:00" || revisions[1].EffectiveAt != "2026-08-30T18:00:00.000000001+01:00" || revisions[1].RecordedAt != revisions[1].EffectiveAt {
		t.Fatalf("CLI revision defaults = %#v", revisions)
	}
}

func TestCLIDefaultRevealAndTerminalTimesUseLedgerTimezone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	keyPath := filepath.Join(filepath.Dir(path), "forecast.key")
	secretPath := filepath.Join(filepath.Dir(path), "private.yaml")
	if err := storage.CreateProtectedFile(secretPath, []byte("representations:\n  - kind: probability\n    outcome: true\n    probability: \"0.7\"\nrationale: private rationale\nkey_factors:\n  - factor\ncomment: private comment\n")); err != nil {
		t.Fatal(err)
	}
	effects := service.ProductionEffects()
	effects.Clock = fixedCLIClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	commands := [][]string{
		{"forecast-ledger", "init", "--file", path, "--ledger-id", "timezone", "--timezone", "Europe/London", "--forecaster-id", "owner", "--forecaster-name", "Owner", "--created-at", "2026-01-01T00:00:00Z"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-one", "--revision-id", "qr-one", "--effective-at", "2026-01-01T00:00:00Z", "--revision-recorded-at", "2026-01-01T00:00:01Z", "--title", "Question", "--resolution-criteria", "Public result", "--expected-resolution-at", "2030-01-01T00:00:00Z", "--outcome-kind", "binary"},
		{"forecast-ledger", "forecast", "seal", "--file", path, "--question", "q-one", "--forecast", "f-one", "--question-revision", "qr-one", "--forecasted-at", "2026-02-01T00:00:00Z", "--recorded-at", "2026-02-01T00:00:01Z", "--secret-input", secretPath, "--key-file", keyPath},
		{"forecast-ledger", "forecast", "reveal", "--file", path, "--question", "q-one", "--forecast", "f-one", "--key-file", keyPath, "--yes"},
		{"forecast-ledger", "question", "update", "--file", path, "--question", "q-one", "--status", "closed"},
		{"forecast-ledger", "question", "void", "--file", path, "--question", "q-one", "--reason", "No public outcome", "--yes"},
	}
	for _, arguments := range commands {
		code, _, stderr := runCLIWithEffects(effects, arguments...)
		if code != 0 {
			t.Fatalf("%v failed with %d: %s", arguments[1:3], code, stderr)
		}
	}
	loaded, err := service.LoadAndValidateLedger(t.Context(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	forecast := loaded.Model.Questions[0].Forecasts[0]
	if forecast.Commitment == nil || forecast.Commitment.Revealed == nil || forecast.Commitment.Revealed.RevealedAt != ledger.Timestamp("2026-08-30T18:00:00+01:00") {
		t.Fatalf("CLI revealed_at = %#v", forecast.Commitment)
	}
	resolution := loaded.Model.Questions[0].Resolution
	if resolution == nil || resolution.Unresolved == nil || resolution.Unresolved.RecordedAt != "2026-08-30T18:00:00+01:00" {
		t.Fatalf("CLI terminal recorded_at = %#v", resolution)
	}
}

func runCLIWithEffects(effects service.Effects, arguments ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	command := newCommandWithEffects(strings.NewReader(""), &stdout, &stderr, effects)
	err := command.Run(context.Background(), arguments)
	if err != nil {
		return app.ExitCodeOf(err), stdout.String(), stderr.String() + err.Error()
	}
	return 0, stdout.String(), stderr.String()
}

func TestNormalizeCLIAuthoredTimeRejectsUnboundedOrAmbiguousForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, raw, zone, contains string
		policy                    dateOnlyPolicy
	}{
		{"relative", "tomorrow", "UTC", "must be RFC 3339", dateOnlyStart},
		{"slash order", "08/10/2030", "UTC", "must be RFC 3339", dateOnlyStart},
		{"two digit year", "10 Aug 30", "UTC", "must be RFC 3339", dateOnlyStart},
		{"timezone abbreviation", "10 Aug 2030 14:05 BST", "Europe/London", "must be RFC 3339", dateOnlyRejected},
		{"invalid leap day", "29 Feb 2030", "UTC", "must be RFC 3339", dateOnlyStart},
		{"unsupported precision", "2030-08-10T14:05:06.1234567890Z", "UTC", "must be RFC 3339", dateOnlyRejected},
		{"date where time required", "10 Aug 2030", "UTC", "requires a time", dateOnlyRejected},
		{"London gap", "2026-03-29 01:30", "Europe/London", "skipped or repeated", dateOnlyRejected},
		{"London fold", "2026-10-25 01:30", "Europe/London", "skipped or repeated", dateOnlyRejected},
		{"Lord Howe gap", "2026-10-04 02:15", "Australia/Lord_Howe", "skipped or repeated", dateOnlyRejected},
		{"Lord Howe fold", "2026-04-05 01:45", "Australia/Lord_Howe", "skipped or repeated", dateOnlyRejected},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeCLIAuthoredTime(test.raw, test.zone, "forecasted_at", test.policy)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error %v does not contain %q", err, test.contains)
			}
		})
	}
}

func TestNormalizeCLIAuthoredTimeDoesNotUseHostTimezone(t *testing.T) {
	original := time.Local
	time.Local = time.FixedZone("host-test", 9*60*60)
	t.Cleanup(func() { time.Local = original })

	got, err := normalizeCLIAuthoredTime("10 Aug 2030 14:05", "America/New_York", "forecasted_at", dateOnlyRejected)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "2030-08-10T14:05:00-04:00" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeCLIAuthoredDatesRoundTrip(t *testing.T) {
	t.Parallel()
	for month := time.January; month <= time.December; month++ {
		for day := 1; day <= 28; day++ {
			raw := time.Date(2032, month, day, 0, 0, 0, 0, time.UTC).Format("2 Jan 2006")
			got, err := normalizeCLIAuthoredTime(raw, "Pacific/Chatham", "opens_at", dateOnlyStart)
			if err != nil {
				t.Fatalf("%s: %v", raw, err)
			}
			parsed, err := time.Parse(time.RFC3339, string(got))
			if err != nil || parsed.In(mustLocation(t, "Pacific/Chatham")).Format("2 Jan 2006") != raw {
				t.Fatalf("%s round trip produced %q (%v)", raw, got, err)
			}
		}
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
