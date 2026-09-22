package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

func TestRemovedGenericInputFlagIsRejectedWithoutReadingOrMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	before := fixtureBytes(t, "individual-ledger.json")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	const canary = "PRIVATE-DIAGNOSTIC-CANARY"
	code, stdout, stderr := runCLIWithStdin(canary, "forecast-ledger", "platform", "add", "--file", path, "--platform", "example", "--input", "-")
	if code != 2 || stdout != "" || strings.Contains(stderr, canary) || !strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("removed flag result code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("removed generic flag changed the ledger")
	}
}

func TestSemanticDirectFlagDiagnosticUsesSafePointer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, fixtureBytes(t, "individual-ledger.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("forecast-ledger", "platform", "add", "--file", path, "--platform", "example", "--name", "Example", "--kind", "informal", "--url", "not a url")
	if code != 3 || stdout != "" || !strings.Contains(stderr, "/platform/url") || !strings.Contains(stderr, "semantic.invalid_field") {
		t.Fatalf("semantic diagnostic code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "line 0") || strings.Contains(stderr, "column 0") {
		t.Fatalf("semantic diagnostic fabricated a position: %s", stderr)
	}
}

func TestProtectedInputDiagnosticsPrecedeAllSealedWriteRoutes(t *testing.T) {
	directory := t.TempDir()
	secretPath := filepath.Join(directory, "invalid-private.yaml")
	const canary = "PRIVATE-DIAGNOSTIC-CANARY"
	if err := storage.CreateProtectedFile(secretPath, []byte("rationale: "+canary+"\n")); err != nil {
		t.Fatal(err)
	}
	assertFailure := func(name string, keyPath string, beforePath string, before []byte, args ...string) {
		t.Helper()
		code, stdout, stderr := runCLI(append([]string{"forecast-ledger", "--json"}, args...)...)
		if code != 3 || stdout != "" || !strings.Contains(stderr, `"pointer":"/representations"`) || strings.Contains(stderr, canary) || strings.Contains(stderr, `"line":1`) {
			t.Fatalf("%s diagnostic code=%d stdout=%q stderr=%q", name, code, stdout, stderr)
		}
		if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
			t.Fatalf("%s created key: %v", name, err)
		}
		if beforePath != "" {
			after, err := os.ReadFile(beforePath)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("%s changed ledger: %v", name, err)
			}
		}
	}

	standaloneLedger := filepath.Join(directory, "standalone.json")
	code, _, stderr := runCLI("forecast-ledger", "init", "--file", standaloneLedger, "--ledger-id", "standalone", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner")
	if code != 0 {
		t.Fatalf("standalone init failed: %s", stderr)
	}
	code, _, stderr = runCLI("forecast-ledger", "question", "add", "--file", standaloneLedger, "--question", "q-one", "--revision-id", "qr-one", "--title", "Will it happen?", "--resolution-criteria", "Use the public result.", "--expected-resolution-at", "2027-01-01T00:00:00Z", "--outcome-kind", "binary")
	if code != 0 {
		t.Fatalf("standalone question failed: %s", stderr)
	}
	standaloneBefore, err := os.ReadFile(standaloneLedger)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure("forecast seal", filepath.Join(directory, "standalone.key"), standaloneLedger, standaloneBefore,
		"forecast", "seal", "--file", standaloneLedger, "--question", "q-one", "--forecast", "f-one", "--question-revision", "qr-one", "--forecasted-at", "2026-09-22T00:00:00Z", "--secret-input", secretPath, "--key-file", filepath.Join(directory, "standalone.key"))

	initLedger := filepath.Join(directory, "initial.json")
	assertFailure("ledger init", filepath.Join(directory, "initial.key"), "", nil,
		"init", "--file", initLedger, "--ledger-id", "initial", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner",
		"--question", "q-initial", "--question-revision-id", "qr-initial", "--question-title", "Will it happen?", "--question-resolution-criteria", "Use the public result.", "--question-expected-resolution-at", "2027-01-01T00:00:00Z", "--question-outcome-kind", "binary",
		"--initial-forecast", "f-initial", "--initial-visibility", "sealed", "--initial-forecasted-at", "2026-09-22T00:00:00Z", "--initial-secret-input", secretPath, "--key-file", filepath.Join(directory, "initial.key"))
	if _, err := os.Stat(initLedger); !os.IsNotExist(err) {
		t.Fatalf("failed ledger init created ledger: %v", err)
	}

	questionLedger := filepath.Join(directory, "question.json")
	code, _, stderr = runCLI("forecast-ledger", "init", "--file", questionLedger, "--ledger-id", "question", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner")
	if code != 0 {
		t.Fatalf("question-route init failed: %s", stderr)
	}
	questionBefore, err := os.ReadFile(questionLedger)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure("question add", filepath.Join(directory, "question.key"), questionLedger, questionBefore,
		"question", "add", "--file", questionLedger, "--question", "q-secret", "--revision-id", "qr-secret", "--title", "Will it happen?", "--resolution-criteria", "Use the public result.", "--expected-resolution-at", "2027-01-01T00:00:00Z", "--outcome-kind", "binary",
		"--initial-forecast", "f-secret", "--initial-visibility", "sealed", "--initial-forecasted-at", "2026-09-22T00:00:00Z", "--initial-secret-input", secretPath, "--key-file", filepath.Join(directory, "question.key"))
}
