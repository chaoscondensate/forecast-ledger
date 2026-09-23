package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/timestamp/rfc3161"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixedMCPClock struct{ value time.Time }

func (clock fixedMCPClock) Now() time.Time { return clock.value }

func TestGeneratedMCPToolCatalogMatchesRuntimeFieldSurface(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "reference", "generated", "mcp-tool-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools map[string]struct {
			ToolSchema map[string]any `json:"tool_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	runtimeContracts := contracts()
	for _, definition := range service.SortedOperationDefinitions() {
		contract, ok := runtimeContracts[definition.Name]
		if !ok {
			t.Fatalf("runtime contract missing for %s", definition.Name)
		}
		contract, err = expandDirectContract(definition, contract)
		if err != nil {
			t.Fatal(err)
		}
		runtimeSchema, err := toolSchema(definition, contract)
		if err != nil {
			t.Fatal(err)
		}
		generated, ok := catalog.Tools[definition.MCPTool]
		if !ok {
			t.Errorf("generated catalog missing %s", definition.MCPTool)
			continue
		}
		if !reflect.DeepEqual(sortedAnyStrings(runtimeSchema["required"]), sortedAnyStrings(generated.ToolSchema["required"])) {
			t.Errorf("%s required fields drift: runtime=%v generated=%v", definition.MCPTool, runtimeSchema["required"], generated.ToolSchema["required"])
		}
		if !reflect.DeepEqual(sortedMapKeys(runtimeSchema["properties"]), sortedMapKeys(generated.ToolSchema["properties"])) {
			t.Errorf("%s property surface drift: runtime=%v generated=%v", definition.MCPTool, sortedMapKeys(runtimeSchema["properties"]), sortedMapKeys(generated.ToolSchema["properties"]))
		}
	}
}

func sortedAnyStrings(value any) []string {
	values, _ := value.([]any)
	if typed, ok := value.([]string); ok {
		result := append([]string(nil), typed...)
		sort.Strings(result)
		return result
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	sort.Strings(result)
	return result
}

func sortedMapKeys(value any) []string {
	values, _ := value.(map[string]any)
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func TestMCPForecastMutationRetriesAutomaticLedgerRecovery(t *testing.T) {
	ledgerRoot := t.TempDir()
	path := filepath.Join(ledgerRoot, "ledger.json")
	copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"), path)
	err := storage.UpdateLedger(t.Context(), path, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error {
			return service.ValidateLedgerDocument(parsed, os.DirFS(ledgerRoot))
		},
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/title", Value: "Interrupted title"}})
		},
		Fault: func(stage storage.TransactionStage) error {
			if stage == storage.StageJournalSynced {
				return errors.New("simulated stop")
			}
			return nil
		},
	})
	if app.ErrorCodeOf(err) != app.CodeIO {
		t.Fatalf("interrupted write error=%v", err)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_add", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-003", "question_revision_id": "qr-election-coalition-2",
		"forecasted_at": "2026-09-01T09:00:00+01:00", "recorded_at": "2026-09-01T09:01:00+01:00", "supersedes_forecast_id": "f-election-coalition-002", "representations": []any{map[string]any{
			"kind": "pmf", "option_set_ref": map[string]any{"id": "coalitions", "version": 2}, "entries": []any{
				map[string]any{"option_id": "centre-left", "probability": "0.42"}, map[string]any{"option_id": "centre-right", "probability": "0.35"}, map[string]any{"option_id": "unity", "probability": "0.15"}, map[string]any{"option_id": "other", "probability": "0.08"},
			},
		}},
	}})
	if err != nil || result.IsError || !strings.Contains(toolText(result), `"code":"forecast.added"`) {
		t.Fatalf("MCP retry result=%s err=%v", toolText(result), err)
	}
	if _, statErr := os.Stat(storage.JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("MCP retry retained journal: %v", statErr)
	}
}

func TestMCPMissingRevealKeyNamesKeyFile(t *testing.T) {
	ledgerRoot, secretRoot := t.TempDir(), t.TempDir()
	path := filepath.Join(ledgerRoot, "ledger.json")
	copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"), path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Mode: service.AccessMode{AllowReveal: true}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_reveal", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001", "key_file": "keys:missing.key", "confirm": true,
	}})
	text := toolText(result)
	if callErr != nil || !result.IsError || !strings.Contains(text, `"code":"not_found"`) || !strings.Contains(text, "key file does not exist") || strings.Contains(text, "ledger file does not exist") || strings.Contains(text, secretRoot) {
		t.Fatalf("missing key result=%s err=%v", text, callErr)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("missing key changed ledger: %v", err)
	}
}

func TestMCPForecastDefaultsUseOneLedgerTimezoneObservation(t *testing.T) {
	ledgerRoot := t.TempDir()
	effects := service.ProductionEffects()
	effects.Clock = fixedMCPClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second, Effects: effects})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()

	calls := []struct {
		name string
		args map[string]any
	}{
		{"ledger_init", map[string]any{"file": "main:ledger.yaml", "ledger_id": "clock", "timezone": "Europe/London", "forecaster_id": "owner", "forecaster_name": "Owner"}},
		{"question_add", map[string]any{"file": "main:ledger.yaml", "question": "q-clock", "revision": binaryRevision("qr-clock", "Question", "Public result", "2030-08-10T23:59:59+01:00")}},
		{"forecast_add", map[string]any{"file": "main:ledger.yaml", "question": "q-clock", "forecast": "f-clock", "question_revision_id": "qr-clock", "representations": probabilityRepresentations("0.5")}},
	}
	for _, call := range calls {
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: call.name, Arguments: call.args})
		if callErr != nil || result.IsError {
			t.Fatalf("%s failed: %v %s", call.name, callErr, toolText(result))
		}
	}

	raw, err := os.ReadFile(filepath.Join(ledgerRoot, "ledger.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(raw), `"2026-08-30T18:00:00+01:00"`); count < 2 {
		t.Fatalf("forecast defaults did not share the fixed ledger-timezone observation:\n%s", raw)
	}
}

func TestMCPQuestionRevisionDefaultsAdvanceOnSameClockTick(t *testing.T) {
	ledgerRoot := t.TempDir()
	effects := service.ProductionEffects()
	effects.Clock = fixedMCPClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second, Effects: effects})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	calls := []struct {
		name string
		args map[string]any
	}{
		{"ledger_init", map[string]any{"file": "main:ledger.yaml", "ledger_id": "revision-clock", "timezone": "Europe/London", "forecaster_id": "owner", "forecaster_name": "Owner"}},
		{"question_add", map[string]any{"file": "main:ledger.yaml", "question": "q-clock", "revision": binaryRevision("qr-one", "Question", "Public result", "2030-08-10T23:59:59+01:00")}},
		{"question_revise", map[string]any{"file": "main:ledger.yaml", "question": "q-clock", "id": "qr-two", "title": "Revised question", "resolution_criteria": "Public result", "expected_resolution_at": "2030-08-10T23:59:59+01:00", "outcome_space": map[string]any{"kind": "binary"}, "domain": map[string]any{"kind": "binary"}}},
	}
	for _, call := range calls {
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: call.name, Arguments: call.args})
		if callErr != nil || result.IsError {
			t.Fatalf("%s failed: %v %s", call.name, callErr, toolText(result))
		}
	}
	loaded, err := service.LoadAndValidateLedger(t.Context(), filepath.Join(ledgerRoot, "ledger.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	revisions := loaded.Model.Questions[0].Revisions
	if len(revisions) != 2 || revisions[0].EffectiveAt != "2026-08-30T18:00:00+01:00" || revisions[1].EffectiveAt != "2026-08-30T18:00:00.000000001+01:00" || revisions[1].RecordedAt != revisions[1].EffectiveAt {
		t.Fatalf("MCP revision defaults = %#v", revisions)
	}
}

func TestMCPDefaultRevealAndTerminalTimesUseLedgerTimezone(t *testing.T) {
	ledgerRoot, secretRoot := t.TempDir(), t.TempDir()
	private := []byte(`{"representations":[{"kind":"probability","outcome":true,"probability":"0.7"}],"rationale":"private rationale","key_factors":["factor"],"comment":"private comment"}`)
	if err := storage.CreateProtectedFile(filepath.Join(secretRoot, "private.json"), private); err != nil {
		t.Fatal(err)
	}
	effects := service.ProductionEffects()
	effects.Clock = fixedMCPClock{value: time.Date(2026, 8, 30, 17, 0, 0, 0, time.UTC)}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Mode: service.AccessMode{AllowReveal: true}, Timeout: time.Second, Effects: effects})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	call := func(name string, arguments map[string]any) {
		t.Helper()
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: name, Arguments: arguments})
		if callErr != nil || result.IsError {
			t.Fatalf("%s failed: %v %s", name, callErr, toolText(result))
		}
	}
	file := "main:ledger.yaml"
	call("ledger_init", map[string]any{"file": file, "ledger_id": "timezone", "timezone": "Europe/London", "forecaster_id": "owner", "forecaster_name": "Owner", "created_at": "2026-01-01T00:00:00Z"})
	revision := binaryRevision("qr-one", "Question", "Public result", "2030-01-01T00:00:00Z")
	revision["effective_at"], revision["recorded_at"] = "2026-01-01T00:00:00Z", "2026-01-01T00:00:01Z"
	call("question_add", map[string]any{"file": file, "question": "q-one", "revision": revision})
	call("forecast_seal", map[string]any{"file": file, "question": "q-one", "forecast": "f-one", "question_revision_id": "qr-one", "forecasted_at": "2026-02-01T00:00:00Z", "recorded_at": "2026-02-01T00:00:01Z", "secret_input_file": "keys:private.json", "key_file": "keys:forecast.key"})
	call("forecast_reveal", map[string]any{"file": file, "question": "q-one", "forecast": "f-one", "key_file": "keys:forecast.key", "confirm": true})
	call("question_update", map[string]any{"file": file, "question": "q-one", "status": "closed"})
	call("question_void", map[string]any{"file": file, "question": "q-one", "reason": "No public outcome", "confirm": true})

	loaded, err := service.LoadAndValidateLedger(t.Context(), filepath.Join(ledgerRoot, "ledger.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	forecast := loaded.Model.Questions[0].Forecasts[0]
	if forecast.Commitment == nil || forecast.Commitment.Revealed == nil || forecast.Commitment.Revealed.RevealedAt != "2026-08-30T18:00:00+01:00" {
		t.Fatalf("MCP revealed_at = %#v", forecast.Commitment)
	}
	resolution := loaded.Model.Questions[0].Resolution
	if resolution == nil || resolution.Unresolved == nil || resolution.Unresolved.RecordedAt != "2026-08-30T18:00:00+01:00" {
		t.Fatalf("MCP terminal recorded_at = %#v", resolution)
	}
}

func TestMCPYAMLStructuralReplacementMutationsRemainRecoverable(t *testing.T) {
	ledgerRoot := t.TempDir()
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()

	calls := []struct {
		name string
		args map[string]any
	}{
		{"ledger_init", map[string]any{"file": "main:ledger.yaml", "ledger_id": "yaml-replacements", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner"}},
		{"platform_add", map[string]any{"file": "main:ledger.yaml", "platform": "local", "name": "Local", "kind": "self_hosted"}},
		{"question_add", map[string]any{"file": "main:ledger.yaml", "question": "q-one", "revision": binaryRevision("qr-one", "Will it happen?", "Use the official result.", "2031-01-01T00:00:00Z")}},
		{"platform_update", map[string]any{"file": "main:ledger.yaml", "platform": "local", "name": "Updated local", "kind": "internal"}},
		{"question_revise", map[string]any{"file": "main:ledger.yaml", "question": "q-one", "id": "qr-two", "effective_at": "2026-09-23T12:00:00Z", "recorded_at": "2026-09-23T12:01:00Z", "title": "Updated question title", "resolution_criteria": "Use the official result.", "expected_resolution_at": "2031-01-01T00:00:00Z", "outcome_space": map[string]any{"kind": "binary"}, "domain": map[string]any{"kind": "binary"}}},
		{"question_update", map[string]any{"file": "main:ledger.yaml", "question": "q-one", "status": "closed", "tags": []any{"reviewed", "mcp"}}},
		{"question_void", map[string]any{"file": "main:ledger.yaml", "question": "q-one", "reason": "Question became unresolvable", "recorded_at": "2026-09-24T12:00:00Z", "confirm": true}},
		{"ledger_validate", map[string]any{"file": "main:ledger.yaml"}},
	}
	for _, call := range calls {
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: call.name, Arguments: call.args})
		if callErr != nil || result.IsError || strings.Contains(toolText(result), `"code":"internal"`) {
			t.Fatalf("%s failed: err=%v result=%s", call.name, callErr, toolText(result))
		}
	}

	raw, err := os.ReadFile(filepath.Join(ledgerRoot, "ledger.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: void") || !strings.Contains(string(raw), "name: Updated local") || strings.Contains(string(raw), "{status:") {
		t.Fatalf("MCP replacements did not retain expanded valid YAML:\n%s", raw)
	}
}

func TestMCPDirectV2DomainRepresentationResolutionAndLifecycleMatrix(t *testing.T) {
	ledgerRoot := t.TempDir()
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	call := func(name string, arguments map[string]any) *sdk.CallToolResult {
		t.Helper()
		result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: result=%s err=%v", name, toolText(result), err)
		}
		return result
	}
	file := "main:matrix.yaml"
	call("ledger_init", map[string]any{"file": file, "ledger_id": "matrix", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner"})
	call("platform_add", map[string]any{"file": file, "platform": "source", "name": "Source", "kind": "internal"})
	call("group_add", map[string]any{"file": file, "group": "matrix-group", "title": "Matrix questions"})

	revision := func(id, kind string, domain map[string]any) map[string]any {
		return map[string]any{
			"id": id, "effective_at": "2026-09-21T10:00:00Z", "recorded_at": "2026-09-21T10:01:00Z",
			"title": id, "resolution_criteria": "Use the named public result.", "expected_resolution_at": "2028-01-01T00:00:00Z",
			"outcome_space": map[string]any{"kind": kind}, "domain": domain,
			"provenance": map[string]any{"platform": "source", "remote_object_id": id, "retrieved_at": "2026-09-21T09:59:00Z"},
		}
	}
	options := func(kind, id string, version int) map[string]any {
		return map[string]any{"kind": kind, "option_set": map[string]any{"id": id, "version": version, "options": []any{map[string]any{"id": "low", "label": "Low"}, map[string]any{"id": "high", "label": "High"}}}}
	}
	questions := []struct {
		id       string
		revision map[string]any
	}{
		{"q-binary", revision("qr-binary-1", "binary", map[string]any{"kind": "binary"})},
		{"q-category", revision("qr-category-1", "categorical", options("categorical", "choices", 1))},
		{"q-ordinal", revision("qr-ordinal-1", "ordinal", options("ordinal", "levels", 2))},
		{"q-numeric", revision("qr-numeric-1", "numeric", map[string]any{"kind": "numeric", "bounds": map[string]any{"lower": map[string]any{"value": "0", "inclusive": true}}, "values": map[string]any{"kind": "continuous"}, "bin_sets": []any{map[string]any{"id": "bands", "version": 3, "bins": []any{map[string]any{"id": "low", "lower": "0", "upper": "50", "lower_inclusive": true, "upper_inclusive": false}, map[string]any{"id": "high", "lower": "50", "upper": "100", "lower_inclusive": true, "upper_inclusive": true}}}}})},
		{"q-date", revision("qr-date-1", "date", map[string]any{"kind": "date", "values": map[string]any{"kind": "allowed_values", "values": []any{"2027-01-01", "2027-02-01"}}})},
		{"q-datetime", revision("qr-datetime-1", "datetime", map[string]any{"kind": "datetime", "values": map[string]any{"kind": "step", "step": "PT3600S", "origin": "2026-01-01T00:00:00Z"}})},
	}
	for _, question := range questions {
		call("question_add", map[string]any{"file": file, "question": question.id, "revision": question.revision})
	}
	dryRelationship := call("relationship_add", map[string]any{"file": file, "dry_run": true, "relationship": map[string]any{"id": "member-binary", "kind": "group_membership", "group_id": "matrix-group", "question_id": "q-binary"}})
	if !strings.Contains(toolText(dryRelationship), `"code":"relationship.add.planned"`) {
		t.Fatalf("unexpected relationship dry-run result: %s", toolText(dryRelationship))
	}
	call("relationship_add", map[string]any{"file": file, "relationship": map[string]any{"id": "member-binary", "kind": "group_membership", "group_id": "matrix-group", "question_id": "q-binary"}})
	call("relationship_add", map[string]any{"file": file, "relationship": map[string]any{"id": "if-low", "kind": "conditional", "parent_question_id": "q-category", "parent_question_revision_id": "qr-category-1", "parent_outcome": "low", "child_question_id": "q-numeric"}})

	forecastBase := func(question, forecast, revision string, representations []any) map[string]any {
		return map[string]any{"file": file, "question": question, "forecast": forecast, "question_revision_id": revision,
			"forecasted_at": "2026-10-01T00:00:00Z", "recorded_at": "2026-10-01T00:01:00Z", "representations": representations}
	}
	call("forecast_add", forecastBase("q-binary", "f-binary", "qr-binary-1", probabilityRepresentations("0.6")))
	call("forecast_add", forecastBase("q-category", "f-category", "qr-category-1", []any{map[string]any{"kind": "pmf", "option_set_ref": map[string]any{"id": "choices", "version": 1}, "entries": []any{map[string]any{"option_id": "low", "probability": "0.4"}, map[string]any{"option_id": "high", "probability": "0.6"}}}}))
	numeric := forecastBase("q-numeric", "f-numeric", "qr-numeric-1", []any{
		map[string]any{"kind": "binned_pmf", "bin_set_ref": map[string]any{"id": "bands", "version": 3}, "entries": []any{map[string]any{"bin_id": "low", "probability": "0.3"}, map[string]any{"bin_id": "high", "probability": "0.5"}}, "left_tail_probability": "0", "right_tail_probability": "0.2"},
		map[string]any{"kind": "quantiles", "interpolation": "linear", "points": []any{map[string]any{"level": "0.1", "value": "10"}, map[string]any{"level": "0.9", "value": "90"}}},
		map[string]any{"kind": "cdf", "interpolation": "linear", "points": []any{map[string]any{"value": "10", "probability": "0.2"}, map[string]any{"value": "90", "probability": "0.8"}}, "left_tail_probability": "0.2", "right_tail_probability": "0.2"},
		map[string]any{"kind": "point", "statistic": "median", "value": "50"},
		map[string]any{"kind": "credible_intervals", "intervals": []any{map[string]any{"coverage": "0.8", "interval_kind": "equal_tailed", "lower": "10", "upper": "90"}}},
	})
	numeric["provenance"] = map[string]any{"platform": "source", "remote_object_id": "forecast-numeric", "retrieved_at": "2026-10-01T00:02:00Z"}
	call("forecast_add", numeric)

	for _, event := range []struct{ tool, id, at string }{{"forecast_withdraw", "event-withdraw", "2026-11-01T00:00:00Z"}, {"forecast_reaffirm", "event-reaffirm", "2026-11-02T00:00:00Z"}, {"forecast_expire", "event-expire", "2026-11-03T00:00:00Z"}} {
		call(event.tool, map[string]any{"file": file, "question": "q-numeric", "forecast": "f-numeric", "id": event.id, "effective_at": event.at, "recorded_at": strings.Replace(event.at, "00:00Z", "01:00Z", 1), "provenance": map[string]any{"platform": "source", "remote_object_id": event.id, "retrieved_at": event.at}})
	}

	closeQuestion := func(id string) {
		call("question_update", map[string]any{"file": file, "question": id, "status": "closed"})
	}
	closeQuestion("q-category")
	call("question_resolve", map[string]any{"file": file, "question": "q-category", "question_revision_id": "qr-category-1", "outcome": "high", "outcome_known_at": "2027-01-01T00:00:00Z", "recorded_at": "2027-01-01T00:01:00Z", "sources": []any{map[string]any{"title": "Official result", "url": "https://example.test/result", "retrieved_at": "2027-01-01T00:00:30Z"}}, "confirm": true})
	call("question_not_applicable", map[string]any{"file": file, "question": "q-numeric", "relationship_id": "if-low", "reason": "The parent resolved high.", "recorded_at": "2027-01-01T00:02:00Z", "confirm": true})
	closeQuestion("q-binary")
	call("question_ambiguous", map[string]any{"file": file, "question": "q-binary", "reason": "Sources conflict.", "recorded_at": "2027-01-02T00:00:00Z", "confirm": true})
	closeQuestion("q-ordinal")
	call("question_void", map[string]any{"file": file, "question": "q-ordinal", "reason": "The event was cancelled.", "recorded_at": "2027-01-02T00:00:00Z", "confirm": true})
	call("question_dispute", map[string]any{"file": file, "question": "q-ordinal", "reason": "The cancellation is disputed.", "recorded_at": "2027-01-02T00:01:00Z", "confirm": true})

	dryRun := call("group_add", map[string]any{"file": file, "group": "dry-group", "title": "Dry", "dry_run": true})
	if !strings.Contains(toolText(dryRun), `"code":"group.add.planned"`) {
		t.Fatalf("unexpected dry-run result: %s", toolText(dryRun))
	}
	invalid, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_add", Arguments: forecastBase("q-binary", "f-invalid", "qr-binary-1", probabilityRepresentations("2"))})
	if err != nil || !invalid.IsError {
		t.Fatalf("invalid forecast was not a recoverable tool error: %s, %v", toolText(invalid), err)
	}

	stored, err := os.ReadFile(filepath.Join(ledgerRoot, "matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "group_membership:\n") || strings.Contains(string(stored), "conditional:\n") || !strings.Contains(string(stored), "kind: group_membership") || !strings.Contains(string(stored), "kind: conditional") {
		t.Fatalf("MCP relationship union shape is invalid:\n%s", stored)
	}
	loaded, err := service.LoadAndValidateLedger(t.Context(), filepath.Join(ledgerRoot, "matrix.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := ledger.SummaryCounts(loaded.Model)
	if counts.Questions != 6 || counts.Forecasts != 3 || counts.Representations != 7 || counts.Groups != 1 || counts.Relationships != 2 || counts.LifecycleEvents != 3 {
		t.Fatalf("MCP authoring counts = %#v", counts)
	}
}

func TestMCPDiscoveryClosedSchemasModesAndParityCall(t *testing.T) {
	ledgerRoot := t.TempDir()
	outputRoot := t.TempDir()
	secretRoot := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"), filepath.Join(ledgerRoot, "ledger.json"))
	server, err := New(Config{
		LedgerRoots: []string{"main=" + ledgerRoot}, OutputRoots: []string{"packages=" + outputRoot}, SecretRoots: []string{"keys=" + secretRoot},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	client := connectClient(t, ctx, server)
	defer client.Close()

	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		encoded, _ := json.Marshal(tool.InputSchema)
		if !strings.Contains(string(encoded), `"additionalProperties":false`) {
			t.Errorf("tool %s schema is not closed: %s", tool.Name, encoded)
		}
		for _, removed := range []string{`"input":`, `"input_file":`} {
			if strings.Contains(string(encoded), removed) {
				t.Errorf("tool %s still exposes removed generic wrapper %s", tool.Name, removed)
			}
		}
		for _, forbidden := range []string{"calendar", "bitcoin_core", "proxy", "explorer"} {
			if strings.Contains(string(encoded), `"`+forbidden+`"`) {
				t.Errorf("tool %s exposes forbidden MCP endpoint input %q", tool.Name, forbidden)
			}
		}
		definition, ok := operationDefinitionByTool(tool.Name)
		if !ok {
			t.Errorf("tool %s has no operation definition", tool.Name)
		} else {
			expectedEffect := "effect=read-only"
			if definition.Policy.PersistentEffect {
				expectedEffect = "effect=mutating"
			}
			if !strings.Contains(tool.Description, expectedEffect) || !strings.Contains(tool.Description, "server_access=read-write") {
				t.Errorf("tool %s description does not separate effect and server mode: %q", tool.Name, tool.Description)
			}
		}
	}
	sort.Strings(names)
	if containsName(names, "forecast_reveal") {
		t.Fatal("reveal discovered without --allow-reveal")
	}
	for _, definition := range service.OperationDefinitions() {
		if definition.Name == service.OperationForecastReveal {
			continue
		}
		if !containsName(names, definition.MCPTool) {
			t.Errorf("completed tool %s is absent", definition.MCPTool)
		}
	}
	for _, tool := range listed.Tools {
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: tool.Name, Arguments: minimumToolArguments(tool.Name)})
		if callErr != nil {
			t.Errorf("registered tool %s returned protocol error: %v", tool.Name, callErr)
			continue
		}
		if tool.Name == "ledger_init" {
			if result.IsError || !strings.Contains(toolText(result), `"question_count":0`) {
				t.Errorf("registered tool %s did not create an empty ledger: %s", tool.Name, toolText(result))
			}
			continue
		}
		if !result.IsError || !strings.Contains(toolText(result), `"code"`) {
			t.Errorf("registered tool %s did not return a recoverable application error: %s", tool.Name, toolText(result))
		}
	}
	if _, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_reveal", Arguments: map[string]any{}}); err == nil {
		t.Fatal("disabled reveal direct call did not return unknown-tool protocol error")
	}

	result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:ledger.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("ledger_validate failed: %#v", result.Content)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(encoded), `"code":"ledger.valid"`) {
		t.Fatalf("unexpected result: %s", encoded)
	}
	targetCheck, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "target_check", Arguments: map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001"}})
	if err != nil || targetCheck.IsError || !strings.Contains(toolText(targetCheck), `"state":"not_applicable"`) || !strings.Contains(toolText(targetCheck), "content.no_retained_target") {
		t.Fatalf("MCP unretained target result=%s err=%v", toolText(targetCheck), err)
	}
	targetBuild, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "target_build", Arguments: map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001"}})
	if err != nil || targetBuild.IsError {
		t.Fatalf("MCP target build result=%s err=%v", toolText(targetBuild), err)
	}
	lifecycleArgs := map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-002", "scope": "lifecycle", "head": "event-election-reaffirmed"}
	lifecycleBuildArgs := maps.Clone(lifecycleArgs)
	lifecycleBuildArgs["checkpoint"], lifecycleBuildArgs["recorded_at"] = "checkpoint-election-reaffirmed", "2026-09-05T00:00:00Z"
	lifecycleBuild, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "target_build", Arguments: lifecycleBuildArgs})
	if err != nil || lifecycleBuild.IsError || !strings.Contains(toolText(lifecycleBuild), `"scope":"forecast-lifecycle/v2"`) || !strings.Contains(toolText(lifecycleBuild), `"head_event_id":"event-election-reaffirmed"`) {
		t.Fatalf("MCP lifecycle target build result=%s err=%v", toolText(lifecycleBuild), err)
	}
	lifecycleCheck, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "target_check", Arguments: lifecycleArgs})
	if err != nil || lifecycleCheck.IsError || !strings.Contains(toolText(lifecycleCheck), `"valid":true`) {
		t.Fatalf("MCP lifecycle target check result=%s err=%v", toolText(lifecycleCheck), err)
	}
	lifecycleStatus, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "timestamp_status", Arguments: lifecycleArgs})
	if err != nil || lifecycleStatus.IsError || !strings.Contains(toolText(lifecycleStatus), `"scope":"lifecycle"`) || !strings.Contains(toolText(lifecycleStatus), `"state":"unanchored"`) {
		t.Fatalf("MCP lifecycle timestamp status result=%s err=%v", toolText(lifecycleStatus), err)
	}
	lifecycleVerify, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "timestamp_verify", Arguments: lifecycleArgs})
	if err != nil || lifecycleVerify.IsError || !strings.Contains(toolText(lifecycleVerify), `"state":"not_applicable"`) || !strings.Contains(toolText(lifecycleVerify), `"scope":"lifecycle"`) {
		t.Fatalf("MCP lifecycle timestamp verify result=%s err=%v", toolText(lifecycleVerify), err)
	}
	targetPath := filepath.Join(ledgerRoot, "proofs", "targets", "f-election-coalition-001.json")
	originalTarget, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	failedTarget, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "target_check", Arguments: map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001"}})
	if err != nil || !failedTarget.IsError || !strings.Contains(toolText(failedTarget), `"code":"target.failed"`) || !strings.Contains(toolText(failedTarget), `"state":"fail"`) || !strings.Contains(toolText(failedTarget), `"actual_sha256"`) {
		t.Fatalf("MCP failed target report=%s err=%v", toolText(failedTarget), err)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if targetBuild, err = callToolForTest(t, client, &sdk.CallToolParams{Name: "target_build", Arguments: map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001"}}); err != nil || !targetBuild.IsError || !strings.Contains(toolText(targetBuild), "evidence.indexed_artifact_missing") {
		t.Fatalf("MCP missing target mutation result=%s err=%v", toolText(targetBuild), err)
	}
	if err := os.WriteFile(targetPath, originalTarget, 0o600); err != nil {
		t.Fatal(err)
	}
	publicationPlan, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "publication_build", Arguments: map[string]any{"file": "main:ledger.json", "output": "packages:dry-run", "dry_run": true}})
	if err != nil || publicationPlan.IsError || !strings.Contains(toolText(publicationPlan), `"code":"publication.build.planned"`) || strings.Contains(toolText(publicationPlan), `"code":"publication.built"`) {
		t.Fatalf("MCP publication dry-run result=%s err=%v", toolText(publicationPlan), err)
	}
	if _, statErr := os.Stat(filepath.Join(outputRoot, "dry-run")); !os.IsNotExist(statErr) {
		t.Fatalf("MCP publication dry-run created output: %v", statErr)
	}
	forecastShow, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_show", Arguments: map[string]any{"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001"}})
	if err != nil || forecastShow.IsError || !strings.Contains(toolText(forecastShow), `"integrity":{"status":"retained"`) {
		t.Fatalf("MCP forecast integrity result=%s err=%v", toolText(forecastShow), err)
	}
	autoPlan, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "timestamp_stamp", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001", "dry_run": true,
	}})
	if err != nil || autoPlan.IsError || !strings.Contains(toolText(autoPlan), `"selection_mode":"auto"`) || !strings.Contains(toolText(autoPlan), `"provider_id":"freetsa"`) || !strings.Contains(toolText(autoPlan), `"request_count":0`) {
		t.Fatalf("MCP automatic timestamp plan result=%s err=%v", toolText(autoPlan), err)
	}
	if _, statErr := os.Stat(filepath.Join(ledgerRoot, "trust")); !os.IsNotExist(statErr) {
		t.Fatalf("MCP automatic dry-run created trust: %v", statErr)
	}
	if err := os.MkdirAll(filepath.Join(ledgerRoot, "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, filepath.Join("..", "..", "timestamp", "rfc3161", "testdata", "root.pem"), filepath.Join(ledgerRoot, "trust", "tsa.pem"))
	tsaFailure, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "timestamp_stamp", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-election-coalition", "forecast": "f-election-coalition-001",
		"tsa_url": "https://127.0.0.1", "ca_bundle": "trust/tsa.pem",
	}})
	if err != nil || !tsaFailure.IsError || !strings.Contains(toolText(tsaFailure), `"code":"timestamp.not_checked"`) || !strings.Contains(toolText(tsaFailure), `"timing.tsa_unavailable"`) || !strings.Contains(toolText(tsaFailure), `"request_count":1`) {
		t.Fatalf("MCP safe TSA failure result=%s err=%v", toolText(tsaFailure), err)
	}
	sessionStillAlive, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:ledger.json"}})
	if err != nil || sessionStillAlive.IsError {
		t.Fatalf("MCP session did not survive TSA failure: result=%s err=%v", toolText(sessionStillAlive), err)
	}
	semanticFailure, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "platform_add", Arguments: map[string]any{"file": "main:ledger.json", "platform": "invalid-semantic", "name": "   ", "kind": "informal"}})
	if err != nil || !semanticFailure.IsError || strings.Contains(toolText(semanticFailure), `"line":0`) || strings.Contains(toolText(semanticFailure), `"column":0`) {
		t.Fatalf("MCP semantic diagnostic fabricated a span: %s, %v", toolText(semanticFailure), err)
	}

	lock, err := storage.AcquireLedgerLock(ctx, filepath.Join(ledgerRoot, "ledger.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	conflict, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "platform_add", Arguments: map[string]any{
		"file": "main:ledger.json", "platform": "locked-platform", "name": "Locked", "kind": "internal",
	}})
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil || !conflict.IsError || !strings.Contains(toolText(conflict), string(app.CodeConflict)) || time.Since(started) > time.Second {
		t.Fatalf("same-ledger writer conflict result=%s err=%v", toolText(conflict), err)
	}

	copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"), filepath.Join(ledgerRoot, "second.json"))
	independent, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "platform_add", Arguments: map[string]any{
		"file": "main:second.json", "platform": "independent-platform", "name": "Independent", "kind": "internal",
	}})
	if err != nil || independent.IsError {
		t.Fatalf("cross-ledger mutation was blocked: result=%s err=%v", toolText(independent), err)
	}

	unknown, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:ledger.json", "typo": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !unknown.IsError || !strings.Contains(toolText(unknown), "unknown field") {
		t.Fatalf("unknown property accepted: %#v", unknown)
	}

	escape, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:../outside.json"}})
	if err != nil {
		t.Fatal(err)
	}
	escapeText := toolText(escape)
	if !escape.IsError || !strings.Contains(escapeText, `"route":"ledger:main"`) || !strings.Contains(escapeText, `"flag":"--ledger-root"`) || strings.Contains(escapeText, ledgerRoot) {
		t.Fatalf("root traversal diagnostic is incomplete or unsafe: %s", escapeText)
	}
	missingRoot, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "missing:ledger.json"}})
	if err != nil {
		t.Fatal(err)
	}
	missingRootText := toolText(missingRoot)
	if !missingRoot.IsError || !strings.Contains(missingRootText, `"root":"missing"`) || !strings.Contains(missingRootText, `"class":"ledger"`) || strings.Contains(missingRootText, ledgerRoot) {
		t.Fatalf("root diagnostic is not safe: %s", missingRootText)
	}

	templates, err := client.ListResourceTemplates(ctx, nil)
	if err != nil || len(templates.ResourceTemplates) != 1 {
		t.Fatalf("resource templates=%d err=%v", len(templates.ResourceTemplates), err)
	}
	resource, err := client.ReadResource(ctx, &sdk.ReadResourceParams{URI: "forecast-ledger://v1/ledger/main/ledger.json"})
	if err != nil || len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "ledger_id") {
		t.Fatalf("ledger resource=%#v err=%v", resource, err)
	}
}

func TestEveryMCPExistingLedgerToolRejectsUnsupportedVersionAtAdmission(t *testing.T) {
	ledgerRoot, outputRoot, secretRoot := t.TempDir(), t.TempDir(), t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldLedger := bytes.Replace(raw, []byte(`"schema_version": "2.2.0"`), []byte(`"schema_version": "2.1.0"`), 1)
	if err := os.WriteFile(filepath.Join(ledgerRoot, "ledger.json"), oldLedger, 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, OutputRoots: []string{"packages=" + outputRoot}, SecretRoots: []string{"keys=" + secretRoot}, Mode: service.AccessMode{AllowReveal: true}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "ledger_init" || tool.Name == "publication_verify" {
			continue
		}
		arguments := minimumToolArguments(tool.Name)
		arguments["file"] = "main:ledger.json"
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: tool.Name, Arguments: arguments})
		if callErr != nil || !result.IsError || !strings.Contains(toolText(result), `"code":"unsupported_schema_version"`) {
			t.Errorf("tool %s bypassed schema-version admission: result=%s err=%v", tool.Name, toolText(result), callErr)
		}
	}
	if _, err := os.Stat(filepath.Join(ledgerRoot, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("old-schema MCP admission created artifacts: %v", err)
	}
	if entries, err := os.ReadDir(outputRoot); err != nil || len(entries) != 0 {
		t.Fatalf("old-schema MCP admission wrote output: entries=%v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(secretRoot); err != nil || len(entries) != 0 {
		t.Fatalf("old-schema MCP admission wrote secrets: entries=%v err=%v", entries, err)
	}
}

func TestMCPPublicationVerifyUsesOnePackageOutputRoot(t *testing.T) {
	ledgerRoot, outputRoot := t.TempDir(), t.TempDir()
	ledgerPath := filepath.Join(ledgerRoot, "ledger.json")
	copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", "examples", "valid", "individual-ledger.json"), ledgerPath)
	if _, err := service.CommitTargetBuild(t.Context(), ledgerPath, false, "q-election-coalition", "f-election-coalition-001"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ledgerRoot, "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, filepath.Join("..", "..", "timestamp", "rfc3161", "testdata", "root.pem"), filepath.Join(ledgerRoot, "trust", "tsa.pem"))
	requestPath, _, err := service.TimestampEvidencePaths("f-election-coalition-001", "https://tsa.example.test")
	if err != nil {
		t.Fatal(err)
	}
	absoluteRequest := filepath.Join(ledgerRoot, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, filepath.Join("..", "..", "timestamp", "rfc3161", "testdata", "request.tsq"), absoluteRequest)
	response, err := os.ReadFile(filepath.Join("..", "..", "timestamp", "rfc3161", "testdata", "response.tsr"))
	if err != nil {
		t.Fatal(err)
	}
	client := &rfc3161.HTTPClient{Resolver: mcpPublicResolver{}, Client: &http.Client{Transport: mcpRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(response)), Request: request}, nil
	})}}
	if _, err := service.CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", service.TimestampStampOptions{TSAURL: "https://tsa.example.test", CABundlePath: "trust/tsa.pem", Effects: service.ProductionEffects(), HTTPClient: client}); err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(outputRoot, "evidence")
	if _, err := service.CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false); err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, OutputRoots: []string{"packages=" + outputRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	mcpClient := connectClient(t, t.Context(), server)
	defer mcpClient.Close()
	result, err := callToolForTest(t, mcpClient, &sdk.CallToolParams{Name: "publication_verify", Arguments: map[string]any{
		"file": "packages:evidence/ledger/ledger.json", "manifest": "packages:evidence/manifest.json",
	}})
	if err != nil || result.IsError || !strings.Contains(toolText(result), `"overall":"pass"`) {
		t.Fatalf("MCP package verification result=%s err=%v", toolText(result), err)
	}
}

type mcpPublicResolver struct{}

func (mcpPublicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

type mcpRoundTripper func(*http.Request) (*http.Response, error)

func (fn mcpRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestMCPEmptyInitAndBacklogQuestion(t *testing.T) {
	ledgerRoot := t.TempDir()
	secretRoot := t.TempDir()
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	client := connectClient(t, ctx, server)
	defer client.Close()

	initialized, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_init", Arguments: map[string]any{
		"file": "main:empty.json", "ledger_id": "empty", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner",
	}})
	if err != nil || initialized.IsError || !strings.Contains(toolText(initialized), `"question_count":0`) || !strings.Contains(toolText(initialized), `"forecast_count":0`) {
		t.Fatalf("empty init result=%s err=%v", toolText(initialized), err)
	}

	added, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "question_add", Arguments: map[string]any{
		"file": "main:empty.json", "question": "q-one", "revision": binaryRevision("qr-one", "Will it happen?", "Resolve from the named source.", "2027-01-01T00:00:00Z"),
	}})
	if err != nil || added.IsError || !strings.Contains(toolText(added), `"message":"Question was added"`) {
		t.Fatalf("question add result=%s err=%v", toolText(added), err)
	}
	listed, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_list", Arguments: map[string]any{"file": "main:empty.json", "question": "q-one"}})
	if err != nil || listed.IsError || !strings.Contains(toolText(listed), `"forecasts":[]`) {
		t.Fatalf("forecast list result=%s err=%v", toolText(listed), err)
	}

	sealed, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "question_add", Arguments: map[string]any{
		"file": "main:empty.json", "question": "q-secret", "revision": binaryRevision("qr-secret", "Secret", "Resolve from the named source.", "2027-01-01T00:00:00Z"),
		"initial_forecast": map[string]any{"id": "f-secret", "question_revision_id": "qr-secret", "visibility": "sealed", "forecasted_at": "2026-09-22T00:00:00Z", "representations": probabilityRepresentations("0.5"), "rationale": "private", "key_factors": []any{}, "comment": "private"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !sealed.IsError || !strings.Contains(toolText(sealed), "initial_secret_input_file") {
		t.Fatalf("inline sealed input was not rejected safely: %s", toolText(sealed))
	}
}

func TestMCPProtectedInputDiagnosticsPrecedeAllSealedWriteRoutes(t *testing.T) {
	ledgerRoot, secretRoot := t.TempDir(), t.TempDir()
	const canary = "PRIVATE-MCP-DIAGNOSTIC-CANARY"
	if err := storage.CreateProtectedFile(filepath.Join(secretRoot, "invalid.yaml"), []byte("rationale: "+canary+"\n")); err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	sealedForecast := func(id string) map[string]any {
		return map[string]any{"id": id, "visibility": "sealed", "forecasted_at": "2026-09-22T00:00:00Z"}
	}
	datedRevision := func(id string) map[string]any {
		revision := binaryRevision(id, "Will it happen?", "Use the public result.", "2027-01-01T00:00:00Z")
		revision["effective_at"] = "2026-01-01T00:00:00Z"
		revision["recorded_at"] = "2026-01-01T00:00:01Z"
		return revision
	}
	assertFailure := func(name, key string, params *sdk.CallToolParams) {
		t.Helper()
		result, err := callToolForTest(t, client, params)
		text := toolText(result)
		if err != nil || !result.IsError || !strings.Contains(text, `"pointer":"/representations"`) || strings.Contains(text, canary) || strings.Contains(text, `"line":1`) {
			t.Fatalf("%s result=%s err=%v", name, text, err)
		}
		if _, statErr := os.Stat(filepath.Join(secretRoot, key)); !os.IsNotExist(statErr) {
			t.Fatalf("%s created key: %v", name, statErr)
		}
	}

	assertFailure("ledger_init", "initial.key", &sdk.CallToolParams{Name: "ledger_init", Arguments: map[string]any{
		"file": "main:initial.json", "ledger_id": "initial", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner",
		"question":                  map[string]any{"id": "q-initial", "revision": datedRevision("qr-initial"), "initial_forecast": sealedForecast("f-initial")},
		"initial_secret_input_file": "keys:invalid.yaml", "key_file": "keys:initial.key",
	}})
	if _, statErr := os.Stat(filepath.Join(ledgerRoot, "initial.json")); !os.IsNotExist(statErr) {
		t.Fatalf("failed MCP init created ledger: %v", statErr)
	}

	created, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_init", Arguments: map[string]any{
		"file": "main:ledger.json", "ledger_id": "main", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner",
	}})
	if err != nil || created.IsError {
		t.Fatalf("base init failed: %s %v", toolText(created), err)
	}
	ledgerPath := filepath.Join(ledgerRoot, "ledger.json")
	before, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure("question_add", "question.key", &sdk.CallToolParams{Name: "question_add", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-secret", "revision": datedRevision("qr-secret"),
		"initial_forecast": sealedForecast("f-secret"), "initial_secret_input_file": "keys:invalid.yaml", "key_file": "keys:question.key",
	}})
	after, err := os.ReadFile(ledgerPath)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed MCP question add changed ledger: %v", err)
	}

	added, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "question_add", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-one", "revision": datedRevision("qr-one"),
	}})
	if err != nil || added.IsError {
		t.Fatalf("base question failed: %s %v", toolText(added), err)
	}
	before, err = os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure("forecast_seal", "forecast.key", &sdk.CallToolParams{Name: "forecast_seal", Arguments: map[string]any{
		"file": "main:ledger.json", "question": "q-one", "forecast": "f-one", "question_revision_id": "qr-one", "forecasted_at": "2026-09-22T00:00:00Z",
		"secret_input_file": "keys:invalid.yaml", "key_file": "keys:forecast.key",
	}})
	after, err = os.ReadFile(ledgerPath)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed MCP forecast seal changed ledger: %v", err)
	}

	validPrivate := []byte("representations:\n  - kind: probability\n    outcome: true\n    probability: '0.7'\n")
	if err := storage.CreateProtectedFile(filepath.Join(secretRoot, "minimal.yaml"), validPrivate); err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"ledger_init", map[string]any{
			"file": "main:minimal.json", "ledger_id": "minimal", "timezone": "UTC", "forecaster_id": "owner", "forecaster_name": "Owner",
			"question":                  map[string]any{"id": "q-minimal", "revision": datedRevision("qr-minimal"), "initial_forecast": sealedForecast("f-minimal")},
			"initial_secret_input_file": "keys:minimal.yaml", "key_file": "keys:minimal-init.key",
		}},
		{"question_add", map[string]any{
			"file": "main:ledger.json", "question": "q-secret", "revision": datedRevision("qr-secret"),
			"initial_forecast": sealedForecast("f-secret"), "initial_secret_input_file": "keys:minimal.yaml", "key_file": "keys:minimal-question.key",
		}},
		{"forecast_seal", map[string]any{
			"file": "main:ledger.json", "question": "q-one", "forecast": "f-one", "question_revision_id": "qr-one", "forecasted_at": "2026-09-22T00:00:00Z",
			"secret_input_file": "keys:minimal.yaml", "key_file": "keys:minimal-forecast.key",
		}},
	} {
		result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError || strings.Contains(toolText(result), "0.7") {
			t.Fatalf("minimal %s result=%s err=%v", call.name, toolText(result), err)
		}
	}
}

func TestMCPActivityCoveragePresentation(t *testing.T) {
	ledgerRoot := t.TempDir()
	for _, coverage := range []service.ActivityCoverage{service.ActivityPartial, service.ActivityVerified} {
		writeMCPActivityCoverageFixture(t, ledgerRoot, string(coverage)+".json", coverage)
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	for _, coverage := range []service.ActivityCoverage{service.ActivityPartial, service.ActivityVerified} {
		result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_show", Arguments: map[string]any{
			"file": "main:" + string(coverage) + ".json", "question": "q-lifecycle-vector", "forecast": "f-lifecycle-vector-1",
		}})
		text := toolText(result)
		if err != nil || result.IsError || !strings.Contains(text, `"coverage":"`+string(coverage)+`"`) || !strings.Contains(text, `"active":true`) {
			t.Fatalf("coverage %s result=%s err=%v", coverage, text, err)
		}
	}
}

func TestMCPForecastInspectionAndResourceKeepPublicCommitmentAndRedactKey(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.Conformance(), "tests/conformance/valid/revealed-representation-only.json")
	if err != nil {
		t.Fatal(err)
	}
	var revealed ledger.Ledger
	if err := json.Unmarshal(raw, &revealed); err != nil {
		t.Fatal(err)
	}
	sealed := revealed
	sealed.Questions = append([]ledger.Question(nil), revealed.Questions...)
	sealed.Questions[0].Forecasts = append([]ledger.Forecast(nil), revealed.Questions[0].Forecasts...)
	forecast := &sealed.Questions[0].Forecasts[0]
	commitment := forecast.Commitment.Revealed
	forecast.Visibility = ledger.VisibilitySealed
	forecast.Representations = nil
	forecast.Commitment = &ledger.Commitment{Sealed: &ledger.SealedCommitment{
		Scheme: commitment.Scheme, CommitmentHash: commitment.CommitmentHash,
		Encryption: commitment.Encryption, KeyHint: commitment.KeyHint,
	}}
	root := t.TempDir()
	write := func(name string, model ledger.Ledger) {
		t.Helper()
		encoded, marshalErr := json.Marshal(model)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(filepath.Join(root, name), encoded, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	write("sealed.json", sealed)
	write("revealed.json", revealed)
	server, err := New(Config{LedgerRoots: []string{"main=" + root}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	const nonce = "MzMzMzMzMzMzMzMz"
	const ciphertextPrefix = "9Go4775177r8oTwO"
	const rawKey = "2222222222222222222222222222222222222222222222222222222222222222"
	for _, testCase := range []struct {
		name     string
		revealed bool
	}{
		{name: "sealed.json"},
		{name: "revealed.json", revealed: true},
	} {
		result, callErr := callToolForTest(t, client, &sdk.CallToolParams{Name: "forecast_show", Arguments: map[string]any{
			"file": "main:" + testCase.name, "question": "q-seal-presence", "forecast": "f-seal-presence-1",
		}})
		if callErr != nil || result.IsError {
			t.Fatalf("forecast_show %s result=%s err=%v", testCase.name, toolText(result), callErr)
		}
		toolJSON := toolText(result)
		resource, resourceErr := client.ReadResource(t.Context(), &sdk.ReadResourceParams{URI: "forecast-ledger://v1/forecast/main/" + testCase.name + "?question=q-seal-presence&forecast=f-seal-presence-1"})
		if resourceErr != nil || len(resource.Contents) != 1 {
			t.Fatalf("forecast resource %s = %#v, %v", testCase.name, resource, resourceErr)
		}
		for surface, value := range map[string]string{"tool": toolJSON, "resource": resource.Contents[0].Text} {
			if !strings.Contains(value, nonce) || !strings.Contains(value, ciphertextPrefix) || strings.Contains(value, rawKey) {
				t.Fatalf("%s %s public/redacted commitment = %s", testCase.name, surface, value)
			}
			if testCase.revealed && !strings.Contains(value, `"revealed_key_redacted":true`) {
				t.Fatalf("%s %s omitted key redaction marker: %s", testCase.name, surface, value)
			}
		}
	}
}

func writeMCPActivityCoverageFixture(t *testing.T, directory, name string, coverage service.ActivityCoverage) {
	t.Helper()
	raw, err := fs.ReadFile(contractschema.Conformance(), "tests/conformance/valid/lifecycle-checkpoints.json")
	if err != nil {
		t.Fatal(err)
	}
	var model ledger.Ledger
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	forecast := &model.Questions[0].Forecasts[0]
	checkpoints := append([]ledger.ActivityCheckpoint(nil), (*forecast.ActivityCheckpoints)...)
	if coverage == service.ActivityPartial {
		checkpoints = checkpoints[:1]
	} else {
		checkpoint := &checkpoints[len(checkpoints)-1]
		target := checkpoint.Integrity.Retained.Target
		genTime := ledger.Timestamp("2026-09-04T10:00:06Z")
		policy, serial := "1.2.3", "1"
		caPath := ledger.RelativePath("trust/example.pem")
		checkpoint.Integrity = ledger.LifecycleIntegrity{Verified: &ledger.VerifiedLifecycleIntegrity{
			Status: ledger.IntegrityVerified, Target: target, VerifiedAt: "2026-09-04T10:00:07Z",
			Timestamps: []ledger.RFC3161Timestamp{{Type: "rfc3161", RequestPath: "proofs/timestamps/request.tsq", ResponsePath: "proofs/timestamps/response.tsr", TSAURL: "https://tsa.example.test/stamp", HashAlgorithm: "sha256", State: ledger.RFC3161Verified, GenTime: &genTime, PolicyOID: &policy, SerialNumber: &serial, CABundlePath: &caPath}},
		}}
	}
	forecast.ActivityCheckpoints = &checkpoints
	encoded, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	vectorRaw, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-lifecycle-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Checkpoints []struct {
			HeadEventID ledger.Slug `json:"head_event_id"`
			Expected    struct {
				CanonicalTarget string `json:"canonical_target"`
			} `json:"expected"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(vectorRaw, &vector); err != nil {
		t.Fatal(err)
	}
	targets := make(map[ledger.Slug]string, len(vector.Checkpoints))
	for _, item := range vector.Checkpoints {
		targets[item.HeadEventID] = item.Expected.CanonicalTarget
	}
	for _, checkpoint := range checkpoints {
		var target ledger.LifecycleTarget
		if checkpoint.Integrity.Retained != nil {
			target = checkpoint.Integrity.Retained.Target
		} else if checkpoint.Integrity.Failed != nil {
			target = *checkpoint.Integrity.Failed.Target
		} else {
			target = checkpoint.Integrity.Verified.Target
		}
		absolute := filepath.Join(directory, filepath.FromSlash(string(target.ArtifactPath)))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(targets[checkpoint.HeadEventID]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMCPValidatesEveryPublishedV2LedgerFixture(t *testing.T) {
	ledgerRoot := t.TempDir()
	server, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, t.Context(), server)
	defer client.Close()
	for index, relative := range []string{
		"examples/valid/empty-ledger.json",
		"examples/valid/individual-ledger.json",
		"examples/valid/question-without-forecasts.yaml",
		"examples/valid/team-ledger.yaml",
		"tests/conformance/valid/relationships-and-datetime.json",
		"tests/conformance/valid/revealed-representation-only.json",
	} {
		name := fmt.Sprintf("fixture-%d%s", index, filepath.Ext(relative))
		copyFixture(t, filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.2.0", filepath.FromSlash(relative)), filepath.Join(ledgerRoot, name))
		result, err := callToolForTest(t, client, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:" + name}})
		if err != nil || result.IsError || !strings.Contains(toolText(result), `"code":"ledger.valid"`) {
			t.Fatalf("fixture %s: result=%s err=%v", relative, toolText(result), err)
		}
	}
}

func TestMCPRevealDiscoveryReadOnlyOfflineAndRootValidation(t *testing.T) {
	ledgerRoot, secretRoot := t.TempDir(), t.TempDir()
	if _, err := New(Config{LedgerRoots: []string{ledgerRoot}}); err == nil {
		t.Fatal("unnamed ledger root succeeded")
	} else if applicationErr, ok := err.(*app.Error); !ok || applicationErr.Details["class"] != service.RootLedger || applicationErr.Details["flag"] != "--ledger-root" {
		t.Fatalf("unnamed root diagnostic = %#v", err)
	}
	reveal, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Mode: service.AccessMode{AllowReveal: true}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := connectClient(t, ctx, reveal)
	defer client.Close()
	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tool := range listed.Tools {
		found = found || tool.Name == "forecast_reveal"
	}
	if !found {
		t.Fatal("reveal was not discovered after explicit enablement")
	}

	if _, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, SecretRoots: []string{"keys=" + secretRoot}, Mode: service.AccessMode{AllowReveal: true, ReadOnly: true}}); err == nil {
		t.Fatal("contradictory reveal/read-only configuration succeeded")
	}
	if _, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Mode: service.AccessMode{AllowReveal: true}}); err == nil {
		t.Fatal("reveal without secret root succeeded")
	}
	inside := filepath.Join(ledgerRoot, "inside")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, OutputRoots: []string{"packages=" + inside}}); err == nil {
		t.Fatal("overlapping roots succeeded")
	} else if applicationErr, ok := err.(*app.Error); !ok || applicationErr.Details["first_route"] != "ledger:main" || applicationErr.Details["second_route"] != "output:packages" {
		t.Fatalf("overlap diagnostic = %#v", err)
	}

	readOnly, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, Mode: service.AccessMode{ReadOnly: true, Offline: true}})
	if err != nil {
		t.Fatal(err)
	}
	client2 := connectClient(t, ctx, readOnly)
	defer client2.Close()
	readOnlyTools, err := client2.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range readOnlyTools.Tools {
		definition, ok := operationDefinitionByTool(tool.Name)
		if !ok || definition.Policy.PersistentEffect {
			t.Fatalf("mutating or unknown tool %s discovered in read-only mode", tool.Name)
		}
		if !strings.Contains(tool.Description, "effect=read-only") || !strings.Contains(tool.Description, "server_access=read-only") {
			t.Fatalf("read-only tool description is ambiguous: %q", tool.Description)
		}
	}
	if _, err := client2.CallTool(ctx, &sdk.CallToolParams{Name: "platform_add", Arguments: map[string]any{"file": "main:missing.json", "platform": "x", "name": "X", "kind": "informal"}}); err == nil {
		t.Fatal("read-only mutation direct call did not return unknown-tool protocol error")
	}

	limited, err := New(Config{LedgerRoots: []string{"main=" + ledgerRoot}, MaxConcurrent: 1, MaxToolBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	client3 := connectClient(t, ctx, limited)
	defer client3.Close()
	limited.sem <- struct{}{}
	busy, err := client3.CallTool(ctx, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:missing.json"}})
	<-limited.sem
	if err != nil || !busy.IsError || !strings.Contains(toolText(busy), "concurrent request limit") {
		t.Fatalf("concurrency limit result=%s err=%v", toolText(busy), err)
	}
	tooLarge, err := client3.CallTool(ctx, &sdk.CallToolParams{Name: "ledger_validate", Arguments: map[string]any{"file": "main:" + strings.Repeat("a", 2048)}})
	if err != nil || !tooLarge.IsError || !strings.Contains(toolText(tooLarge), "size limit") {
		t.Fatalf("argument limit result=%s err=%v", toolText(tooLarge), err)
	}
}

func TestMCPRealProcessStdioIsProtocolCleanAndShutsDownOnEOF(t *testing.T) {
	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=TestMCPHelperProcess")
	command.Env = append(os.Environ(), "FORECAST_LEDGER_MCP_HELPER=1", "FORECAST_LEDGER_MCP_ROOT="+root)
	var stderr strings.Builder
	command.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "real-process-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command, TerminateDuration: 10 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) == 0 {
		t.Fatalf("tools=%d err=%v", len(listed.Tools), err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "" {
		t.Fatalf("unexpected server diagnostic output: %q", stderr.String())
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("FORECAST_LEDGER_MCP_HELPER") != "1" {
		return
	}
	server, err := New(Config{LedgerRoots: []string{"main=" + os.Getenv("FORECAST_LEDGER_MCP_ROOT")}, Stderr: os.Stderr})
	if err == nil {
		err = server.ServeStdio(context.Background())
	}
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func FuzzMCPToolArguments(f *testing.F) {
	f.Add([]byte(`{"file":"main:ledger.json"}`))
	f.Add([]byte(`{"file":null,"unknown":true}`))
	allowed := map[string]bool{"file": true, "question": true, "forecast": true, "dry_run": true}
	definition := service.OperationDefinition{Name: service.OperationLedgerValidate}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		_, _ = decodeToolArguments(data, 64<<10, definition, allowed, []string{"file"})
	})
}

func callToolForTest(t *testing.T, client *sdk.ClientSession, params *sdk.CallToolParams) (*sdk.CallToolResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return client.CallTool(ctx, params)
}

func connectClient(t *testing.T, ctx context.Context, server *Server) *sdk.ClientSession {
	t.Helper()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "forecast-ledger-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func copyFixture(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsName(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func operationDefinitionByTool(name string) (service.OperationDefinition, bool) {
	for _, definition := range service.OperationDefinitions() {
		if definition.MCPTool == name {
			return definition, true
		}
	}
	return service.OperationDefinition{}, false
}

func toolText(result *sdk.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if text, ok := result.Content[0].(*sdk.TextContent); ok {
		return text.Text
	}
	return ""
}

func binaryRevision(id, title, criteria, expected string) map[string]any {
	return map[string]any{
		"id": id, "title": title, "resolution_criteria": criteria, "expected_resolution_at": expected,
		"outcome_space": map[string]any{"kind": "binary"}, "domain": map[string]any{"kind": "binary"},
	}
}

func probabilityRepresentations(probability string) []any {
	return []any{map[string]any{"kind": "probability", "outcome": true, "probability": probability}}
}

func minimumToolArguments(name string) map[string]any {
	arguments := map[string]any{"file": "main:missing.json"}
	if strings.HasPrefix(name, "platform_") {
		arguments["platform"] = "p-one"
	}
	if strings.HasPrefix(name, "question_") {
		arguments["question"] = "q-one"
	}
	if strings.HasPrefix(name, "group_") {
		arguments["group"] = "group-one"
	}
	if name == "relationship_show" || name == "relationship_remove" {
		arguments["relationship_id"] = "rel-one"
	}
	if strings.HasPrefix(name, "forecast_") || strings.HasPrefix(name, "timestamp_") {
		arguments["question"], arguments["forecast"] = "q-one", "f-one"
	}
	switch name {
	case "ledger_init":
		arguments["file"], arguments["ledger_id"], arguments["timezone"] = "main:new.json", "ledger-one", "UTC"
		arguments["forecaster_id"], arguments["forecaster_name"] = "me", "Me"
	case "platform_add":
		arguments["name"], arguments["kind"] = "Platform", "internal"
	case "question_add":
		arguments["revision"] = binaryRevision("qr-one", "Question", "Criteria", "2030-01-01T00:00:00Z")
	case "question_revise":
		arguments["id"], arguments["title"], arguments["resolution_criteria"], arguments["expected_resolution_at"] = "qr-two", "Question", "Criteria", "2030-01-01T00:00:00Z"
		arguments["outcome_space"], arguments["domain"] = map[string]any{"kind": "binary"}, map[string]any{"kind": "binary"}
	case "question_resolve":
		arguments["question_revision_id"], arguments["outcome"], arguments["outcome_known_at"], arguments["sources"], arguments["confirm"] = "qr-one", true, "2030-01-01T00:00:00Z", []any{}, true
	case "question_ambiguous", "question_void", "question_dispute":
		arguments["reason"], arguments["confirm"] = "Reason", true
	case "question_not_applicable":
		arguments["relationship_id"], arguments["reason"], arguments["confirm"] = "rel-one", "Reason", true
	case "forecast_add":
		arguments["question_revision_id"], arguments["representations"] = "qr-one", probabilityRepresentations("0.5")
	case "forecast_seal":
		arguments["question_revision_id"], arguments["secret_input_file"], arguments["key_file"] = "qr-one", "keys:missing.json", "keys:new.key"
	case "forecast_withdraw", "forecast_expire", "forecast_reaffirm":
		arguments["id"], arguments["effective_at"] = "event-one", "2030-01-01T00:00:00Z"
	case "forecast_reveal":
		arguments["key_file"], arguments["confirm"] = "keys:missing.key", true
	case "forecast_key_hint_update":
		arguments["key_hint"] = "vault:item"
	case "publication_build":
		arguments["output"] = "packages:new-package"
	case "publication_verify":
		arguments["manifest"] = "packages:missing-manifest.json"
	case "timestamp_stamp":
		arguments["tsa_url"], arguments["ca_bundle"] = "https://tsa.example.test", "tsa.pem"
	case "group_add":
		arguments["title"] = "Group"
	case "relationship_add":
		arguments["relationship"] = map[string]any{"id": "rel-one", "kind": "group_membership", "group_id": "group-one", "question_id": "q-one"}
	}
	if name == "platform_list" {
		delete(arguments, "platform")
	}
	if name == "question_list" {
		delete(arguments, "question")
	}
	if name == "forecast_list" {
		delete(arguments, "forecast")
	}
	if name == "group_list" {
		delete(arguments, "group")
	}
	if name == "platform_remove" || name == "group_remove" || name == "relationship_remove" || name == "question_resolve" || name == "question_dispute" {
		arguments["confirm"] = true
	}
	return arguments
}
