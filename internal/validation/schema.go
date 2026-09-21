package validation

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/dlclark/regexp2"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	contractID = "https://raw.githubusercontent.com/chaoscondensate/schema/v2.0.0/schema/forecast-ledger.schema.json"
	draft2020  = "https://json-schema.org/draft/2020-12/schema"
)

type externalReferenceError struct {
	URL string
}

func (e *externalReferenceError) Error() string {
	return "external schema resolution is disabled: " + e.URL
}

type denyURLLoader struct{}

func (denyURLLoader) Load(url string) (any, error) {
	return nil, &externalReferenceError{URL: url}
}

type ecmaRegexp regexp2.Regexp

func (regexp *ecmaRegexp) MatchString(value string) bool {
	matched, err := (*regexp2.Regexp)(regexp).MatchString(value)
	return err == nil && matched
}

func (regexp *ecmaRegexp) String() string {
	return (*regexp2.Regexp)(regexp).String()
}

func compileECMARegexp(pattern string) (jsonschema.Regexp, error) {
	compiled, err := regexp2.Compile(pattern, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	compiled.MatchTimeout = 100 * time.Millisecond
	return (*ecmaRegexp)(compiled), nil
}

type StructuralValidator struct {
	schema *jsonschema.Schema
}

var loadDefault = sync.OnceValues(func() (*StructuralValidator, error) {
	return compileStructuralValidator(contractschema.Contract())
})

func DefaultStructuralValidator() (*StructuralValidator, error) {
	return loadDefault()
}

func compileStructuralValidator(schemaBytes []byte) (*StructuralValidator, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return nil, fmt.Errorf("decode embedded Forecast Ledger schema: %w", err)
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, errors.New("embedded Forecast Ledger schema must be a JSON object")
	}
	if got, _ := object["$schema"].(string); got != draft2020 {
		return nil, fmt.Errorf("embedded Forecast Ledger schema draft is %q, want %q", got, draft2020)
	}
	if got, _ := object["$id"].(string); got != contractID {
		return nil, fmt.Errorf("embedded Forecast Ledger schema id is %q, want %q", got, contractID)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(denyURLLoader{})
	// JSON Schema patterns use ECMA-262 syntax. The pinned contract includes
	// lookaheads that Go's RE2 syntax intentionally does not support.
	compiler.UseRegexpEngine(compileECMARegexp)
	if err := compiler.AddResource(contractID, document); err != nil {
		return nil, fmt.Errorf("register embedded Forecast Ledger schema: %w", err)
	}
	compiled, err := compiler.Compile(contractID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded Forecast Ledger schema: %w", err)
	}
	return &StructuralValidator{schema: compiled}, nil
}

type SchemaIssue struct {
	Layer            string `json:"layer"`
	Code             string `json:"code"`
	InstanceLocation string `json:"instance_location"`
	SchemaLocation   string `json:"schema_location"`
	Message          string `json:"message"`
}

func (v *StructuralValidator) Validate(document any) ([]SchemaIssue, error) {
	if v == nil || v.schema == nil {
		return nil, errors.New("structural validator is not initialized")
	}
	err := v.schema.Validate(document)
	if err == nil {
		return nil, nil
	}
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return nil, fmt.Errorf("run embedded schema validation: %w", err)
	}

	issues := make([]SchemaIssue, 0)
	collectSchemaIssues(validationErr, &issues)
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.InstanceLocation != b.InstanceLocation {
			return a.InstanceLocation < b.InstanceLocation
		}
		if a.SchemaLocation != b.SchemaLocation {
			return a.SchemaLocation < b.SchemaLocation
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	issues = compactIssues(issues)
	return issues, nil
}

func collectSchemaIssues(validationErr *jsonschema.ValidationError, issues *[]SchemaIssue) {
	if validationErr.ErrorKind != nil {
		keyword := "schema"
		if path := validationErr.ErrorKind.KeywordPath(); len(path) > 0 {
			keyword = path[0]
		}
		*issues = append(*issues, SchemaIssue{
			Layer:            "schema",
			Code:             "schema." + strings.TrimPrefix(keyword, "$"),
			InstanceLocation: pointerFromTokens(validationErr.InstanceLocation),
			SchemaLocation:   validationErr.SchemaURL,
			Message:          safeSchemaMessage(keyword),
		})
	}
	for _, cause := range validationErr.Causes {
		collectSchemaIssues(cause, issues)
	}
}

func pointerFromTokens(tokens []string) string {
	var pointer string
	for _, token := range tokens {
		token = strings.ReplaceAll(token, "~", "~0")
		token = strings.ReplaceAll(token, "/", "~1")
		pointer += "/" + token
	}
	return pointer
}

func safeSchemaMessage(keyword string) string {
	switch keyword {
	case "required":
		return "a required field is missing"
	case "additionalProperties", "unevaluatedProperties":
		return "an unknown field is not allowed"
	case "type":
		return "the value has the wrong type"
	case "const", "enum":
		return "the value is not allowed"
	case "format":
		return "the value does not match the required format"
	case "pattern":
		return "the value does not match the required pattern"
	case "minLength", "maxLength":
		return "the text length is outside the allowed range"
	case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
		return "the number is outside the allowed range"
	case "minItems", "maxItems", "uniqueItems", "contains", "minContains":
		return "the list does not meet the required shape"
	case "oneOf", "anyOf", "allOf", "not", "if":
		return "the value does not match the required schema branch"
	default:
		return "the value does not satisfy the schema rule"
	}
}

func compactIssues(issues []SchemaIssue) []SchemaIssue {
	if len(issues) < 2 {
		return issues
	}
	result := issues[:1]
	for _, issue := range issues[1:] {
		if issue != result[len(result)-1] {
			result = append(result, issue)
		}
	}
	return result
}
