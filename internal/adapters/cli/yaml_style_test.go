package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestApplicationWrittenYAMLUsesBlockStyleForPopulatedCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	commands := [][]string{
		{"forecast-ledger", "init", "--file", path, "--ledger-id", "yaml-style", "--timezone", "Europe/London", "--forecaster-id", "owner", "--forecaster-name", "Owner", "--initial-platform", "internal,Internal,internal,https://example.com"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-style", "--revision-id", "qr-style", "--title", "Will it happen?", "--resolution-criteria", "Use the named public result.", "--expected-resolution-at", "10 Aug 2030", "--outcome-kind", "binary", "--tag", "review", "--initial-forecast", "f-style-001", "--initial-probability", "0.51", "--initial-probability-outcome", "--initial-key-factor", "First factor", "--initial-key-factor", "Second factor"},
		{"forecast-ledger", "forecast", "add", "--file", path, "--question", "q-style", "--forecast", "f-style-002", "--question-revision", "qr-style", "--probability", "0.52", "--probability-outcome", "--key-factor", "Third factor", "--supersedes-forecast", "f-style-001"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-empty", "--revision-id", "qr-empty", "--title", "Backlog question", "--resolution-criteria", "Use the named public result.", "--expected-resolution-at", "11 Aug 2030", "--outcome-kind", "binary"},
		{"forecast-ledger", "platform", "update", "--file", path, "--platform", "internal", "--name", "Updated internal", "--kind", "internal", "--account-username", "reviewer"},
		{"forecast-ledger", "question", "update", "--file", path, "--question", "q-style", "--status", "closed", "--tag", "review", "--tag", "regression"},
		{"forecast-ledger", "question", "void", "--file", path, "--question", "q-style", "--reason", "Question became unresolvable", "--recorded-at", "2030-08-11T00:00:00+01:00", "--yes"},
	}
	for _, arguments := range commands {
		code, _, stderr := runCLI(arguments...)
		if code != 0 {
			t.Fatalf("%s failed with %d: %s", strings.Join(arguments[1:3], " "), code, stderr)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	assertNoPopulatedFlowCollections(t, &document, "$")
	if !strings.Contains(string(raw), "forecasts:\n") || !strings.Contains(string(raw), "forecasts: []") {
		t.Fatalf("expected expanded populated collections:\n%s", raw)
	}
}

func assertNoPopulatedFlowCollections(t *testing.T, node *yaml.Node, path string) {
	t.Helper()
	if (node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode) && len(node.Content) > 0 && node.Style&yaml.FlowStyle != 0 {
		t.Errorf("populated collection %s uses YAML flow style", path)
	}
	for index, child := range node.Content {
		assertNoPopulatedFlowCollections(t, child, path+"/"+child.Value)
		_ = index
	}
}
