package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestOptionalTracksOmittedNullAndValue(t *testing.T) {
	var patch RootMetadataPatchInput
	if err := json.Unmarshal([]byte(`{"title":null,"description":"kept"}`), &patch); err != nil {
		t.Fatal(err)
	}
	if !patch.Title.Set || !patch.Title.Null {
		t.Fatalf("title state = %#v", patch.Title)
	}
	if !patch.Description.Set || patch.Description.Null || patch.Description.Value != "kept" {
		t.Fatalf("description state = %#v", patch.Description)
	}
	if patch.DefaultTimezone.Set {
		t.Fatalf("omitted field marked as set: %#v", patch.DefaultTimezone)
	}
}

func TestAllOperationInputSchemasCompileAndAreClosed(t *testing.T) {
	for _, name := range InputSchemaNames() {
		t.Run(string(name), func(t *testing.T) {
			schemaBytes, err := InputSchema(name)
			if err != nil {
				t.Fatal(err)
			}
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
			if err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			compiler.UseRegexpEngine(compileInputECMARegexp)
			if err := compiler.AddResource("schema.json", document); err != nil {
				t.Fatal(err)
			}
			if _, err := compiler.Compile("schema.json"); err != nil {
				t.Fatalf("schema does not compile: %v", err)
			}
			if !strings.Contains(string(schemaBytes), `"additionalProperties": false`) {
				t.Fatal("schema has no closed object")
			}
		})
	}
}

func TestInitAndQuestionAddHaveDeliberatelyDifferentIdentityInputs(t *testing.T) {
	initSchema := compileInputSchema(t, InputSchemaInit)
	questionSchema := compileInputSchema(t, InputSchemaQuestionAdd)

	initDocument := minimalQuestionDocument(true)
	if err := initSchema.Validate(initDocument); err != nil {
		t.Fatalf("init input rejected question identity: %v", err)
	}
	questionDocument := minimalQuestionDocument(false)
	if err := questionSchema.Validate(questionDocument["question"]); err != nil {
		t.Fatalf("question-add input rejected selector-owned identity: %v", err)
	}
	questionDocument["question"].(map[string]any)["id"] = "q-one"
	if err := questionSchema.Validate(questionDocument["question"]); err == nil {
		t.Fatal("question-add schema accepted duplicate request identity")
	}
}

func TestRelationshipInputSchemaKeepsWrapperAndLedgerUnionDistinct(t *testing.T) {
	schema := compileInputSchema(t, InputSchemaRelationship)
	valid := map[string]any{"relationship": map[string]any{
		"id": "rel-one", "kind": "group_membership", "group_id": "group-one", "question_id": "q-one",
	}}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("valid relationship input was rejected: %v", err)
	}
	if err := schema.Validate(map[string]any{"relationship": valid}); err == nil {
		t.Fatal("recursive relationship wrapper was accepted")
	}
	direct, err := DirectRequestSchema(InputSchemaRelationship)
	if err != nil {
		t.Fatal(err)
	}
	properties := direct["properties"].(map[string]any)
	if reference := properties["relationship"].(map[string]any)["$ref"]; reference != "#/$defs/relationship" {
		t.Fatalf("direct relationship reference = %v", reference)
	}
}

func compileInputSchema(t *testing.T, name InputSchemaName) *jsonschema.Schema {
	t.Helper()
	schemaBytes, err := InputSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(compileInputECMARegexp)
	if err := compiler.AddResource("schema.json", document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func minimalQuestionDocument(includeID bool) map[string]any {
	question := map[string]any{
		"revision": map[string]any{
			"id": "qr-one", "title": "Will it happen?", "resolution_criteria": "Resolve from the named source.",
			"expected_resolution_at": "2027-01-02T00:00:00Z",
			"outcome_space":          map[string]any{"kind": "binary"}, "domain": map[string]any{"kind": "binary"},
		},
		"initial_forecast": map[string]any{
			"id": "f-one", "visibility": "public", "forecasted_at": "2026-01-01T00:00:00Z",
			"representations": []any{map[string]any{"kind": "probability", "outcome": true, "probability": "0.5"}},
		},
	}
	if includeID {
		question["id"] = "q-one"
		return map[string]any{"question": question}
	}
	return map[string]any{"question": question}
}
