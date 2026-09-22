package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

type InputSchemaName string

const (
	InputSchemaInit                 InputSchemaName = "init"
	InputSchemaRootMetadata         InputSchemaName = "root-metadata-patch"
	InputSchemaPlatformCreate       InputSchemaName = "platform-create"
	InputSchemaPlatformPatch        InputSchemaName = "platform-patch"
	InputSchemaQuestionAdd          InputSchemaName = "question-add"
	InputSchemaQuestionPatch        InputSchemaName = "question-patch"
	InputSchemaQuestionRevision     InputSchemaName = "question-revision"
	InputSchemaForecastCreate       InputSchemaName = "forecast-create"
	InputSchemaForecastSeal         InputSchemaName = "forecast-seal"
	InputSchemaForecastSealPrivate  InputSchemaName = "forecast-seal-private"
	InputSchemaKeyHintUpdate        InputSchemaName = "key-hint-update"
	InputSchemaResolution           InputSchemaName = "resolution"
	InputSchemaUnresolvedResolution InputSchemaName = "unresolved-resolution"
	InputSchemaNotApplicable        InputSchemaName = "not-applicable"
	InputSchemaGroupCreate          InputSchemaName = "group-create"
	InputSchemaGroupPatch           InputSchemaName = "group-patch"
	InputSchemaRelationship         InputSchemaName = "relationship"
	InputSchemaLifecycle            InputSchemaName = "lifecycle"
	InputSchemaPublicationBuild     InputSchemaName = "publication-build"
	InputSchemaPublicationVerify    InputSchemaName = "publication-verify"
)

const inputSchemaVersion = "2"

var inputSchemaNames = []InputSchemaName{
	InputSchemaInit, InputSchemaRootMetadata, InputSchemaPlatformCreate, InputSchemaPlatformPatch,
	InputSchemaQuestionAdd, InputSchemaQuestionPatch, InputSchemaQuestionRevision,
	InputSchemaForecastCreate, InputSchemaForecastSeal, InputSchemaForecastSealPrivate, InputSchemaKeyHintUpdate,
	InputSchemaResolution, InputSchemaUnresolvedResolution, InputSchemaNotApplicable,
	InputSchemaGroupCreate, InputSchemaGroupPatch, InputSchemaRelationship, InputSchemaLifecycle,
	InputSchemaPublicationBuild, InputSchemaPublicationVerify,
}

func InputSchemaNames() []InputSchemaName {
	result := append([]InputSchemaName(nil), inputSchemaNames...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func InputSchema(name InputSchemaName) ([]byte, error) {
	definitions, err := inputDefinitions()
	if err != nil {
		return nil, err
	}
	definition, ok := operationDefinitions()[name]
	if !ok {
		return nil, fmt.Errorf("unknown input schema %q", name)
	}
	rootDefinition := string(name)
	if _, collision := definitions[rootDefinition]; collision {
		rootDefinition += "Input"
	}
	definitions[rootDefinition] = definition
	definitions = reachableDefinitions(definitions, rootDefinition)
	document := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": fmt.Sprintf("https://chaoscondensate.com/schemas/forecast-ledger-cli/input/%s/v%s", name, inputSchemaVersion), "$ref": "#/$defs/" + rootDefinition, "$defs": definitions}
	return json.MarshalIndent(document, "", "  ")
}

func DirectRequestSchema(name InputSchemaName) (map[string]any, error) {
	content, err := InputSchema(name)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	definitions, _ := document["$defs"].(map[string]any)
	reference, _ := document["$ref"].(string)
	current := any(map[string]any{"$ref": reference})
	seen := map[string]bool{}
	for {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("direct request schema %q is not an object", name)
		}
		reference, ok = object["$ref"].(string)
		if !ok || len(object) != 1 {
			result := cloneSchemaObject(object)
			result["$defs"] = definitions
			return result, nil
		}
		const prefix = "#/$defs/"
		if !strings.HasPrefix(reference, prefix) || seen[reference] {
			return nil, fmt.Errorf("direct request schema %q has an invalid root reference", name)
		}
		seen[reference] = true
		current = definitions[strings.TrimPrefix(reference, prefix)]
	}
}

func cloneSchemaObject(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}
func reachableDefinitions(all map[string]any, root string) map[string]any {
	result := map[string]any{}
	queue := []string{root}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, ok := result[name]; ok {
			continue
		}
		definition, ok := all[name]
		if !ok {
			continue
		}
		result[name] = definition
		collectDefinitionReferences(definition, func(reference string) {
			if _, ok := result[reference]; !ok {
				queue = append(queue, reference)
			}
		})
	}
	return result
}
func collectDefinitionReferences(value any, add func(string)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				if reference, ok := child.(string); ok && strings.HasPrefix(reference, "#/$defs/") {
					add(strings.TrimPrefix(reference, "#/$defs/"))
				}
				continue
			}
			collectDefinitionReferences(child, add)
		}
	case []any:
		for _, child := range typed {
			collectDefinitionReferences(child, add)
		}
	}
}

func inputDefinitions() (map[string]any, error) {
	var contract struct {
		Definitions map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(ledgerschema.Contract(), &contract); err != nil {
		return nil, fmt.Errorf("decode embedded ledger definitions: %w", err)
	}
	result := make(map[string]any, len(contract.Definitions)+16)
	for name, definition := range contract.Definitions {
		result[name] = definition
	}
	for name, definition := range commonInputDefinitions() {
		result[name] = definition
	}
	return result, nil
}

func operationDefinitions() map[InputSchemaName]any {
	return map[InputSchemaName]any{
		InputSchemaInit:           closedObject(nil, map[string]any{"title": stringValue(0), "description": stringValue(0), "created_at": ref("timestamp"), "contact": ref("contact"), "profiles": arrayOf(ref("profile"), 0), "members": arrayOf(ref("member"), 2), "platforms": map[string]any{"type": "object", "propertyNames": ref("slug"), "additionalProperties": ref("platform")}, "question": ref("initialQuestionInput")}, nil),
		InputSchemaRootMetadata:   closedObject(nil, map[string]any{"title": nullable(stringValue(0)), "description": nullable(stringValue(0)), "default_timezone": stringValue(1), "forecaster": ref("forecasterPatchInput")}, map[string]any{"minProperties": 1}),
		InputSchemaPlatformCreate: ref("platform"),
		InputSchemaPlatformPatch:  closedObject(nil, map[string]any{"name": nullable(stringValue(1)), "kind": nullable(map[string]any{"enum": []string{"scoring_platform", "prediction_market", "self_hosted", "internal", "informal"}}), "url": nullable(map[string]any{"type": "string", "format": "uri"}), "account": nullable(ref("platformAccountPatchInput"))}, map[string]any{"minProperties": 1}),
		InputSchemaQuestionAdd:    ref("questionAddInput"), InputSchemaQuestionRevision: ref("revisionInput"),
		InputSchemaQuestionPatch:  closedObject(nil, map[string]any{"tags": nullable(arrayOf(ref("slug"), 0)), "notes": nullable(stringValue(0)), "status": map[string]any{"enum": []string{"open", "closed", "awaiting_resolution"}}}, map[string]any{"minProperties": 1}),
		InputSchemaForecastCreate: ref("publicForecastInput"), InputSchemaForecastSeal: ref("sealedForecastInput"),
		InputSchemaForecastSealPrivate:  closedObject([]string{"representations"}, map[string]any{"representations": arrayOf(ref("forecastRepresentation"), 1), "rationale": stringValue(0), "key_factors": arrayOf(stringValue(1), 0), "comment": stringValue(0)}, nil),
		InputSchemaKeyHintUpdate:        closedObject([]string{"key_hint"}, map[string]any{"key_hint": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9+.-]*:[A-Za-z0-9._~+-]+$`}}, nil),
		InputSchemaResolution:           closedObject([]string{"question_revision_id", "outcome", "outcome_known_at", "sources"}, map[string]any{"question_revision_id": ref("slug"), "outcome": ref("scalarValue"), "outcome_known_at": ref("timestamp"), "recorded_at": ref("timestamp"), "sources": arrayOf(ref("resolutionSource"), 1), "notes": stringValue(0)}, nil),
		InputSchemaUnresolvedResolution: reasonInputDefinition(),
		InputSchemaNotApplicable:        closedObject([]string{"relationship_id", "reason"}, map[string]any{"relationship_id": ref("slug"), "reason": stringValue(1), "recorded_at": ref("timestamp")}, nil),
		InputSchemaGroupCreate:          closedObject([]string{"title"}, map[string]any{"title": stringValue(1), "description": stringValue(0)}, nil),
		InputSchemaGroupPatch:           closedObject(nil, map[string]any{"title": stringValue(1), "description": nullable(stringValue(0))}, map[string]any{"minProperties": 1}),
		InputSchemaRelationship:         closedObject([]string{"relationship"}, map[string]any{"relationship": ref("relationship")}, nil),
		InputSchemaLifecycle:            closedObject([]string{"id", "effective_at"}, map[string]any{"id": ref("slug"), "effective_at": ref("timestamp"), "recorded_at": ref("timestamp"), "reason": stringValue(1), "provenance": ref("provenance")}, nil),
		InputSchemaPublicationBuild:     closedObject([]string{"file", "output"}, map[string]any{"file": stringValue(1), "output": stringValue(1), "dry_run": map[string]any{"type": "boolean"}}, nil),
		InputSchemaPublicationVerify:    closedObject([]string{"file", "manifest"}, map[string]any{"file": stringValue(1), "manifest": stringValue(1)}, nil),
	}
}

func commonInputDefinitions() map[string]any {
	revisionProperties := map[string]any{"id": ref("slug"), "effective_at": ref("timestamp"), "recorded_at": ref("timestamp"), "title": stringValue(1), "resolution_criteria": stringValue(1), "forecasting_opens_at": ref("timestamp"), "expected_resolution_at": ref("timestamp"), "outcome_space": ref("outcomeSpace"), "domain": ref("domain"), "provenance": ref("provenance")}
	forecastProperties := map[string]any{"question_revision_id": ref("slug"), "forecasted_at": ref("timestamp"), "recorded_at": ref("timestamp"), "representations": arrayOf(ref("forecastRepresentation"), 1), "rationale": stringValue(0), "key_factors": arrayOf(stringValue(1), 0), "comment": stringValue(0), "public_note": stringValue(0), "supersedes_forecast_id": ref("slug"), "provenance": ref("provenance")}
	initialForecastProperties := cloneProperties(forecastProperties)
	delete(initialForecastProperties, "question_revision_id")
	initialForecastProperties["id"] = ref("slug")
	initialForecastProperties["visibility"] = map[string]any{"enum": []string{"public", "sealed"}}
	questionProperties := map[string]any{"created_at": ref("timestamp"), "revision": ref("revisionInput"), "tags": arrayOf(ref("slug"), 0), "notes": stringValue(0), "initial_forecast": ref("initialForecastInput")}
	initialQuestionProperties := cloneProperties(questionProperties)
	initialQuestionProperties["id"] = ref("slug")
	return map[string]any{
		"revisionInput":             closedObject([]string{"id", "title", "resolution_criteria", "expected_resolution_at", "outcome_space", "domain"}, revisionProperties, nil),
		"initialForecastInput":      closedObject([]string{"id", "visibility"}, initialForecastProperties, map[string]any{"allOf": []any{map[string]any{"if": map[string]any{"properties": map[string]any{"visibility": map[string]any{"const": "public"}}}, "then": map[string]any{"required": []string{"representations"}}}, map[string]any{"if": map[string]any{"properties": map[string]any{"visibility": map[string]any{"const": "sealed"}}}, "then": map[string]any{"not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"representations"}}, map[string]any{"required": []string{"rationale"}}, map[string]any{"required": []string{"key_factors"}}, map[string]any{"required": []string{"comment"}}}}}}}}),
		"initialQuestionInput":      closedObject([]string{"id", "revision"}, initialQuestionProperties, nil),
		"questionAddInput":          closedObject([]string{"revision"}, questionProperties, nil),
		"publicForecastInput":       closedObject([]string{"question_revision_id", "representations"}, forecastProperties, nil),
		"sealedForecastInput":       closedObject([]string{"question_revision_id", "representations"}, forecastProperties, nil),
		"forecasterPatchInput":      closedObject(nil, map[string]any{"kind": map[string]any{"enum": []string{"individual", "team"}}, "name": stringValue(1), "contact": nullable(ref("contact")), "profiles": nullable(arrayOf(ref("profile"), 0)), "members": nullable(arrayOf(ref("member"), 2))}, map[string]any{"minProperties": 1}),
		"platformAccountPatchInput": closedObject(nil, map[string]any{"username": nullable(stringValue(1)), "user_id": nullable(stringValue(1)), "profile_url": nullable(map[string]any{"type": "string", "format": "uri"})}, map[string]any{"minProperties": 1}),
	}
}

func reasonInputDefinition() map[string]any {
	return closedObject([]string{"reason"}, map[string]any{"reason": stringValue(1), "recorded_at": ref("timestamp"), "sources": arrayOf(ref("resolutionSource"), 0)}, nil)
}
func closedObject(required []string, properties map[string]any, extra map[string]any) map[string]any {
	result := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		result["required"] = required
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}
func ref(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
func nullable(value any) map[string]any {
	return map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
}
func stringValue(minimum int) map[string]any {
	return map[string]any{"type": "string", "minLength": minimum}
}
func arrayOf(items any, minimum int) map[string]any {
	result := map[string]any{"type": "array", "items": items}
	if minimum > 0 {
		result["minItems"] = minimum
	}
	return result
}
func cloneProperties(properties map[string]any) map[string]any {
	result := make(map[string]any, len(properties))
	for key, value := range properties {
		result[key] = value
	}
	return result
}
