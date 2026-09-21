package validation

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/document"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEmbeddedSchemaCompilesAndValidatesBothFormats(t *testing.T) {
	validator, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name   string
		parse  func(string) (*document.Document, error)
		format string
	}{
		{name: "examples/valid/empty-ledger.json", format: "empty-json", parse: func(input string) (*document.Document, error) {
			return document.ParseJSON(strings.NewReader(input), document.DefaultLimits)
		}},
		{name: "examples/valid/individual-ledger.json", format: "json", parse: func(input string) (*document.Document, error) {
			return document.ParseJSON(strings.NewReader(input), document.DefaultLimits)
		}},
		{name: "examples/valid/question-without-forecasts.yaml", format: "backlog-yaml", parse: func(input string) (*document.Document, error) {
			return document.ParseYAML(strings.NewReader(input), document.DefaultLimits)
		}},
		{name: "examples/valid/team-ledger.yaml", format: "yaml", parse: func(input string) (*document.Document, error) {
			return document.ParseYAML(strings.NewReader(input), document.DefaultLimits)
		}},
		{name: "tests/conformance/valid/relationships-and-datetime.json", format: "relationships-json", parse: func(input string) (*document.Document, error) {
			return document.ParseJSON(strings.NewReader(input), document.DefaultLimits)
		}},
	}
	fixtureFS := contractschema.Conformance()
	for _, fixture := range fixtures {
		t.Run(fixture.format, func(t *testing.T) {
			data, err := fs.ReadFile(fixtureFS, fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := fixture.parse(string(data))
			if err != nil {
				t.Fatal(err)
			}
			issues, err := validator.Validate(parsed.Root.Any())
			if err != nil {
				t.Fatal(err)
			}
			if len(issues) != 0 {
				t.Fatalf("valid fixture has schema issues: %#v", issues)
			}
		})
	}
}

func TestFormatAssertionsRejectCalendarInvalidTimestamp(t *testing.T) {
	validator, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	documentValue := validIndividualDocument(t)
	documentValue["created_at"] = "2026-02-30T12:00:00Z"
	issues, err := validator.Validate(documentValue)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "/created_at", "schema.format") {
		t.Fatalf("format issue missing: %#v", issues)
	}
}

func TestSchemaIssuesAreDeterministicAndDoNotEchoValues(t *testing.T) {
	validator, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	documentValue := validIndividualDocument(t)
	const canary = "CANARY-PRIVATE-VALUE"
	documentValue["created_at"] = "2026-02-30T12:00:00Z-" + canary
	documentValue["unknown_"+canary] = canary

	var baseline []byte
	for index := 0; index < 100; index++ {
		issues, err := validator.Validate(documentValue)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(issues)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), canary) {
			t.Fatalf("schema issue leaked rejected value: %s", encoded)
		}
		if index == 0 {
			baseline = encoded
		} else if string(encoded) != string(baseline) {
			t.Fatalf("nondeterministic issues:\n%s\n%s", baseline, encoded)
		}
	}
}

func TestEmbeddedSchemaValidationIsConcurrent(t *testing.T) {
	validator, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	documentValue := validIndividualDocument(t)
	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			issues, validateErr := validator.Validate(documentValue)
			if validateErr != nil || len(issues) != 0 {
				t.Errorf("concurrent validation: issues=%#v err=%v", issues, validateErr)
			}
		}()
	}
	wait.Wait()
}

func TestSchemaCompilerRejectsMalformedMetadataAndExternalRefs(t *testing.T) {
	for _, schemaBytes := range [][]byte{
		[]byte(`not json`),
		[]byte(`{} trailing`),
		[]byte(`{"$schema":"wrong","$id":"` + contractID + `"}`),
		[]byte(`{"$schema":"` + draft2020 + `","$id":"wrong"}`),
	} {
		if _, err := compileStructuralValidator(schemaBytes); err == nil {
			t.Fatalf("invalid embedded schema accepted: %s", schemaBytes)
		}
	}

	contract := string(contractschema.Contract())
	withExternalRef := strings.Replace(contract, `"$ref": "#/$defs/forecaster"`, `"$ref": "https://invalid.example/private-schema.json"`, 1)
	_, err := compileStructuralValidator([]byte(withExternalRef))
	if err == nil {
		t.Fatal("external schema reference was accepted")
	}
	var loadErr *jsonschema.LoadURLError
	if !errors.As(err, &loadErr) {
		t.Fatalf("error does not contain LoadURLError: %v", err)
	}
	var offlineErr *externalReferenceError
	if !errors.As(loadErr.Err, &offlineErr) {
		t.Fatalf("loader cause is not externalReferenceError: %v", loadErr.Err)
	}
}

func validIndividualDocument(t *testing.T) map[string]any {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), "examples/valid/individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := document.ParseJSON(strings.NewReader(string(data)), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := parsed.Root.Any().(map[string]any)
	if !ok {
		t.Fatal("fixture root is not an object")
	}
	return result
}

func hasIssue(issues []SchemaIssue, pointer, code string) bool {
	for _, issue := range issues {
		if issue.InstanceLocation == pointer && issue.Code == code {
			return true
		}
	}
	return false
}
