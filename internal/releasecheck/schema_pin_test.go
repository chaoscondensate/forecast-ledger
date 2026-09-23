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
		"2.2.0",
		"ae02de9ebca3eb2ae87c480596bf620bdcdced11",
		"8feb323ce895a6a197183c4eb857a55463648873",
		"e8f92450e7e73eb559e762878dd4188156968cdad6328c03ea94d6b0c01ff419",
		"7e1d79e6d8bd4df20a5877ef51c17f149cec7619d49ddfa2fff82899034989a0",
		"6a048928f2573519fd25988ba23f0006b6f8d84eb2f7179d7ff8edd7affa4f9e",
		"forecast-seal/v3",
		"forecast-envelope/v3",
		"forecast-lifecycle/v2",
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
		ledgerschema.LifecycleTargetProfile,
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
		filepath.Join("internal", "schema", "upstream", "forecast-ledger", "v2.2.0", "SOURCE.md"),
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
