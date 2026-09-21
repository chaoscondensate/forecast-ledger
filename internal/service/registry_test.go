package service

import "testing"

func TestOperationRegistryIsCompleteAndUnique(t *testing.T) {
	definitions := OperationDefinitions()
	if len(definitions) != 45 {
		t.Fatalf("operation definitions = %d, want 45", len(definitions))
	}
	names := make(map[OperationName]struct{}, len(definitions))
	tools := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if _, duplicate := names[definition.Name]; duplicate {
			t.Fatalf("duplicate operation %q", definition.Name)
		}
		names[definition.Name] = struct{}{}
		if definition.MCPTool == "" {
			t.Fatalf("operation %q has no MCP tool", definition.Name)
		}
		if _, duplicate := tools[definition.MCPTool]; duplicate {
			t.Fatalf("duplicate MCP tool %q", definition.MCPTool)
		}
		tools[definition.MCPTool] = struct{}{}
		if definition.RequestSchema != "" {
			if _, err := InputSchema(definition.RequestSchema); err != nil {
				t.Fatalf("operation %q input: %v", definition.Name, err)
			}
		}
	}
}

func TestOperationRegistryDeclaresClosedOutcomeContracts(t *testing.T) {
	for _, definition := range OperationDefinitions() {
		if len(definition.OutcomeStates) == 0 {
			t.Fatalf("operation %s has no outcome states", definition.Name)
		}
		seen := make(map[OutcomeState]bool, len(definition.OutcomeStates))
		for _, state := range definition.OutcomeStates {
			if state == "" || seen[state] {
				t.Fatalf("operation %s has invalid outcome states %v", definition.Name, definition.OutcomeStates)
			}
			seen[state] = true
		}
		outcome := definition.ClassifyOutcome(OutcomeInput{Data: representativeOutcomeData(definition.Name)})
		if outcome.Code == "" || outcome.Message == "" || !outcome.HasData {
			t.Fatalf("operation %s has incomplete success outcome: %+v", definition.Name, outcome)
		}
	}
}

func representativeOutcomeData(operation OperationName) any {
	switch operation {
	case OperationLedgerUpdate:
		return RootMetadataFileResult{Changed: true}
	case OperationPlatformUpdate:
		return PlatformFileResult{Changed: true}
	case OperationQuestionUpdate:
		return QuestionFileResult{Changed: true}
	case OperationForecastReveal, OperationForecastKeyHintUpdate:
		return ForecastFileResult{Changed: true}
	case OperationTargetCheck:
		return TargetOperationResult{}
	case OperationTimestampStamp:
		return TimestampArtifactResult{State: TimestampVerified}
	case OperationTimestampVerify:
		return TimestampVerifyResult{Verification: VerificationLayer{State: LayerPass}}
	case OperationVerificationRun:
		return VerificationReport{Overall: VerificationPass}
	case OperationPublicationVerify:
		return PublicationVerifyResult{Overall: VerificationPass}
	default:
		return struct{}{}
	}
}
