package releasecheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestForecastLedgerV2ReleasePinsMoveTogether(t *testing.T) {
	t.Parallel()

	pins := []string{
		"2.0.0",
		"1d3b186a15136bc5aff38647cb59fbef475dbe55",
		"7b4a9e85e0df9350750828a57b03ff729f704ee4",
		"1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e",
		"77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc",
		"efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21",
		"forecast-seal/v2",
		"forecast-envelope/v2",
	}
	actual := []string{
		ledgerschema.Version,
		ledgerschema.Commit,
		ledgerschema.AnnotatedTagObject,
		ledgerschema.ReleaseArchiveSHA256,
		ledgerschema.ReleaseChecksumsSHA256,
		ledgerschema.SchemaSHA256,
		ledgerschema.ForecastSealProtocol,
		ledgerschema.ForecastTargetProfile,
	}
	for index := range pins {
		if actual[index] != pins[index] {
			t.Fatalf("schema pin %d = %q, want %q", index, actual[index], pins[index])
		}
	}

	root := repositoryRoot(t)
	for _, name := range []string{
		"AGENTS.md",
		filepath.Join("third_party", "forecast-ledger", "README.md"),
		filepath.Join("internal", "schema", "upstream", "forecast-ledger", "v2.0.0", "SOURCE.md"),
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, pin := range pins[:6] {
			if !strings.Contains(string(data), pin) {
				t.Errorf("%s does not record release pin %s", name, pin)
			}
		}
	}
}

func TestForecastLedgerUpdateGuidanceNamesCoupledEvidence(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	guidance := string(data)
	for _, required := range []string{
		"exact commit",
		"tag object",
		"release archive digest",
		"checksum-asset digest",
		"schema digest",
		"attribution",
		"fixtures",
		"reference semantics",
		"vectors",
	} {
		if !strings.Contains(guidance, required) {
			t.Errorf("schema update guidance does not name %q", required)
		}
	}
}

func TestReleasePackagesCarryThirdPartyNotice(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "THIRD_PARTY_NOTICES.md"); count < 3 {
		t.Fatalf("release configuration names THIRD_PARTY_NOTICES.md %d times, want archive source plus Linux package source and destination", count)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
