package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

func TestYAMLReplacementDogfoodingMatrixMatchesJSONSuccess(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, string) (int, string, string)
	}{
		{name: "question status", run: func(t *testing.T, path string) (int, string, string) {
			return runCLI("forecast-ledger", "--json", "question", "update", "--file", path, "--question", "q-one", "--status", "closed")
		}},
		{name: "question revision", run: func(t *testing.T, path string) (int, string, string) {
			return runCLI("forecast-ledger", "--json", "question", "revise", "--file", path, "--question", "q-one", "--revision-id", "qr-two", "--effective-at", "2026-09-23T12:00:00Z", "--revision-recorded-at", "2026-09-23T12:01:00Z", "--title", "Updated question title", "--resolution-criteria", "Use the official result.", "--expected-resolution-at", "2031-01-01T00:00:00Z", "--outcome-kind", "binary")
		}},
		{name: "question notes addition", run: func(t *testing.T, path string) (int, string, string) {
			return runCLI("forecast-ledger", "--json", "question", "update", "--file", path, "--question", "q-one", "--notes", "Control insertion")
		}},
		{name: "question void", run: func(t *testing.T, path string) (int, string, string) {
			code, stdout, stderr := runCLI("forecast-ledger", "--json", "question", "update", "--file", path, "--question", "q-one", "--status", "closed")
			if code != 0 || stderr != "" {
				t.Fatalf("close before void code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			return runCLI("forecast-ledger", "--json", "question", "void", "--file", path, "--question", "q-one", "--reason", "Question became unresolvable", "--recorded-at", "2026-09-23T12:00:00Z", "--yes")
		}},
		{name: "platform update", run: func(t *testing.T, path string) (int, string, string) {
			return runCLI("forecast-ledger", "--json", "platform", "update", "--file", path, "--platform", "local", "--name", "Updated local platform", "--kind", "internal")
		}},
		{name: "forecast reveal", run: runCLIRevealReplacement},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, extension := range []string{".json", ".yaml"} {
				t.Run(extension, func(t *testing.T) {
					path := newCLIReplacementLedger(t, extension)
					code, stdout, stderr := test.run(t, path)
					if code != 0 || stderr != "" || !strings.Contains(stdout, `"ok":true`) || strings.Contains(stdout+stderr, `"code":"internal"`) {
						t.Fatalf("%s %s code=%d stdout=%q stderr=%q", test.name, extension, code, stdout, stderr)
					}
					code, stdout, stderr = runCLI("forecast-ledger", "--json", "validate", "--file", path)
					if code != 0 || stderr != "" || !strings.Contains(stdout, `"code":"ledger.valid"`) {
						t.Fatalf("validate %s after %s code=%d stdout=%q stderr=%q", extension, test.name, code, stdout, stderr)
					}
				})
			}
		})
	}
}

func newCLIReplacementLedger(t *testing.T, extension string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger"+extension)
	commands := [][]string{
		{"forecast-ledger", "init", "--file", path, "--ledger-id", "replacement-matrix", "--timezone", "UTC", "--forecaster-id", "owner", "--forecaster-name", "Owner"},
		{"forecast-ledger", "platform", "add", "--file", path, "--platform", "local", "--name", "Local", "--kind", "self_hosted"},
		{"forecast-ledger", "question", "add", "--file", path, "--question", "q-one", "--revision-id", "qr-one", "--title", "Will it happen?", "--resolution-criteria", "Use the official result.", "--expected-resolution-at", "2031-01-01T00:00:00Z", "--outcome-kind", "binary"},
		{"forecast-ledger", "forecast", "add", "--file", path, "--question", "q-one", "--forecast", "f-public", "--question-revision", "qr-one", "--forecasted-at", "2026-09-22T12:00:00Z", "--recorded-at", "2026-09-22T12:01:00Z", "--probability", "0.6", "--probability-outcome"},
	}
	for _, command := range commands {
		code, stdout, stderr := runCLI(command...)
		if code != 0 {
			t.Fatalf("setup %s code=%d stdout=%q stderr=%q", strings.Join(command[1:3], " "), code, stdout, stderr)
		}
	}
	return path
}

func runCLIRevealReplacement(t *testing.T, path string) (int, string, string) {
	t.Helper()
	directory := filepath.Dir(path)
	secretPath := filepath.Join(directory, "private.yaml")
	keyPath := filepath.Join(directory, "sealed.key")
	secret := "representations:\n  - kind: probability\n    outcome: true\n    probability: \"0.7\"\nrationale: PRIVATE-CLI-RATIONALE\nkey_factors:\n  - PRIVATE-CLI-FACTOR\ncomment: PRIVATE-CLI-COMMENT\n"
	if err := storage.CreateProtectedFile(secretPath, []byte(secret)); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("forecast-ledger", "--json", "forecast", "seal", "--file", path, "--question", "q-one", "--forecast", "f-sealed", "--question-revision", "qr-one", "--forecasted-at", "2026-09-24T13:00:00Z", "--recorded-at", "2026-09-24T13:01:00Z", "--secret-input", secretPath, "--key-file", keyPath)
	if code != 0 || stderr != "" || strings.Contains(stdout+stderr, "PRIVATE-CLI") {
		t.Fatalf("seal code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI("forecast-ledger", "--json", "forecast", "reveal", "--file", path, "--question", "q-one", "--forecast", "f-sealed", "--key-file", keyPath, "--revealed-at", "2026-09-25T00:00:00Z", "--yes")
	if strings.Contains(stdout+stderr, "PRIVATE-CLI") || strings.Contains(stdout+stderr, keyPath) || strings.Contains(stdout+stderr, secretPath) {
		t.Fatalf("reveal output leaked protected data: stdout=%q stderr=%q", stdout, stderr)
	}
	return code, stdout, stderr
}
