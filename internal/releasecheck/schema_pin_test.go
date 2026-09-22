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
		"2.0.1",
		"55b1431d379128e1d75b9c30a3874398cea9ff0f",
		"ac69f718de92f2c087b44b1b02d325ba37945db6",
		"bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41",
		"fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6",
		"5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b",
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
		filepath.Join("internal", "schema", "upstream", "forecast-ledger", "v2.0.1", "SOURCE.md"),
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
