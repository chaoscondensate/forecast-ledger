package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
)

type customRedactionShape struct{}

func (customRedactionShape) MarshalJSON() ([]byte, error) {
	return []byte(`{"public_name":"kept","nested":{"token":"CANARY-CUSTOM"}}`), nil
}

func TestStableJSONUsesCorrectStreamsAndRedactsSecrets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	presenter := New(&stdout, &stderr, Options{JSON: true})
	data := map[string]any{"question_id": "q-one", "revealed_key": "CANARY-KEY", "nested": map[string]any{"token": "CANARY-TOKEN", "key_hint": "safe-hint"}}
	if err := presenter.Success("forecast.shown", "Forecast found", data); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("success wrote stderr: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "CANARY") || !strings.Contains(stdout.String(), `"revealed_key":"[redacted]"`) || !strings.Contains(stdout.String(), `"key_hint":"safe-hint"`) {
		t.Fatalf("unsafe JSON success: %s", stdout.String())
	}

	stdout.Reset()
	err := app.WithDetails(app.NewError(app.CodeNotFound, "forecast not found", errors.New("private cause")), map[string]any{"key": "CANARY", "forecast_id": "f-one"})
	if writeErr := presenter.Failure(err); writeErr != nil {
		t.Fatal(writeErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure wrote stdout: %q", stdout.String())
	}
	var envelope ErrorEnvelope
	if decodeErr := json.Unmarshal(stderr.Bytes(), &envelope); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if envelope.Code != app.CodeNotFound || envelope.Details["key"] != "[redacted]" || strings.Contains(stderr.String(), "private cause") {
		t.Fatalf("unexpected JSON failure: %#v raw=%s", envelope, stderr.String())
	}
}

func TestNonTTYPlainQuietVerboseAndNoColor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	notTTY, isTTY := false, true
	presenter := New(&stdout, &stderr, Options{StdoutTTY: &notTTY, StderrTTY: &notTTY})
	if presenter.Mode() != ModePlain || presenter.ColorEnabled() {
		t.Fatalf("non-TTY mode=%s color=%v", presenter.Mode(), presenter.ColorEnabled())
	}
	_ = presenter.Success("ok", "done", nil)
	_ = presenter.Verbose("details")
	if stdout.String() != "done\n" || stderr.Len() != 0 {
		t.Fatalf("plain stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	presenter = New(&stdout, &stderr, Options{Quiet: true, Verbose: true, StdoutTTY: &isTTY, StderrTTY: &isTTY})
	_ = presenter.Success("ok", "done", nil)
	_ = presenter.Verbose("details")
	if stdout.Len() != 0 || stderr.String() != "details\n" {
		t.Fatalf("quiet/verbose stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	presenter = New(&stdout, &stderr, Options{StdoutTTY: &isTTY, LookupEnv: func(name string) (string, bool) { return "", name == "NO_COLOR" }})
	if presenter.ColorEnabled() {
		t.Fatal("NO_COLOR was ignored")
	}

	presenter = New(&stdout, &stderr, Options{StdoutTTY: &isTTY, LookupEnv: func(name string) (string, bool) {
		if name == "TERM" {
			return "dumb", true
		}
		return "", false
	}})
	if presenter.ColorEnabled() {
		t.Fatal("TERM=dumb was ignored")
	}

	presenter = New(&stdout, &stderr, Options{NoColor: true, StdoutTTY: &isTTY})
	if presenter.ColorEnabled() {
		t.Fatal("NoColor option was ignored")
	}
}

func TestRedactUsesPublicTaggedUnionJSONShape(t *testing.T) {
	data := struct {
		Representations []ledger.ForecastRepresentation `json:"representations"`
		Integrity       ledger.Integrity                `json:"integrity"`
	}{
		Representations: []ledger.ForecastRepresentation{{Probability: &ledger.ProbabilityRepresentation{Kind: ledger.RepresentationProbability, Outcome: true, Probability: "0.625"}}},
		Integrity: ledger.Integrity{Pending: &ledger.PendingIntegrity{
			Status: ledger.IntegrityPending,
			Target: ledger.ForecastTarget{Scope: "forecast-envelope/v3", Canonicalization: "RFC8785", ArtifactPath: "proofs/targets/f-one.json", Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(strings.Repeat("a", 64))}},
		}},
	}
	redacted, err := Redact(data)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"Probability", "PMF", "BinnedPMF", "Quantiles", "CDF", "Point", "CredibleIntervals", "Unanchored", "Pending", "Verified", "Failed"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("internal union branch %q leaked in %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"kind":"probability"`) || !strings.Contains(text, `"status":"pending"`) {
		t.Fatalf("public union shape missing from %s", text)
	}
}

func TestRedactPreservesEncodingJSONShapeAndNumbers(t *testing.T) {
	data := struct {
		service.TimestampArtifactResult
		Verification service.VerificationLayer `json:"verification"`
		Empty        []string                  `json:"empty,omitempty"`
		StringNumber int64                     `json:"string_number,string"`
		Large        int64                     `json:"large"`
		Ignored      string                    `json:"-"`
		Custom       customRedactionShape      `json:"custom"`
		Nested       map[string]any            `json:"nested"`
	}{
		TimestampArtifactResult: service.TimestampArtifactResult{QuestionID: "q-one", ForecastID: "f-one", State: service.TimestampVerified},
		Verification:            service.VerificationLayer{Name: "existence_timing", State: service.LayerPass},
		Empty:                   []string{},
		StringNumber:            42,
		Large:                   9_007_199_254_740_993,
		Ignored:                 "CANARY-IGNORED",
		Custom:                  customRedactionShape{},
		Nested:                  map[string]any{"api_key": "CANARY-NESTED", "safe": "kept"},
	}
	redacted, err := Redact(data)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"TimestampArtifactResult", `"empty"`, "CANARY", "Ignored"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden %q in %s", forbidden, text)
		}
	}
	for _, expected := range []string{`"question_id":"q-one"`, `"forecast_id":"f-one"`, `"verification":{"name":"existence_timing","state":"pass"}`, `"string_number":"42"`, `"large":9007199254740993`, `"token":"[redacted]"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
	byteMap, err := Redact(map[string]any{"bytes": []byte("CANARY-BYTES")})
	if err != nil {
		t.Fatal(err)
	}
	byteJSON, err := json.Marshal(byteMap)
	if err != nil || string(byteJSON) != `{"bytes":"[redacted bytes]"}` {
		t.Fatalf("byte redaction = %s, %v", byteJSON, err)
	}
}
