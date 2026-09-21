package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/dlclark/regexp2"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	jsonschemakind "github.com/santhosh-tekuri/jsonschema/v6/kind"
)

var inputValidators sync.Map

type inputECMARegexp regexp2.Regexp

func (regexp *inputECMARegexp) MatchString(value string) bool {
	matched, err := (*regexp2.Regexp)(regexp).MatchString(value)
	return err == nil && matched
}

func (regexp *inputECMARegexp) String() string {
	return (*regexp2.Regexp)(regexp).String()
}

func compileInputECMARegexp(pattern string) (jsonschema.Regexp, error) {
	compiled, err := regexp2.Compile(pattern, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	compiled.MatchTimeout = 100 * time.Millisecond
	return (*inputECMARegexp)(compiled), nil
}

// DecodeOperationInput parses, validates, and decodes one closed operation
// input. Source may be a JSON/YAML path or "-" for the supplied stdin reader.
func DecodeOperationInput(ctx context.Context, source string, stdin io.Reader, schemaName InputSchemaName, destination any) error {
	return decodeOperationInput(ctx, source, stdin, schemaName, destination, document.DefaultLimits)
}

func decodeOperationInput(ctx context.Context, source string, stdin io.Reader, schemaName InputSchemaName, destination any, limits document.Limits) error {
	if err := validateDecodeDestination(destination); err != nil {
		return app.NewError(app.CodeInternal, "operation input destination is not valid", err)
	}
	if ctx != nil && ctx.Err() != nil {
		return app.NewError(app.CodeInterrupted, "operation input was interrupted", ctx.Err())
	}
	if strings.TrimSpace(source) == "" {
		return app.NewError(app.CodeUsage, "request data source is required", nil)
	}

	parsed, err := parseOperationInputSource(source, stdin, limits)
	if err != nil {
		var applicationErr *app.Error
		if errors.As(err, &applicationErr) {
			return applicationErr
		}
		parseFailure := app.NewError(app.CodeInvalidData, "operation input cannot be parsed", err)
		var parseErr *document.ParseError
		if errors.As(err, &parseErr) {
			return app.WithDetails(parseFailure, map[string]any{"issues": []document.Diagnostic{parseErr.Diagnostic}})
		}
		return parseFailure
	}
	if ctx != nil && ctx.Err() != nil {
		return app.NewError(app.CodeInterrupted, "operation input was interrupted", ctx.Err())
	}

	if issues := operationTimestampTagIssues(parsed); len(issues) > 0 {
		return app.WithDetails(
			app.NewError(app.CodeInvalidData, "operation input uses a YAML timestamp outside a timestamp field", nil),
			map[string]any{"schema": schemaName, "issues": issues},
		)
	}

	validator, err := compiledInputValidator(schemaName)
	if err != nil {
		return app.NewError(app.CodeInternal, "operation input schema cannot be loaded", err)
	}
	if err := validator.Validate(parsed.Root.Any()); err != nil {
		return app.WithDetails(
			app.NewError(app.CodeInvalidData, "operation input does not match its closed schema", nil),
			map[string]any{"schema": schemaName, "issues": inputSchemaIssues(parsed, err)},
		)
	}

	encoded, err := json.Marshal(parsed.Root.Any())
	if err != nil {
		return app.NewError(app.CodeInternal, "validated operation input cannot be encoded", err)
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return app.NewError(app.CodeInternal, "validated operation input cannot be decoded", err)
	}
	return nil
}

func parseOperationInputSource(source string, stdin io.Reader, limits document.Limits) (*document.Document, error) {
	if source == "-" {
		if stdin == nil {
			return nil, app.NewError(app.CodeUsage, "request data is not available on stdin", nil)
		}
		return parseUnknownInputFormat(stdin, limits)
	}
	file, err := os.Open(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, app.NewError(app.CodeNotFound, "operation input file does not exist", err)
		}
		return nil, app.NewError(app.CodeIO, "operation input file cannot be opened", err)
	}
	defer file.Close()

	switch strings.ToLower(filepath.Ext(source)) {
	case ".json":
		return document.ParseJSON(file, limits)
	case ".yaml", ".yml":
		return document.ParseYAMLWithTimestampScalars(file, limits)
	default:
		return nil, app.NewError(app.CodeUsage, "operation input filename must end in .json, .yaml, or .yml", nil)
	}
}

func parseUnknownInputFormat(reader io.Reader, limits document.Limits) (*document.Document, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limits.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read operation input: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return document.ParseJSON(bytes.NewReader(raw), limits)
	}
	return document.ParseYAMLWithTimestampScalars(bytes.NewReader(raw), limits)
}

func compiledInputValidator(name InputSchemaName) (*jsonschema.Schema, error) {
	if cached, ok := inputValidators.Load(name); ok {
		return cached.(*jsonschema.Schema), nil
	}
	schemaBytes, err := InputSchema(name)
	if err != nil {
		return nil, err
	}
	documentValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return nil, fmt.Errorf("decode operation input schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseRegexpEngine(compileInputECMARegexp)
	if err := compiler.AddResource("operation-input-schema.json", documentValue); err != nil {
		return nil, fmt.Errorf("register operation input schema: %w", err)
	}
	compiled, err := compiler.Compile("operation-input-schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile operation input schema: %w", err)
	}
	actual, _ := inputValidators.LoadOrStore(name, compiled)
	return actual.(*jsonschema.Schema), nil
}

func validateDecodeDestination(destination any) error {
	if destination == nil {
		return errors.New("destination is nil")
	}
	value := reflect.ValueOf(destination)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errors.New("destination must be a non-nil pointer")
	}
	return nil
}

func safeInputSchemaIssue(err error) string {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return "schema.validation"
	}
	if len(validationErr.InstanceLocation) == 0 {
		return "schema.root"
	}
	return "schema." + strings.Join(validationErr.InstanceLocation, ".")
}

var timestampInputFields = map[string]struct{}{
	"created_at": {}, "opens_at": {}, "expected_resolution_at": {},
	"forecasted_at": {}, "recorded_at": {}, "outcome_known_at": {},
	"retrieved_at": {}, "published_at": {},
}

func operationTimestampTagIssues(parsed *document.Document) []document.Diagnostic {
	if parsed == nil || parsed.Root == nil {
		return nil
	}
	issues := make([]document.Diagnostic, 0)
	var visit func(*document.Value)
	visit = func(value *document.Value) {
		if value == nil {
			return
		}
		if value.SourceTag == "!!timestamp" {
			field := pointerLastToken(value.Source.Pointer)
			if _, allowed := timestampInputFields[field]; !allowed {
				issues = append(issues, document.Diagnostic{
					Code: "input.timestamp_field", Message: "YAML timestamp scalars are allowed only in timestamp fields", Location: value.Source,
				})
			}
		}
		for _, child := range value.Array {
			visit(child)
		}
		for _, member := range value.Object {
			visit(member.Value)
		}
	}
	visit(parsed.Root)
	return issues
}

func inputSchemaIssues(parsed *document.Document, err error) []document.Diagnostic {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return []document.Diagnostic{{Code: "schema.validation", Message: "input does not match its schema", Location: rootInputLocation(parsed)}}
	}
	issues := make([]document.Diagnostic, 0)
	var collect func(*jsonschema.ValidationError)
	collect = func(current *jsonschema.ValidationError) {
		if current == nil {
			return
		}
		if len(current.Causes) > 0 {
			for _, cause := range current.Causes {
				collect(cause)
			}
			return
		}
		pointer := jsonPointer(current.InstanceLocation)
		code := "schema.validation"
		keyword := "validation"
		if current.ErrorKind != nil {
			path := current.ErrorKind.KeywordPath()
			if len(path) > 0 {
				keyword = strings.Join(path, ".")
				code = "schema." + keyword
			}
		}
		if additional, ok := current.ErrorKind.(*jsonschemakind.AdditionalProperties); ok && len(additional.Properties) > 0 {
			properties := append([]string(nil), additional.Properties...)
			sort.Strings(properties)
			for _, property := range properties {
				propertyPointer := appendJSONPointer(pointer, property)
				issues = append(issues, document.Diagnostic{
					Code: code, Message: "unknown field " + property, Location: inputLocation(parsed, propertyPointer),
				})
			}
			return
		}
		issues = append(issues, document.Diagnostic{
			Code: code, Message: "input does not satisfy " + keyword, Location: inputLocation(parsed, pointer),
		})
	}
	collect(validationErr)
	if len(issues) == 0 {
		issues = append(issues, document.Diagnostic{Code: safeInputSchemaIssue(err), Message: "input does not match its schema", Location: rootInputLocation(parsed)})
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Location.Pointer != issues[j].Location.Pointer {
			return issues[i].Location.Pointer < issues[j].Location.Pointer
		}
		return issues[i].Code < issues[j].Code
	})
	return issues
}

func inputLocation(parsed *document.Document, pointer string) document.SourceRef {
	if parsed != nil {
		if locations := parsed.Locations[pointer]; len(locations) > 0 {
			return locations[0]
		}
		for parent := pointer; parent != ""; {
			parent = parentPointer(parent)
			if locations := parsed.Locations[parent]; len(locations) > 0 {
				location := locations[0]
				location.Pointer = pointer
				return location
			}
		}
	}
	return document.SourceRef{Pointer: pointer}
}

func rootInputLocation(parsed *document.Document) document.SourceRef {
	return inputLocation(parsed, "")
}

func jsonPointer(tokens []string) string {
	result := ""
	for _, token := range tokens {
		result = appendJSONPointer(result, token)
	}
	return result
}

func appendJSONPointer(pointer, token string) string {
	token = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	return pointer + "/" + token
}

func pointerLastToken(pointer string) string {
	index := strings.LastIndex(pointer, "/")
	if index < 0 || index == len(pointer)-1 {
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(pointer[index+1:], "~1", "/"), "~0", "~")
}

func parentPointer(pointer string) string {
	if index := strings.LastIndex(pointer, "/"); index >= 0 {
		return pointer[:index]
	}
	return ""
}
