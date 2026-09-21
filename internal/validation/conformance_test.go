package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/document"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestAllPinnedInvalidCasesAreRejected(t *testing.T) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/invalid-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name       string `json:"name"`
		Base       string `json:"base"`
		Operations []struct {
			Op    string `json:"op"`
			Path  string `json:"path"`
			Value any    `json:"value"`
		} `json:"operations"`
		ExpectContains string `json:"expect_contains"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 26 {
		t.Fatalf("got %d invalid cases, want 26", len(cases))
	}
	expectedSemanticCode := map[string]string{
		"outcome-space-domain-mismatch":          "semantic.domain_kind",
		"pmf-probabilities-do-not-sum":           "semantic.probability_sum",
		"pmf-missing-option":                     "semantic.pmf_coverage",
		"option-set-version-mismatch":            "semantic.option_set_ref",
		"bin-gap":                                "semantic.bin_continuity",
		"bin-overlap":                            "semantic.bin_continuity",
		"binned-pmf-tails-do-not-sum":            "semantic.binned_probability_sum",
		"noncanonical-numeric-value":             "semantic.point_domain",
		"quantiles-not-monotonic":                "semantic.quantile_value_order",
		"cdf-not-monotonic":                      "semantic.cdf_probability_order",
		"unknown-platform-provenance":            "semantic.unknown_platform",
		"unknown-group-reference":                "semantic.relationship_group",
		"duplicate-question-id":                  "semantic.duplicate_question_id",
		"forecast-before-bound-revision":         "semantic.forecast_revision_chronology",
		"tampered-revealed-probability":          "semantic.revealed_mirror",
		"resolved-outcome-outside-domain":        "semantic.resolution_outcome",
		"invalid-lifecycle-transition":           "semantic.lifecycle_transition",
		"conditional-reference-cycle":            "semantic.relationship_cycle",
		"not-applicable-condition-was-satisfied": "semantic.not_applicable_satisfied",
		"rfc3161-timestamp-after-known-outcome":  "semantic.timestamp_chronology",
	}

	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			base := loadValidLedgerDocument(t, testCase.Base).Root.Any()
			for _, operation := range testCase.Operations {
				if err := applyFixtureOperation(base, operation.Op, operation.Path, normalizeFixtureNumber(operation.Value)); err != nil {
					t.Fatal(err)
				}
			}
			schemaIssues, semanticIssues := validateGenericDocument(t, base)
			if len(schemaIssues) == 0 && len(semanticIssues) == 0 {
				t.Fatal("invalid upstream fixture was accepted")
			}
			if code, semantic := expectedSemanticCode[testCase.Name]; semantic {
				if len(schemaIssues) != 0 || !hasSemanticCode(semanticIssues, code) {
					t.Fatalf("wanted semantic code %s, got schema=%#v semantic=%#v", code, schemaIssues, semanticIssues)
				}
			} else if len(schemaIssues) == 0 {
				t.Fatalf("wanted schema rejection for %s (%s), got semantic=%#v", testCase.Name, testCase.ExpectContains, semanticIssues)
			}
		})
	}
}

func loadValidLedgerDocument(t *testing.T, name string) *document.Document {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), name)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(name, ".json") {
		parsed, err := document.ParseJSON(bytes.NewReader(data), document.DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	parsed, err := document.ParseYAML(bytes.NewReader(data), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func validateGenericDocument(t *testing.T, value any) ([]SchemaIssue, []SemanticIssue) {
	t.Helper()
	structural, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	schemaIssues, err := structural.Validate(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(schemaIssues) > 0 {
		return schemaIssues, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	model, err := DecodeLedger(parsed)
	if err != nil {
		t.Fatal(err)
	}
	semanticIssues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	return nil, semanticIssues
}

func applyFixtureOperation(root any, operation, pointer string, value any) error {
	if operation != "replace" && operation != "add" && operation != "remove" {
		return fmt.Errorf("unsupported pinned fixture operation %q", operation)
	}
	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for index, token := range tokens {
		tokens[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
	}
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		switch container := current.(type) {
		case map[string]any:
			current = container[token]
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(container) {
				return fmt.Errorf("invalid fixture pointer %q", pointer)
			}
			current = container[index]
		default:
			return fmt.Errorf("invalid fixture pointer %q", pointer)
		}
	}
	last := tokens[len(tokens)-1]
	switch container := current.(type) {
	case map[string]any:
		if operation == "remove" {
			delete(container, last)
		} else {
			container[last] = value
		}
	case []any:
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 || index > len(container) || (index == len(container) && operation != "add") {
			return fmt.Errorf("invalid fixture pointer %q", pointer)
		}
		if operation == "add" && index == len(container) {
			container = append(container, value)
			return replaceContainerAtPointer(root, tokens[:len(tokens)-1], container)
		}
		if operation == "remove" {
			copy(container[index:], container[index+1:])
			container[len(container)-1] = nil
			container = container[:len(container)-1]
			// The fixture corpus only removes array members through a map-owned
			// array, so update it while traversing instead of silently retaining
			// the old slice length.
			return replaceContainerAtPointer(root, tokens[:len(tokens)-1], container)
		}
		container[index] = value
	default:
		return fmt.Errorf("invalid fixture pointer %q", pointer)
	}
	return nil
}

func replaceContainerAtPointer(root any, tokens []string, replacement []any) error {
	if len(tokens) == 0 {
		return fmt.Errorf("cannot replace fixture root array")
	}
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		switch container := current.(type) {
		case map[string]any:
			current = container[token]
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(container) {
				return fmt.Errorf("invalid fixture pointer")
			}
			current = container[index]
		default:
			return fmt.Errorf("invalid fixture pointer")
		}
	}
	last := tokens[len(tokens)-1]
	switch container := current.(type) {
	case map[string]any:
		container[last] = replacement
	case []any:
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 || index >= len(container) {
			return fmt.Errorf("invalid fixture pointer")
		}
		container[index] = replacement
	default:
		return fmt.Errorf("invalid fixture pointer")
	}
	return nil
}

func normalizeFixtureNumber(value any) any {
	switch value := value.(type) {
	case json.Number:
		integer, err := value.Int64()
		if err != nil {
			return value.String()
		}
		return integer
	case []any:
		for index := range value {
			value[index] = normalizeFixtureNumber(value[index])
		}
		return value
	case map[string]any:
		for key := range value {
			value[key] = normalizeFixtureNumber(value[key])
		}
		return value
	default:
		return value
	}
}
