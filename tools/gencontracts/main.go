package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/service"
)

const outputRoot = "../../docs/reference/generated"

func main() {
	must(os.MkdirAll(filepath.Join(outputRoot, "request-schemas"), 0o755))
	for _, name := range service.InputSchemaNames() {
		content, err := service.InputSchema(name)
		must(err)
		content = append(content, '\n')
		mustWrite(filepath.Join(outputRoot, "request-schemas", string(name)+".schema.json"), content)
	}
	mustWrite(filepath.Join(outputRoot, "mcp-tool-schemas.json"), mustJSON(mcpCatalog()))
	mustWrite(filepath.Join(outputRoot, "result.schema.json"), mustJSON(resultSchema()))
	mustWrite(filepath.Join(outputRoot, "index.md"), []byte(generatedIndex()))
	mustWrite(filepath.Join(outputRoot, "request-schemas", "index.md"), []byte(requestSchemaIndex()))
	mustWrite(filepath.Join(outputRoot, "operation-contracts.md"), []byte(markdownReference()))
}

func mcpCatalog() map[string]any {
	tools := make(map[string]any)
	for _, definition := range service.SortedOperationDefinitions() {
		entry := map[string]any{
			"operation":      definition.Name,
			"outcome_states": definition.OutcomeStates,
			"tool_schema":    toolRequestSchema(definition),
			"result_schema":  operationResultSchema(definition),
		}
		if definition.ResultNotes != "" {
			entry["description"] = definition.ResultNotes
		}
		tools[definition.MCPTool] = entry
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://chaoscondensate.com/schemas/forecast-ledger-mcp/tool-catalog/v1",
		"profile": "forecast-ledger-mcp-tools/v1",
		"tools":   tools,
	}
}

func operationResultSchema(definition service.OperationDefinition) map[string]any {
	base := map[string]any{"$ref": "result.schema.json"}
	if definition.Name != service.OperationTimestampVerify {
		return base
	}
	return map[string]any{"allOf": []any{base, map[string]any{
		"type": "object", "properties": map[string]any{"data": map[string]any{"$ref": "result.schema.json#/$defs/timestampVerificationData"}},
	}}}
}

func toolRequestSchema(definition service.OperationDefinition) map[string]any {
	fileDescription := "Ledger reference as root-name:relative/path"
	if definition.Name == service.OperationPublicationVerify {
		fileDescription = "Package ledger reference inside an output root as root-name:relative/path"
	}
	properties := map[string]any{
		"file": map[string]any{"type": "string", "minLength": 1, "description": fileDescription},
	}
	required := []string{"file"}
	addSelector(properties, &required, definition)
	result := map[string]any{}
	if definition.RequestSchema != "" && definition.RequestMode != service.RequestSecret {
		request := directRequestSchema(definition.RequestSchema)
		for name, property := range request["properties"].(map[string]any) {
			if _, collision := properties[name]; collision {
				panic("direct request field collides with selector or control: " + name)
			}
			properties[name] = property
		}
		required = append(required, schemaStrings(request["required"])...)
		result["$defs"] = request["$defs"]
		for _, keyword := range []string{"allOf", "anyOf", "oneOf", "dependentRequired"} {
			if value, ok := request[keyword]; ok {
				result[keyword] = value
			}
		}
	}
	for _, field := range definition.Fields {
		schema := map[string]any{"type": field.Type, "description": field.Description}
		if len(field.Enum) > 0 {
			schema["enum"] = field.Enum
		}
		properties[field.Name] = schema
		if field.Required {
			required = append(required, field.Name)
		}
	}
	if definition.Policy.PersistentEffect {
		properties["dry_run"] = map[string]any{"type": "boolean", "description": "Validate and return a plan without persistent or network effects"}
	}
	if definition.Policy.RequiresConfirmation {
		properties["confirm"] = map[string]any{"const": true, "description": "Explicit caller confirmation"}
		required = append(required, "confirm")
	}
	sort.Strings(required)
	result["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	result["type"] = "object"
	result["additionalProperties"] = false
	result["properties"] = properties
	result["required"] = required
	return result
}

func addSelector(properties map[string]any, required *[]string, definition service.OperationDefinition) {
	identifier := func(description string) map[string]any {
		return map[string]any{"type": "string", "pattern": `^[a-z0-9][a-z0-9._-]{0,127}$`, "description": description}
	}
	switch definition.Selection {
	case service.SelectionPlatform:
		properties["platform"] = identifier("Stable platform ID")
		*required = append(*required, "platform")
	case service.SelectionQuestion:
		properties["question"] = identifier("Stable question ID")
		*required = append(*required, "question")
	case service.SelectionForecast:
		properties["question"] = identifier("Stable question ID")
		properties["forecast"] = identifier("Stable forecast ID")
		*required = append(*required, "question", "forecast")
	case service.SelectionTarget:
		properties["question"] = identifier("Stable question ID")
		properties["forecast"] = identifier("Stable forecast ID")
		properties["all"] = map[string]any{"type": "boolean"}
	case service.SelectionGroup:
		properties["group"] = identifier("Stable group ID")
		*required = append(*required, "group")
	case service.SelectionRelationship:
		if definition.Name != service.OperationRelationshipAdd {
			properties["relationship_id"] = identifier("Stable relationship ID")
			*required = append(*required, "relationship_id")
		}
	}
}

func directRequestSchema(name service.InputSchemaName) map[string]any {
	document, err := service.DirectRequestSchema(name)
	must(err)
	return document
}

func schemaStrings(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func resultSchema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://chaoscondensate.com/schemas/forecast-ledger-cli/result/v1",
		"title":   "Forecast Ledger operation result",
		"type":    "object", "additionalProperties": false,
		"required": []string{"operation", "code", "message", "data", "recovery"},
		"properties": map[string]any{
			"operation": map[string]any{"type": "string"}, "code": map[string]any{"type": "string"},
			"message": map[string]any{"type": "string"}, "data": map[string]any{"type": "object", "description": "Operation-specific public data; timestamp.verify uses $defs.timestampVerificationData."},
			"warnings": map[string]any{"type": "array", "items": closedRecord([]string{"code", "message"}, map[string]any{
				"code": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "details": map[string]any{"type": "object"},
			})},
			"effects": map[string]any{"type": "array", "items": closedRecord([]string{"kind", "action", "status"}, map[string]any{
				"kind":   map[string]any{"enum": []string{"ledger", "target", "evidence_index", "timestamp_request", "timestamp_response", "timestamp_trust", "key", "package", "network"}},
				"action": map[string]any{"enum": []string{"read", "create", "replace", "remove", "contact"}},
				"status": map[string]any{"enum": []string{"planned", "deferred", "completed", "unchanged"}},
				"root":   map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}, "source_id": map[string]any{"type": "string"},
				"owned": map[string]any{"type": "boolean"}, "rollback": map[string]any{"enum": []string{"none", "created_public", "retain_secret"}},
			})},
			"recovery": closedRecord([]string{"state"}, map[string]any{
				"state":   map[string]any{"enum": []string{"none", "complete", "pending", "retained", "required"}},
				"message": map[string]any{"type": "string"}, "paths": stringArray(), "actions": stringArray(),
			}),
		},
		"$defs": resultDefinitions(),
	}
}

func resultDefinitions() map[string]any {
	stringList := stringArray()
	requestSummary := closedRecord([]string{"request_count"}, map[string]any{
		"request_count": map[string]any{"type": "integer", "minimum": 0, "maximum": 16},
		"tsa_origin":    map[string]any{"type": "string"},
	})
	verificationLayer := closedRecord([]string{"name", "state"}, map[string]any{
		"name": map[string]any{"type": "string"}, "state": map[string]any{"enum": []string{"pass", "fail", "pending", "not_applicable", "not_checked"}},
		"reason_codes": stringList, "evidence": map[string]any{"type": "object"}, "limitations": stringList,
	})
	timestampEntry := closedRecord([]string{"tsa_url", "state", "request_path", "response_path", "request_present", "response_present", "ca_bundle_present", "check_state"}, map[string]any{
		"provider_id": map[string]any{"type": "string"},
		"tsa_url":     map[string]any{"type": "string", "format": "uri"}, "state": map[string]any{"enum": []string{"pending", "verified"}},
		"request_path": map[string]any{"type": "string"}, "response_path": map[string]any{"type": "string"}, "ca_bundle_path": map[string]any{"type": "string"},
		"request_present": map[string]any{"type": "boolean"}, "response_present": map[string]any{"type": "boolean"}, "ca_bundle_present": map[string]any{"type": "boolean"},
		"check_state": map[string]any{"enum": []string{"pass", "fail", "pending", "not_applicable", "not_checked"}}, "reason_codes": stringList,
		"gen_time": map[string]any{"type": "string", "format": "date-time"}, "policy_oid": map[string]any{"type": "string"}, "serial_number": map[string]any{"type": "string"},
		"signer_subject": map[string]any{"type": "string"}, "signer_fingerprint_sha256": map[string]any{"type": "string"}, "ca_bundle_sha256": map[string]any{"type": "string"},
	})
	timestampAttempt := closedRecord([]string{"provider_id", "ordinal", "attempted"}, map[string]any{
		"provider_id": map[string]any{"type": "string"}, "ordinal": map[string]any{"type": "integer", "minimum": 1, "maximum": 16},
		"attempted": map[string]any{"type": "boolean"}, "reason_code": map[string]any{"type": "string"},
	})
	timestampData := closedRecord([]string{"question_id", "forecast_id", "scope", "state", "target_path", "target_sha256", "target_present", "verification"}, map[string]any{
		"question_id": map[string]any{"type": "string"}, "forecast_id": map[string]any{"type": "string"},
		"scope": map[string]any{"enum": []string{"forecast", "lifecycle"}}, "head_event_id": map[string]any{"type": "string"},
		"selection_mode": map[string]any{"enum": []string{"auto", "named", "custom"}}, "selected_provider": map[string]any{"type": "string"},
		"attempts":    map[string]any{"type": "array", "maxItems": 16, "items": timestampAttempt},
		"state":       map[string]any{"enum": []string{"unanchored", "pending", "verified", "failed", "inconsistent"}},
		"target_path": map[string]any{"type": "string"}, "target_sha256": map[string]any{"type": "string"},
		"target_present": map[string]any{"type": "boolean"}, "timestamps": map[string]any{"type": "array", "items": timestampEntry},
		"request_summary": requestSummary, "next_actions": stringList,
		"warnings": map[string]any{"type": "array"}, "effects": map[string]any{"type": "array"}, "recovery": map[string]any{"type": "object"},
		"verification": verificationLayer,
	})
	return map[string]any{
		"verificationOverall": map[string]any{"enum": []string{"pass", "fail", "pending", "incomplete", "no_evidence"}},
		"verificationLayer":   verificationLayer, "timestampEntry": timestampEntry,
		"requestSummary": requestSummary, "timestampVerificationData": timestampData,
	}
}

func closedRecord(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

func stringArray() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func markdownReference() string {
	var builder strings.Builder
	builder.WriteString("# Generated operation contracts\n\n")
	builder.WriteString("<!-- doc-metadata\ncoverage: operation-contracts-v1\nreviewed: 2026-08-31\nowner: interface\ngenerated: true\nsecurity-critical: true\nprerequisites: index.md\nnext: ../index.md\nsource: go generate ./internal/service\n-->\n\n")
	builder.WriteString("> Generated; do not edit by hand. Run `go generate ./internal/service`.\n\n")
	builder.WriteString("These declarations are shared request contracts for CLI reference and MCP discovery. A declaration does not make a hidden command available.\n\n")
	builder.WriteString("| Operation | CLI | MCP tool | Selection | Request contract | Outcomes | Dry-run | Confirmation | Network | Result notes |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, definition := range service.SortedOperationDefinitions() {
		request := "—"
		if definition.RequestSchema != "" {
			request = fmt.Sprintf("[%s](request-schemas/%s.schema.json) (%s)", definition.RequestSchema, definition.RequestSchema, definition.RequestMode)
		}
		outcomes := make([]string, len(definition.OutcomeStates))
		for index, state := range definition.OutcomeStates {
			outcomes[index] = "`" + string(state) + "`"
		}
		builder.WriteString(fmt.Sprintf("| `%s` | `forecast-ledger %s` | `%s` | `%s` | %s | %s | %t | %t | `%s` | %s |\n",
			definition.Name, definition.CLI, definition.MCPTool, definition.Selection, request,
			strings.Join(outcomes, ", "), definition.Policy.PersistentEffect, definition.Policy.RequiresConfirmation, definition.Policy.Network, definition.ResultNotes))
	}
	builder.WriteString("\nThe common [operation result schema](result.schema.json) defines warning, side-effect, and recovery fields. The [MCP tool catalog](mcp-tool-schemas.json) contains closed request schemas.\n\n")
	builder.WriteString("[Reference index](../index.md)\n")
	return builder.String()
}

func generatedIndex() string {
	return `# Generated interface reference

<!-- doc-metadata
coverage: operation-contracts-v1
reviewed: 2026-08-26
owner: interface
generated: true
security-critical: true
prerequisites: ../index.md
next: operation-contracts.md
source: go generate ./internal/service
-->

> Generated; do not edit by hand. Run ` + "`go generate ./internal/service`" + `.

- [Operation contracts](operation-contracts.md)
- [Direct request schemas](request-schemas/index.md)
- [Common result schema](result.schema.json)
- [MCP tool schema catalog](mcp-tool-schemas.json)

[Reference index](../index.md)
`
}

func requestSchemaIndex() string {
	var builder strings.Builder
	builder.WriteString("# Generated request schemas\n\n")
	builder.WriteString("<!-- doc-metadata\ncoverage: operation-contracts-v1\nreviewed: 2026-08-26\nowner: interface\ngenerated: true\nsecurity-critical: true\nprerequisites: ../index.md\nnext: ../operation-contracts.md\nsource: go generate ./internal/service\n-->\n\n")
	builder.WriteString("> Generated; do not edit by hand. Run `go generate ./internal/service`.\n\n")
	builder.WriteString("These closed Draft 2020-12 schemas define direct public request fields and purpose-named protected bundles.\n\n")
	for _, name := range service.InputSchemaNames() {
		builder.WriteString(fmt.Sprintf("- [`%s`](%s.schema.json)\n", name, name))
	}
	builder.WriteString("\n[Generated interface reference](../index.md)\n")
	return builder.String()
}

func mustJSON(value any) []byte {
	content, err := json.MarshalIndent(value, "", "  ")
	must(err)
	return append(content, '\n')
}

func mustWrite(path string, content []byte) {
	must(os.WriteFile(path, content, 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
