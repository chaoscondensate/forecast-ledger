package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/buildinfo"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	urfavecli "github.com/urfave/cli/v3"
)

func TestProtectedArgumentErrorLabelsSelectedRole(t *testing.T) {
	inputErr := protectedArgumentError(app.NewError(app.CodeConflict, "protected key file must have mode 0600", nil), "--secret-input")
	if !strings.Contains(inputErr.Error(), "--secret-input") || strings.Contains(inputErr.Error(), "key file") {
		t.Fatalf("input error = %q", inputErr)
	}
	keyErr := app.NewError(app.CodeConflict, "protected key file must have mode 0600", nil)
	if !strings.Contains(keyErr.Error(), "key file") || strings.Contains(keyErr.Error(), "--secret-input") {
		t.Fatalf("key error = %q", keyErr)
	}
}

func TestCLIQuestionMutationRetriesAutomaticLedgerRecovery(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, fixtureBytes(t, "individual-ledger.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	interruptValidLedgerWrite(t, path, directory)
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "question", "update", "--file", path, "--question", "q-election-coalition", "--notes", "Recovered question")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"code":"question.updated"`) {
		t.Fatalf("retry code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(storage.JournalPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CLI retry retained journal: %v", err)
	}
}

func interruptValidLedgerWrite(t *testing.T, path, directory string) {
	t.Helper()
	err := storage.UpdateLedger(t.Context(), path, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error {
			return service.ValidateLedgerDocument(parsed, os.DirFS(directory))
		},
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/title", Value: "Interrupted title"}})
		},
		Fault: func(stage storage.TransactionStage) error {
			if stage == storage.StageJournalSynced {
				return errors.New("simulated stop")
			}
			return nil
		},
	})
	if app.ErrorCodeOf(err) != app.CodeIO {
		t.Fatalf("interrupted write error=%v", err)
	}
}

func TestCommandTreeAndLeafLocalFileFlags(t *testing.T) {
	root := NewCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	for _, name := range []string{"init", "ledger", "validate", "status", "platform", "group", "relationship", "question", "forecast", "target", "timestamp", "verify", "publish", "mcp", "version"} {
		if root.Command(name) == nil {
			t.Errorf("root command %q missing", name)
		}
	}
	expectedGroups := map[string][]string{
		"ledger": {"update"}, "platform": {"add", "update", "list", "show", "remove"},
		"group": {"add", "update", "list", "show", "remove"}, "relationship": {"add", "list", "show", "remove"},
		"question": {"add", "revise", "update", "list", "show", "resolve", "ambiguous", "void", "dispute", "not-applicable"},
		"forecast": {"add", "list", "show", "seal", "reveal", "withdraw", "expire", "reaffirm"},
		"target":   {"build", "check"}, "timestamp": {"stamp", "status", "verify"}, "publish": {"build", "verify"}, "mcp": {"serve"},
	}
	for groupName, children := range expectedGroups {
		group := root.Command(groupName)
		for _, childName := range children {
			child := group.Command(childName)
			if child == nil {
				t.Errorf("command %s %s missing", groupName, childName)
				continue
			}
			if groupName != "mcp" {
				assertRequiredFileFlag(t, child)
			}
		}
	}
	for _, leafName := range []string{"init", "validate", "status", "verify"} {
		assertRequiredFileFlag(t, root.Command(leafName))
	}
	if root.Command("question").Command("annul") != nil {
		t.Fatal("removed v1 question annul command is still exposed")
	}
}

func TestEveryVisibleLeafHasActionAdmissionAndExample(t *testing.T) {
	root := NewCommand(strings.NewReader(""), io.Discard, io.Discard)
	if err := root.Walk(func(command *urfavecli.Command) error {
		if command == root || command.Hidden || len(command.Commands) > 0 {
			return nil
		}
		if command.Action == nil {
			t.Errorf("visible leaf %q has no action", command.FullName())
		}
		if !strings.Contains(command.Description, "Example:\n  forecast-ledger ") {
			t.Errorf("visible leaf %q has no concrete example", command.FullName())
		}
		hasFile := false
		for _, flag := range command.Flags {
			hasFile = hasFile || contains(flag.Names(), "file")
		}
		if hasFile && command.Name != "init" && command.Before == nil {
			t.Errorf("existing-ledger leaf %q has no schema admission hook", command.FullName())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestV200AdmissionFailsBeforeTimestampSideEffects(t *testing.T) {
	directory := t.TempDir()
	old := []byte(`{"schema_version":"2.0.0","questions":[{"id":"q","forecasts":[{"id":"f","lifecycle_events":[{"id":"withdrawn","type":"withdrawn"}],"integrity":{"status":"verified","target":{"artifact_path":"proofs/targets/f.json"}}}]}]}`)
	path := filepath.Join(directory, "old.json")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "timestamp", "stamp", "--file", path, "--question", "q", "--forecast", "f", "--tsa-url", "https://tsa.example.test", "--ca-bundle", "tsa.pem")
	if code != 3 || stdout != "" || !strings.Contains(stderr, `"code":"unsupported_schema_version"`) {
		t.Fatalf("admission code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(directory, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("admission created artifacts: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, old) {
		t.Fatalf("admission changed ledger: %v", err)
	}
}

func TestValidateStatusAndVersionUseV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, fixtureBytes(t, "individual-ledger.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "validate", "--file", path)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"code":"ledger.valid"`) {
		t.Fatalf("validate code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI("forecast-ledger", "--json", "status", "--file", path)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"schema_version":"2.0.1"`) {
		t.Fatalf("status code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = runCLIWithStdin(`{"schema_version":"1.3.0","secret":"do-not-print"}`, "forecast-ledger", "--json", "validate", "--file", "-")
	if code != 3 || stdout != "" || !strings.Contains(stderr, `"code":"unsupported_schema_version"`) || strings.Contains(stderr, "do-not-print") {
		t.Fatalf("legacy validate code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, args := range [][]string{{"forecast-ledger", "--json", "version"}, {"forecast-ledger", "version", "--json"}} {
		code, stdout, stderr = runCLI(args...)
		var got buildinfo.Info
		if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &got) != nil || got.Schema.Version != "2.0.1" {
			t.Fatalf("version %v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestCLIValidatesEveryPublishedV2LedgerFixture(t *testing.T) {
	root := filepath.Join("..", "..", "schema", "upstream", "forecast-ledger", "v2.0.1")
	for _, relative := range []string{
		"examples/valid/empty-ledger.json",
		"examples/valid/individual-ledger.json",
		"examples/valid/question-without-forecasts.yaml",
		"examples/valid/team-ledger.yaml",
		"tests/conformance/valid/relationships-and-datetime.json",
	} {
		t.Run(filepath.Base(relative), func(t *testing.T) {
			code, stdout, stderr := runCLI("forecast-ledger", "--json", "validate", "--file", filepath.Join(root, filepath.FromSlash(relative)))
			if code != 0 || stderr != "" || !strings.Contains(stdout, `"code":"ledger.valid"`) {
				t.Fatalf("fixture %s: code=%d stdout=%q stderr=%q", relative, code, stdout, stderr)
			}
		})
	}
}

func TestTargetBuildCheckAndDryRun(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, fixtureBytes(t, "individual-ledger.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	selector := []string{"--file", path, "--question", "q-election-coalition", "--forecast", "f-election-coalition-001"}
	dry := append([]string{"forecast-ledger", "--json", "target", "build"}, selector...)
	dry = append(dry, "--dry-run")
	code, stdout, stderr := runCLI(dry...)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"code":"target.build.planned"`) {
		t.Fatalf("target dry-run code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(directory, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target artifacts: %v", err)
	}
	build := append([]string{"forecast-ledger", "--json", "target", "build"}, selector...)
	code, stdout, stderr = runCLI(build...)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"sha256":"e232db4b3ef9609976f34a6abc70a56faf39e6811f6f5d2a17655afc039aa391"`) || !strings.Contains(stdout, `"size":1152`) {
		t.Fatalf("target build code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	check := append([]string{"forecast-ledger", "--json", "target", "check"}, selector...)
	code, stdout, stderr = runCLI(check...)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"valid":true`) {
		t.Fatalf("target check code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestHelpCompletionAndUnknownCommands(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "pwsh"} {
		code, stdout, stderr := runCLI("forecast-ledger", "completion", shell)
		if code != 0 || stdout == "" || stderr != "" {
			t.Errorf("completion %s code=%d stdout=%d bytes stderr=%q", shell, code, len(stdout), stderr)
		}
	}
	code, stdout, stderr := runCLI("forecast-ledger", "validte")
	if code == 0 || (!strings.Contains(stdout+stderr, "validate") && !strings.Contains(stdout+stderr, "Did you mean")) {
		t.Fatalf("typo guidance missing: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr = runCLI("forecast-ledger", "--plain", "version", "--json")
	if code != 2 || !strings.Contains(stderr, "cannot be combined") {
		t.Fatalf("mixed modes code=%d stderr=%q", code, stderr)
	}
}

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(contractschema.ValidExamples(), name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertRequiredFileFlag(t *testing.T, command *urfavecli.Command) {
	t.Helper()
	for _, flag := range command.Flags {
		if flag.Names()[0] == "file" {
			requiredFlag, ok := flag.(interface{ IsRequired() bool })
			if !ok || !requiredFlag.IsRequired() || !contains(flag.Names(), "f") {
				t.Errorf("%s file flag required=%v names=%v", command.FullName(), ok && requiredFlag.IsRequired(), flag.Names())
			}
			return
		}
	}
	t.Errorf("%s has no --file flag", command.FullName())
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func runCLI(arguments ...string) (int, string, string) { return runCLIWithStdin("", arguments...) }

func runCLIWithStdin(stdin string, arguments ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), arguments, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
