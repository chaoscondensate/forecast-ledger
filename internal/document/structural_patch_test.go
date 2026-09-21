package document

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestApplyPatchPreservesUnchangedJSONBytesAcrossStructuralEdits(t *testing.T) {
	input := "{\r\n  \"title\": \"Old\",\r\n  \"platforms\": {\"old\": {\"kind\":\"informal\"}},\r\n  \"questions\": [{\"id\":\"q-one\"}],\r\n  \"untouched\" : { \"odd spacing\" : true }\r\n}\r\n"
	doc, err := ParseJSON(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchReplace, Pointer: "/title", Value: "New"},
		{Kind: PatchAdd, Pointer: "/platforms/new", Value: map[string]any{"kind": "internal"}},
		{Kind: PatchRemove, Pointer: "/platforms/old"},
		{Kind: PatchAdd, Pointer: "/questions/-", Value: map[string]any{"id": "q-two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	if !strings.Contains(output, `"untouched" : { "odd spacing" : true }`) || !strings.Contains(output, "\r\n") {
		t.Fatalf("untouched JSON presentation changed:\n%s", output)
	}
	if strings.Contains(output, `"old"`) || !strings.Contains(output, `"new"`) || !strings.Contains(output, `"q-two"`) {
		t.Fatalf("structural JSON edits missing:\n%s", output)
	}
}

func TestApplyPatchPreservesYAMLCommentsOrderStyleAndCRLF(t *testing.T) {
	input := "# ledger\r\ntitle: 'Old' # title\r\nplatforms:\r\n  old: {kind: informal}\r\nquestions:\r\n  - id: q-one\r\nuntouched: |\r\n  keep this\r\n# tail\r\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchReplace, Pointer: "/title", Value: "New's"},
		{Kind: PatchAdd, Pointer: "/platforms/new", Value: map[string]any{"kind": "internal"}},
		{Kind: PatchRemove, Pointer: "/platforms/old"},
		{Kind: PatchAdd, Pointer: "/questions/-", Value: map[string]any{"id": "q-two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	for _, unchanged := range []string{"# ledger\r\n", " # title\r\n", "untouched: |\r\n  keep this\r\n", "# tail\r\n"} {
		if !strings.Contains(output, unchanged) {
			t.Fatalf("untouched YAML bytes %q changed:\n%s", unchanged, output)
		}
	}
	if !strings.Contains(output, "title: 'New''s'") || strings.Contains(output, "  old:") || !strings.Contains(output, "  new:") || !strings.Contains(output, "q-two") {
		t.Fatalf("structural YAML edits missing:\n%s", output)
	}
}

func TestApplyPatchCanAddRemoveOptionalFieldsAndReplaceSubtrees(t *testing.T) {
	input := "root:\n  keep: yes\n  remove: old\n  replace: {old: value}\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchRemove, Pointer: "/root/remove"},
		{Kind: PatchAdd, Pointer: "/root/added", Value: "new"},
		{Kind: PatchReplace, Pointer: "/root/replace", Value: map[string]any{"new": "value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	if !strings.Contains(output, "  keep: yes\n") || strings.Contains(output, "remove:") || !strings.Contains(output, "added: new") || !strings.Contains(output, "replace:\n    new: value") {
		t.Fatalf("unexpected patched YAML:\n%s", output)
	}
}

func TestApplyPatchReplacesNormalizedOrderedScalarsInBlockYAML(t *testing.T) {
	input := "root:\n  status: open\n  recorded_at: 'old'\n  boolean_like: 'false'\n  numeric_like: '100'\n  null_like: 'null'\n  flag: false\n  count: 1\n  nullable: old\nrecords:\n  - status: open\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchReplace, Pointer: "/root/status", Value: orderedJSONValue(t, `"closed"`)},
		{Kind: PatchReplace, Pointer: "/root/recorded_at", Value: orderedJSONValue(t, `"2026-08-30T20:30:00Z"`)},
		{Kind: PatchReplace, Pointer: "/root/boolean_like", Value: orderedJSONValue(t, `"true"`)},
		{Kind: PatchReplace, Pointer: "/root/numeric_like", Value: orderedJSONValue(t, `"1234"`)},
		{Kind: PatchReplace, Pointer: "/root/null_like", Value: orderedJSONValue(t, `"null"`)},
		{Kind: PatchReplace, Pointer: "/root/flag", Value: orderedJSONValue(t, `true`)},
		{Kind: PatchReplace, Pointer: "/root/count", Value: orderedJSONValue(t, `42`)},
		{Kind: PatchReplace, Pointer: "/root/nullable", Value: orderedJSONValue(t, `null`)},
		{Kind: PatchReplace, Pointer: "/records/0/status", Value: orderedJSONValue(t, `"closed"`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseYAML(strings.NewReader(string(got)), DefaultLimits)
	if err != nil {
		t.Fatalf("patched YAML does not parse: %v\n%s", err, got)
	}
	wants := map[string]any{
		"/root/status":       "closed",
		"/root/recorded_at":  "2026-08-30T20:30:00Z",
		"/root/boolean_like": "true",
		"/root/numeric_like": "1234",
		"/root/null_like":    "null",
		"/root/flag":         true,
		"/root/count":        int64(42),
		"/root/nullable":     nil,
		"/records/0/status":  "closed",
	}
	for pointer, want := range wants {
		value, lookupErr := lookupValue(parsed.Root, pointer)
		if lookupErr != nil {
			t.Fatalf("lookup %s: %v", pointer, lookupErr)
		}
		if fmt.Sprint(value.Any()) != fmt.Sprint(want) {
			t.Errorf("%s = %#v, want %#v", pointer, value.Any(), want)
		}
	}
}

func TestApplyPatchReplacesCollectionsAcrossYAMLContexts(t *testing.T) {
	input := "# keep heading\nroot:\n  block_map:\n    old: value\n  block_sequence:\n    - old\n  flow_map: {old: value}\n  flow_sequence: [old]\n  untouched: {keep: flow}\nrecords:\n  - id: one\n    details:\n      old: value\nblock_items:\n  - old: value\nflow_items:\n  - {old: value}\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchReplace, Pointer: "/root/block_map", Value: orderedJSONValue(t, `{"first":"one","second":{"nested":true}}`)},
		{Kind: PatchReplace, Pointer: "/root/block_sequence", Value: orderedJSONValue(t, `[{"id":"one"},{"id":"two"}]`)},
		{Kind: PatchReplace, Pointer: "/root/flow_map", Value: orderedJSONValue(t, `{"first":"one","second":"two"}`)},
		{Kind: PatchReplace, Pointer: "/root/flow_sequence", Value: orderedJSONValue(t, `["one","two"]`)},
		{Kind: PatchReplace, Pointer: "/records/0/details", Value: orderedJSONValue(t, `{"first":"one","second":"two"}`)},
		{Kind: PatchReplace, Pointer: "/block_items/0", Value: orderedJSONValue(t, `{"first":"one","second":"two"}`)},
		{Kind: PatchReplace, Pointer: "/flow_items/0", Value: orderedJSONValue(t, `{"first":"one","second":"two"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	for _, want := range []string{
		"# keep heading\n",
		"  block_map:\n    first: one\n    second:\n      nested: true\n",
		"  block_sequence:\n    - id: one\n    - id: two\n",
		"  flow_map:\n    first: one\n    second: two\n",
		"  flow_sequence:\n    - one\n    - two\n",
		"  untouched: {keep: flow}\n",
		"    details:\n      first: one\n      second: two\n",
		"block_items:\n  - first: one\n    second: two\n",
		"flow_items:\n  - first: one\n    second: two\n",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in replacement output:\n%s", want, output)
		}
	}
	if _, err := ParseYAML(strings.NewReader(output), DefaultLimits); err != nil {
		t.Fatalf("patched YAML does not parse: %v\n%s", err, output)
	}
}

func TestApplyPatchStructuralReplacementPreservesUnrelatedCRLFBytes(t *testing.T) {
	input := "# heading\r\nrecords:\r\n  - id: one\r\n    details:\r\n      old: value\r\n    untouched: 'quoted' # keep\r\n\r\n  - id: two\r\n    details: {keep: flow}\r\ntail: {keep: flow}\r\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{{Kind: PatchReplace, Pointer: "/records/0/details", Value: orderedJSONValue(t, `{"first":"one","empty":[],"nested":{"enabled":true}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	for _, unchanged := range []string{
		"# heading\r\n",
		"    untouched: 'quoted' # keep\r\n\r\n",
		"  - id: two\r\n    details: {keep: flow}\r\n",
		"tail: {keep: flow}\r\n",
	} {
		if !strings.Contains(output, unchanged) {
			t.Errorf("unrelated bytes %q changed:\n%s", unchanged, output)
		}
	}
	if !strings.Contains(output, "    details:\r\n      first: one\r\n      empty: []\r\n      nested:\r\n        enabled: true\r\n") {
		t.Fatalf("replacement is not expanded CRLF block YAML:\n%s", output)
	}
	if strings.Contains(strings.ReplaceAll(output, "\r\n", ""), "\n") {
		t.Fatalf("replacement introduced bare LF into CRLF document: %q", output)
	}
	if _, err := ParseYAML(strings.NewReader(output), DefaultLimits); err != nil {
		t.Fatalf("patched YAML does not parse: %v\n%s", err, output)
	}
}

func orderedJSONValue(t *testing.T, raw string) OrderedValue {
	t.Helper()
	doc, err := ParseJSON(strings.NewReader(raw), DefaultLimits)
	if err != nil {
		t.Fatalf("parse ordered JSON value %s: %v", raw, err)
	}
	return Ordered(doc.Root)
}

func TestApplyPatchExpandsPopulatedYAMLCollectionsButKeepsEmptyCollectionsCompact(t *testing.T) {
	input := "# review\nquestions: []\nplatforms: {}\nuntouched: {keep: flow}\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	question, err := ParseJSON(strings.NewReader(`{"id":"q-one","forecast_window":{"opens_at":"2030-08-10T00:00:00+01:00"},"forecasts":[]}`), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{
		{Kind: PatchAdd, Pointer: "/questions/-", Value: Ordered(question.Root)},
		{Kind: PatchAdd, Pointer: "/platforms/internal", Value: map[string]any{"kind": "internal", "name": "Team"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	for _, want := range []string{
		"# review\n",
		"questions:\n  - id: q-one\n",
		"    forecast_window:\n      opens_at:",
		"    forecasts: []\n",
		"platforms:\n  internal:\n    kind: internal\n    name: Team\n",
		"untouched: {keep: flow}\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in expanded YAML:\n%s", want, output)
		}
	}
	if strings.Contains(output, "questions: [") || strings.Contains(output, "internal: {") {
		t.Fatalf("populated collection used flow style:\n%s", output)
	}
}

func TestApplyPatchKeepsLargeExpandedYAMLLedgerReviewable(t *testing.T) {
	input := "# keep this review note\nquestions:\n  - id: q-one\n    forecasts:\n      - id: f-000\n        representations:\n          - kind: probability\n            outcome: true\n            probability: \"0.5\"\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]PatchOperation, 0, 30)
	for index := 1; index <= 30; index++ {
		operations = append(operations, PatchOperation{Kind: PatchAdd, Pointer: "/questions/0/forecasts/-", Value: map[string]any{
			"id":              fmt.Sprintf("f-%03d", index),
			"representations": []any{map[string]any{"kind": "point", "statistic": "mean", "value": fmt.Sprintf("%d", index)}},
			"key_factors":     []string{"first factor", "second factor"},
		}})
	}
	got, err := ApplyPatch(doc, operations)
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	if !strings.Contains(output, "# keep this review note\n") || strings.Contains(output, `{"id"`) {
		t.Fatalf("expanded YAML was collapsed into JSON fragments:\n%s", output)
	}
	for lineNumber, line := range strings.Split(output, "\n") {
		if len(line) > 200 {
			t.Fatalf("line %d has %d bytes; document is not reviewable", lineNumber+1, len(line))
		}
	}
}

func TestOrderedPatchValueKeepsDeclaredOrderInJSONAndYAML(t *testing.T) {
	valueDocument, err := ParseJSON(strings.NewReader(`{"id":"f-new","forecasted_at":"2026-08-26T12:00:00Z","recorded_at":"2026-08-26T12:01:00Z","visibility":"public","representations":[{"kind":"probability","outcome":true,"probability":"0.51"}],"integrity":{"status":"unanchored"}}`), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		input string
		parse func(io.Reader, Limits) (*Document, error)
	}{
		{name: "json", input: `{"forecasts": []}`, parse: ParseJSON},
		{name: "yaml", input: "forecasts: []\n", parse: ParseYAML},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := test.parse(strings.NewReader(test.input), DefaultLimits)
			if err != nil {
				t.Fatal(err)
			}
			output, err := ApplyPatch(doc, []PatchOperation{{Kind: PatchAdd, Pointer: "/forecasts/-", Value: Ordered(valueDocument.Root)}})
			if err != nil {
				t.Fatal(err)
			}
			text := string(output)
			positions := []int{strings.Index(text, "id"), strings.Index(text, "forecasted_at"), strings.Index(text, "recorded_at"), strings.Index(text, "visibility"), strings.Index(text, "representations"), strings.Index(text, "integrity")}
			for index := 1; index < len(positions); index++ {
				if positions[index-1] < 0 || positions[index] <= positions[index-1] {
					t.Fatalf("semantic order changed: %s", text)
				}
			}
		})
	}
}

func TestApplyPatchIndentsNewFragmentsInPrettyJSON(t *testing.T) {
	input := "{\n  \"questions\": [\n    {\n      \"id\": \"q-one\",\n      \"forecasts\": [\n        {\n          \"id\": \"f-zero\"\n        }\n      ]\n    }\n  ],\n  \"untouched\": { \"spacing\" : true }\n}\n"
	doc, err := ParseJSON(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{{Kind: PatchAdd, Pointer: "/questions/0/forecasts/-", Value: map[string]any{
		"id": "f-one", "representations": []any{map[string]any{"kind": "probability", "outcome": true, "probability": "0.5"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	output := string(got)
	if strings.Contains(output, `{"id":"f-one"`) || !strings.Contains(output, "        \"id\": \"f-one\"") || !strings.Contains(output, `"untouched": { "spacing" : true }`) {
		t.Fatalf("pretty JSON fragment or untouched bytes changed:\n%s", output)
	}
	if strings.Contains(output, "      }      ]") || !strings.Contains(output, "\n      ]") {
		t.Fatalf("closing array delimiter did not remain expanded:\n%s", output)
	}
}

func TestApplyPatchKeepsRepeatedJSONAdditionsExpanded(t *testing.T) {
	input := "{\n  \"forecasts\": [\n    {\n      \"id\": \"f-zero\"\n    }\n  ]\n}\n"
	doc, err := ParseJSON(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]PatchOperation, 0, 30)
	for index := 1; index <= 30; index++ {
		operations = append(operations, PatchOperation{Kind: PatchAdd, Pointer: "/forecasts/-", Value: map[string]any{
			"id": fmt.Sprintf("f-%03d", index), "representations": []any{map[string]any{"kind": "point", "statistic": "mean", "value": fmt.Sprintf("%d", index)}},
		}})
	}
	got, err := ApplyPatch(doc, operations)
	if err != nil {
		t.Fatal(err)
	}
	for lineNumber, line := range strings.Split(string(got), "\n") {
		if len(line) > 200 {
			t.Fatalf("line %d has %d bytes; repeated JSON additions collapsed", lineNumber+1, len(line))
		}
	}
	if count := strings.Count(string(got), `"id":`); count != 31 {
		t.Fatalf("got %d forecast IDs, want 31", count)
	}
	if strings.Contains(string(got), "\n,\n") {
		t.Fatalf("repeated additions put commas on separate lines:\n%s", got)
	}
}
