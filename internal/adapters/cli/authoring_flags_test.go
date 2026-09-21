package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/buildinfo"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/presentation"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	urfavecli "github.com/urfave/cli/v3"
)

func TestAuthoringInventoryIsClassifiedAndHasNoGenericInputFlag(t *testing.T) {
	root := NewCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	for _, entry := range authoringInventory {
		if entry.Path == "" || entry.Schema == "" || len(entry.Fields) == 0 {
			t.Errorf("incomplete authoring inventory entry: %#v", entry)
		}
		command := commandAtPath(root, entry.Path)
		if command == nil {
			t.Errorf("inventory command %q is not registered", entry.Path)
			continue
		}
		for _, field := range entry.Fields {
			if field.Field == "" || field.Class == "" || field.Route == "" && field.Class != "service-only" {
				t.Errorf("incomplete route in %s: %#v", entry.Path, field)
			}
		}
		for _, flag := range command.Flags {
			if flag.Names()[0] == "input" {
				t.Errorf("authoring command %s exposes removed generic --input", entry.Path)
			}
		}
	}
}

func TestRepeatableCSVGrammarSupportsQuotingAndRejectsAmbiguity(t *testing.T) {
	rows, err := parseCSVValues("source", []string{`"Official result, final",https://example.test/result,2026-09-21T12:00:00Z`}, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0][0] != "Official result, final" || rows[0][1] != "https://example.test/result" {
		t.Fatalf("quoted CSV row = %#v", rows)
	}
	for _, test := range []struct {
		name  string
		value string
	}{
		{"too few", "only-one"},
		{"too many", "a,b,c,d,e,f,g"},
		{"two records", "a,b,c\nd,e,f"},
		{"broken quote", `"a,b,c`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseCSVValues("source", []string{test.value}, 3, 6); err == nil {
				t.Fatalf("accepted ambiguous CSV %q", test.value)
			}
		})
	}
}

func TestRemovedV1AuthoringSurfacesStayAbsent(t *testing.T) {
	root := NewCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	forbiddenFlags := map[string]bool{"input": true, "input-file": true, "type": true, "value-kind": true, "probability-bp": true, "choice-probability": true, "interval": true}
	if err := root.Walk(func(command *urfavecli.Command) error {
		for _, flag := range command.Flags {
			for _, name := range flag.Names() {
				if forbiddenFlags[name] {
					t.Errorf("%s exposes removed --%s", command.FullName(), name)
				}
			}
		}
		if command.FullName() == "forecast-ledger question annul" {
			t.Error("removed question annul command is registered")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	questionUpdate := commandAtPath(root, "question update")
	if questionUpdate == nil {
		t.Fatal("question update command is not registered")
	}
	for _, forbidden := range []string{"title", "resolution-criteria", "forecasting-opens-at", "expected-resolution-at", "outcome-kind", "option", "bin"} {
		for _, flag := range questionUpdate.Flags {
			if slices.Contains(flag.Names(), forbidden) {
				t.Errorf("question update exposes immutable revision flag --%s; use question revise", forbidden)
			}
		}
	}
	catalog, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "reference", "generated", "mcp-tool-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated struct {
		Tools map[string]struct {
			ToolSchema struct {
				Properties map[string]any `json:"properties"`
			} `json:"tool_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(catalog, &generated); err != nil {
		t.Fatal(err)
	}
	for tool, definition := range generated.Tools {
		for _, forbidden := range []string{"input", "input_file", "value", "probability_bp"} {
			if _, exists := definition.ToolSchema.Properties[forbidden]; exists {
				t.Errorf("generated MCP tool %s contains removed top-level field %s", tool, forbidden)
			}
		}
	}
	for _, forbidden := range []string{`"probability_bp":`, `"multiple_choice"`, `"question_annul"`} {
		if bytes.Contains(catalog, []byte(forbidden)) {
			t.Errorf("generated MCP catalog contains removed surface %s", forbidden)
		}
	}
	if update, ok := generated.Tools["question_update"]; ok {
		for _, forbidden := range []string{"revision", "title", "resolution_criteria", "forecasting_opens_at", "expected_resolution_at", "outcome_space", "domain"} {
			if _, exists := update.ToolSchema.Properties[forbidden]; exists {
				t.Errorf("question_update exposes immutable revision property %s", forbidden)
			}
		}
	} else {
		t.Error("generated MCP catalog is missing question_update")
	}
}

func TestDirectV2BuildersMatchClosedSchemas(t *testing.T) {
	questionCommand := &urfavecli.Command{Name: "builder", Flags: questionCreateFlags(true), DisableSliceFlagSeparator: true}
	var question service.QuestionAddInput
	questionCommand.Action = func(_ context.Context, command *urfavecli.Command) error {
		var err error
		question, err = buildQuestionAddInput(context.Background(), command, strings.NewReader(""))
		return err
	}
	args := []string{"builder", "--revision-id", "qr-one", "--effective-at", "2026-09-21T10:00:00Z", "--revision-recorded-at", "2026-09-21T10:01:00Z", "--title", "Will it happen?", "--resolution-criteria", "Use the official result.", "--expected-resolution-at", "2027-01-01T00:00:00Z", "--outcome-kind", "binary", "--initial-forecast", "f-one", "--initial-forecasted-at", "2026-09-22T00:00:00Z", "--initial-recorded-at", "2026-09-22T00:01:00Z", "--initial-probability", "0.6", "--initial-probability-outcome"}
	if err := questionCommand.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(question)
	if err != nil {
		t.Fatal(err)
	}
	var decoded service.QuestionAddInput
	if err := service.DecodeOperationInput(context.Background(), "-", bytes.NewReader(encoded), service.InputSchemaQuestionAdd, &decoded); err != nil {
		t.Fatalf("direct question input does not match its schema: %v", err)
	}
	if decoded.Revision.ID != "qr-one" || decoded.InitialForecast == nil || len(decoded.InitialForecast.Representations) != 1 {
		t.Fatalf("decoded question = %#v", decoded)
	}

	forecastCommand := &urfavecli.Command{Name: "builder", Flags: forecastCreateFlags(), DisableSliceFlagSeparator: true}
	var forecast service.ForecastCreateInput
	forecastCommand.Action = func(_ context.Context, command *urfavecli.Command) error {
		var err error
		forecast, err = buildForecastCreateInput(command)
		return err
	}
	if err := forecastCommand.Run(context.Background(), []string{"builder", "--question-revision", "qr-numeric", "--forecasted-at", "2026-09-22T00:00:00Z", "--point", "median,42.5", "--quantile-interpolation", "linear", "--quantile", "0.1,30", "--quantile", "0.9,60", "--credible-interval", "0.8,equal_tailed,30,60"}); err != nil {
		t.Fatal(err)
	}
	if len(forecast.Representations) != 3 {
		t.Fatalf("representation count = %d, want 3", len(forecast.Representations))
	}
}

func TestFlagOnlyV2WorkflowJSONAndYAML(t *testing.T) {
	for _, extension := range []string{".json", ".yaml"} {
		t.Run(extension, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger"+extension)
			code, stdout, stderr := runCLI("forecast-ledger", "--json", "init", "--file", path, "--ledger-id", "flags", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner")
			if code != 0 || stderr != "" || !strings.Contains(stdout, `"schema_version":"2.0.0"`) {
				t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			code, stdout, stderr = runCLI("forecast-ledger", "--json", "question", "add", "--file", path, "--question", "q-launch", "--revision-id", "qr-launch-1", "--effective-at", "2026-09-21T10:00:00Z", "--revision-recorded-at", "2026-09-21T10:01:00Z", "--title", "Will it launch?", "--resolution-criteria", "Resolve yes on launch.", "--expected-resolution-at", "2027-01-01T00:00:00Z", "--outcome-kind", "binary")
			if code != 0 || stderr != "" || !strings.Contains(stdout, `"question_id":"q-launch"`) {
				t.Fatalf("question add code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			code, stdout, stderr = runCLI("forecast-ledger", "--json", "forecast", "add", "--file", path, "--question", "q-launch", "--forecast", "f-one", "--question-revision", "qr-launch-1", "--forecasted-at", "2026-09-22T09:00:00Z", "--recorded-at", "2026-09-22T09:01:00Z", "--probability", "0.65", "--probability-outcome", "--rationale", "Public rationale")
			if code != 0 || stderr != "" || !strings.Contains(stdout, `"forecast_id":"f-one"`) {
				t.Fatalf("forecast add code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			code, stdout, stderr = runCLI("forecast-ledger", "--json", "question", "revise", "--file", path, "--question", "q-launch", "--revision-id", "qr-launch-2", "--effective-at", "2026-10-01T00:00:00Z", "--revision-recorded-at", "2026-10-01T00:01:00Z", "--title", "Will the revised product launch?", "--resolution-criteria", "Resolve yes on revised launch.", "--expected-resolution-at", "2027-02-01T00:00:00Z", "--outcome-kind", "binary")
			if code != 0 || stderr != "" || !strings.Contains(stdout, `"question_id":"q-launch"`) {
				t.Fatalf("question revise code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			stored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"qr-launch-1", "qr-launch-2", "question_revision_id", "representations"} {
				if !bytes.Contains(stored, []byte(want)) {
					t.Errorf("ledger missing %q:\n%s", want, stored)
				}
			}
		})
	}
}

func TestEndToEndDirectV2AuthoringMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matrix.json")
	mustRunCLI(t, "init", "--file", path, "--ledger-id", "matrix", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner", "--created-at", "2026-01-01T00:00:00Z")
	mustRunCLI(t, "platform", "add", "--file", path, "--platform", "source", "--name", "Source", "--kind", "internal")
	mustRunCLI(t, "group", "add", "--file", path, "--group", "matrix-group", "--title", "Matrix questions")

	base := func(id, revision, title, kind string, extra ...string) {
		args := []string{"question", "add", "--file", path, "--question", id, "--revision-id", revision,
			"--effective-at", "2026-09-21T10:00:00Z", "--revision-recorded-at", "2026-09-21T10:01:00Z",
			"--title", title, "--resolution-criteria", "Use the named public result.",
			"--expected-resolution-at", "2028-01-01T00:00:00Z", "--outcome-kind", kind,
			"--revision-provenance-platform", "source", "--revision-remote-object-id", id,
			"--revision-retrieved-at", "2026-09-21T09:59:00Z"}
		mustRunCLI(t, append(args, extra...)...)
	}
	base("q-binary", "qr-binary-1", "Binary question", "binary")
	base("q-category", "qr-category-1", "Categorical question", "categorical",
		"--option-set-id", "choices", "--option-set-version", "1", "--option", "low,Low", "--option", "high,High")
	base("q-ordinal", "qr-ordinal-1", "Ordinal question", "ordinal",
		"--option-set-id", "levels", "--option-set-version", "2", "--option", "first,First", "--option", "second,Second")
	base("q-numeric", "qr-numeric-1", "Numeric question", "numeric",
		"--lower-bound", "0", "--values-kind", "continuous", "--unit-name", "items",
		"--bin-set", "bands,3", "--bin", "bands,3,low,Low,0,50,true,false", "--bin", "bands,3,high,High,50,100,true,true")
	base("q-date", "qr-date-1", "Date question", "date",
		"--values-kind", "allowed_values", "--allowed-value", "2027-01-01", "--allowed-value", "2027-02-01")
	base("q-datetime", "qr-datetime-1", "Datetime question", "datetime",
		"--values-kind", "step", "--step", "PT3600S", "--origin", "2026-01-01T00:00:00Z")

	mustRunCLI(t, "relationship", "add", "--file", path, "--relationship", "member-binary", "--kind", "group_membership", "--group", "matrix-group", "--question", "q-binary")
	mustRunCLI(t, "relationship", "add", "--file", path, "--relationship", "if-low", "--kind", "conditional", "--parent-question", "q-category", "--parent-revision", "qr-category-1", "--parent-outcome", "low", "--child-question", "q-numeric")

	mustRunCLI(t, "forecast", "add", "--file", path, "--question", "q-binary", "--forecast", "f-binary", "--question-revision", "qr-binary-1", "--forecasted-at", "2026-10-01T00:00:00Z", "--recorded-at", "2026-10-01T00:01:00Z", "--probability", "0.6", "--probability-outcome")
	mustRunCLI(t, "forecast", "add", "--file", path, "--question", "q-category", "--forecast", "f-category", "--question-revision", "qr-category-1", "--forecasted-at", "2026-10-01T00:00:00Z", "--recorded-at", "2026-10-01T00:01:00Z", "--pmf-set", "choices,1", "--pmf", "low,0.4", "--pmf", "high,0.6")
	mustRunCLI(t, "forecast", "add", "--file", path, "--question", "q-numeric", "--forecast", "f-numeric", "--question-revision", "qr-numeric-1", "--forecasted-at", "2026-10-01T00:00:00Z", "--recorded-at", "2026-10-01T00:01:00Z",
		"--binned-pmf-set", "bands,3", "--bin-probability", "low,0.3", "--bin-probability", "high,0.5", "--left-tail-probability", "0", "--right-tail-probability", "0.2",
		"--quantile-interpolation", "linear", "--quantile", "0.1,10", "--quantile", "0.9,90",
		"--cdf-interpolation", "linear", "--cdf", "10,0.2", "--cdf", "90,0.8", "--cdf-left-tail", "0.2", "--cdf-right-tail", "0.2",
		"--point", "median,50", "--credible-interval", "0.8,equal_tailed,10,90",
		"--provenance-platform", "source", "--remote-object-id", "forecast-numeric", "--retrieved-at", "2026-10-01T00:02:00Z")

	mustRunCLI(t, "forecast", "withdraw", "--file", path, "--question", "q-numeric", "--forecast", "f-numeric", "--event", "event-withdraw", "--effective-at", "2026-11-01T00:00:00Z", "--recorded-at", "2026-11-01T00:01:00Z")
	mustRunCLI(t, "forecast", "reaffirm", "--file", path, "--question", "q-numeric", "--forecast", "f-numeric", "--event", "event-reaffirm", "--effective-at", "2026-11-02T00:00:00Z", "--recorded-at", "2026-11-02T00:01:00Z")
	mustRunCLI(t, "forecast", "expire", "--file", path, "--question", "q-numeric", "--forecast", "f-numeric", "--event", "event-expire", "--effective-at", "2026-11-03T00:00:00Z", "--recorded-at", "2026-11-03T00:01:00Z")

	mustRunCLI(t, "question", "update", "--file", path, "--question", "q-category", "--status", "closed")
	mustRunCLI(t, "question", "resolve", "--file", path, "--question", "q-category", "--question-revision", "qr-category-1", "--outcome", "high", "--outcome-known-at", "2027-01-01T00:00:00Z", "--recorded-at", "2027-01-01T00:01:00Z", "--source", "Official result,https://example.test/result,2027-01-01T00:00:30Z", "--yes")
	mustRunCLI(t, "question", "not-applicable", "--file", path, "--question", "q-numeric", "--relationship", "if-low", "--reason", "The parent resolved to high.", "--recorded-at", "2027-01-01T00:02:00Z", "--yes")

	loaded, err := service.LoadAndValidateLedger(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := ledger.SummaryCounts(loaded.Model)
	if counts.Questions != 6 || counts.Forecasts != 3 || counts.Representations != 7 || counts.Groups != 1 || counts.Relationships != 2 || counts.LifecycleEvents != 3 {
		t.Fatalf("direct authoring counts = %#v", counts)
	}
	if loaded.Model.Questions[3].Resolution == nil || loaded.Model.Questions[3].Resolution.NotApplicable == nil {
		t.Fatal("conditional child was not recorded as not applicable")
	}
}

func mustRunCLI(t *testing.T, arguments ...string) {
	t.Helper()
	code, stdout, stderr := runCLI(append([]string{"forecast-ledger", "--json"}, arguments...)...)
	if code != 0 {
		t.Fatalf("command %v failed: code=%d stdout=%q stderr=%q", arguments, code, stdout, stderr)
	}
}

func TestProtectedV2SealDoesNotLeakPrivateFields(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	code, _, stderr := runCLI("forecast-ledger", "init", "--file", path, "--ledger-id", "protected", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner")
	if code != 0 {
		t.Fatalf("init code=%d stderr=%q", code, stderr)
	}
	code, _, stderr = runCLI("forecast-ledger", "question", "add", "--file", path, "--question", "q-one", "--revision-id", "qr-one", "--effective-at", "2026-09-21T10:00:00Z", "--revision-recorded-at", "2026-09-21T10:01:00Z", "--title", "Will it happen?", "--resolution-criteria", "Use the official result.", "--expected-resolution-at", "2027-01-02T00:00:00Z", "--outcome-kind", "binary")
	if code != 0 {
		t.Fatalf("question add code=%d stderr=%q", code, stderr)
	}
	privatePath := filepath.Join(directory, "private.json")
	privateJSON := `{"representations":[{"kind":"probability","outcome":true,"probability":"0.7"}],"rationale":"secret-rationale","key_factors":["private-factor"],"comment":"secret-comment"}`
	if err := storage.CreateProtectedFile(privatePath, []byte(privateJSON)); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "forecast.key")
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "forecast", "seal", "--file", path, "--question", "q-one", "--forecast", "f-sealed", "--question-revision", "qr-one", "--forecasted-at", "2026-09-22T00:00:00Z", "--recorded-at", "2026-09-22T00:01:00Z", "--secret-input", privatePath, "--key-file", keyPath)
	if code != 0 || stderr != "" {
		t.Fatalf("seal code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-rationale", "private-factor", "secret-comment"} {
		if strings.Contains(stdout+stderr+string(stored), secret) {
			t.Fatalf("sealed private value %q leaked", secret)
		}
	}
}

func TestVersionFormattingAndStableJSON(t *testing.T) {
	info := buildinfo.Current()
	var plain bytes.Buffer
	if err := writeVersionInfo(&plain, info, presentation.ModePlain, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.String(), "Forecast Ledger schema: 2.0.0") {
		t.Fatalf("plain version is stale:\n%s", plain.String())
	}
	code, stdout, stderr := runCLI("forecast-ledger", "version", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("version code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var decoded buildinfo.Info
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema.Version != "2.0.0" || decoded.Schema != info.Schema {
		t.Fatalf("version JSON = %#v", decoded)
	}
}

func commandAtPath(root *urfavecli.Command, path string) *urfavecli.Command {
	current := root
	for _, name := range strings.Fields(path) {
		current = current.Command(name)
		if current == nil {
			return nil
		}
	}
	return current
}

var _ = ledger.RepresentationProbability
