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
		"2.1.0",
		"d6ceebe4d42eac9f9e6d6df18167dc0dbf253bd4",
		"6d74d483f7cb177470ba77f4fcb4063fe04f4003",
		"f417a40f1ddd0ef8448a983db8c6836241320cf914987554f077e90d6222ec17",
		"6b7437ed7bd1834792039d8a0b360f72e70710eb9a707336ba016fad0874799b",
		"cb4ba1d3c71c9fd824fb49d2e08defa01a3fa69bd70edd8139ce808d31cd5107",
		"forecast-seal/v2",
		"forecast-envelope/v2",
		"forecast-lifecycle/v1",
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
		filepath.Join("internal", "schema", "upstream", "forecast-ledger", "v2.1.0", "SOURCE.md"),
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
