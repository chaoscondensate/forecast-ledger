package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/presentation"
)

func TestV2HelpSurfaces(t *testing.T) {
	tests := []struct {
		args      []string
		contains  []string
		forbidden []string
	}{
		{[]string{"init"}, []string{"--question-revision-id", "--question-outcome-kind", "--initial-probability"}, []string{"--question-type", "--initial-value-kind", "--initial-probability-bp"}},
		{[]string{"question", "add"}, []string{"--revision-id", "--outcome-kind", "--initial-probability"}, []string{"--type", "--initial-probability-bp"}},
		{[]string{"question", "revise"}, []string{"--revision-id", "--effective-at", "--outcome-kind"}, nil},
		{[]string{"forecast", "add"}, []string{"--question-revision", "--probability", "--pmf", "--binned-pmf-set", "--quantile", "--cdf", "--point", "--credible-interval"}, []string{"--value-kind", "--probability-bp"}},
		{[]string{"forecast", "seal"}, []string{"--question-revision", "--secret-input", "--key-file"}, nil},
		{[]string{"question", "void"}, []string{"--reason", "--source", "--yes"}, nil},
	}
	for _, test := range tests {
		args := append([]string{"forecast-ledger"}, test.args...)
		args = append(args, "--help")
		code, stdout, stderr := runCLI(args...)
		if code != 0 || stderr != "" {
			t.Fatalf("help %v code=%d stderr=%q", test.args, code, stderr)
		}
		for _, value := range test.contains {
			if !strings.Contains(stdout, value) {
				t.Errorf("help %v missing %q", test.args, value)
			}
		}
		for _, value := range test.forbidden {
			if strings.Contains(stdout, value) {
				t.Errorf("help %v retained obsolete %q", test.args, value)
			}
		}
	}
	code, stdout, stderr := runCLI("forecast-ledger", "question", "annul", "--help")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "No help topic") {
		t.Fatalf("annul remains visible: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestStableV2JSONShapes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "init", "--file", path, "--ledger-id", "research", "--timezone", "UTC", "--forecaster-id", "andrey", "--forecaster-name", "Andrey", "--created-at", "2026-01-01T00:00:00Z", "--question", "q-one", "--question-revision-id", "qr-one", "--question-effective-at", "2026-01-01T00:00:00Z", "--question-revision-recorded-at", "2026-01-01T00:00:00Z", "--question-title", "Will it happen?", "--question-resolution-criteria", "Resolve from the named source.", "--question-expected-resolution-at", "2027-01-01T00:00:00Z", "--question-outcome-kind", "binary", "--initial-forecast", "f-one", "--initial-forecasted-at", "2026-01-01T00:00:00Z", "--initial-recorded-at", "2026-01-01T00:00:00Z", "--initial-probability", "0.5", "--initial-probability-outcome")
	if code != 0 || stderr != "" {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertJSONFields(t, stdout, "ledger.initialized", []string{`"schema_version":"2.1.0"`, `"question_id":"q-one"`, `"forecast_id":"f-one"`})
	code, stdout, stderr = runCLI("forecast-ledger", "--json", "question", "list", "--file", path)
	if code != 0 || stderr != "" {
		t.Fatalf("question list code=%d stderr=%q", code, stderr)
	}
	assertJSONFields(t, stdout, "question.list", []string{`"current_revision_id":"qr-one"`, `"outcome_kind":"binary"`})
	code, stdout, stderr = runCLI("forecast-ledger", "--json", "forecast", "list", "--file", path, "--question", "q-one")
	if code != 0 || stderr != "" {
		t.Fatalf("forecast list code=%d stderr=%q", code, stderr)
	}
	assertJSONFields(t, stdout, "forecast.list", []string{`"question_revision_id":"qr-one"`, `"representation_kinds":["probability"]`})
}

func TestTargetBuildJSONUsesV2Projection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, fixtureBytes(t, "individual-ledger.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "target", "build", "--file", path, "--question", "q-election-coalition", "--forecast", "f-election-coalition-001")
	if code != 0 || stderr != "" {
		t.Fatalf("target code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertJSONFields(t, stdout, "target.built", []string{`"sha256":"e232db4b3ef9609976f34a6abc70a56faf39e6811f6f5d2a17655afc039aa391"`, `"size":1152`})
}

func TestJSONErrorSchemaExitCodesAndCancellation(t *testing.T) {
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "validate")
	if code != 2 || stdout != "" {
		t.Fatalf("usage code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var envelope presentation.ErrorEnvelope
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("JSON error is invalid: %v (%s)", err, stderr)
	}
	if envelope.OK || envelope.Code != app.CodeUsage || envelope.Message == "" {
		t.Fatalf("unexpected JSON error: %#v", envelope)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var canceledOut, canceledErr bytes.Buffer
	code = Run(ctx, []string{"forecast-ledger", "validate", "--file", "ledger.yaml"}, strings.NewReader(""), &canceledOut, &canceledErr)
	if code != 130 || canceledOut.Len() != 0 || !strings.Contains(canceledErr.String(), "interrupted") {
		t.Fatalf("cancellation code=%d stdout=%q stderr=%q", code, canceledOut.String(), canceledErr.String())
	}
}

func TestOutputWriterFailureReturnsInternalExit(t *testing.T) {
	writer := failingWriter{}
	if code := Run(context.Background(), []string{"forecast-ledger", "version"}, strings.NewReader(""), writer, writer); code != 1 {
		t.Fatalf("writer failure exit=%d, want 1", code)
	}
}

func assertJSONFields(t *testing.T, output, code string, fields []string) {
	t.Helper()
	if !strings.Contains(output, `"code":"`+code+`"`) {
		t.Errorf("output missing code %q: %s", code, output)
	}
	for _, field := range fields {
		if !strings.Contains(output, field) {
			t.Errorf("output missing %s: %s", field, output)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrPermission }
