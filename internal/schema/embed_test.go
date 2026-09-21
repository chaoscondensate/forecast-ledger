package schema

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPinnedV2ReleaseIdentity(t *testing.T) {
	t.Parallel()

	if Version != "2.0.0" || Commit != "1d3b186a15136bc5aff38647cb59fbef475dbe55" || AnnotatedTagObject != "7b4a9e85e0df9350750828a57b03ff729f704ee4" {
		t.Fatalf("unexpected upstream identity %q %q %q", Version, Commit, AnnotatedTagObject)
	}
	if ReleaseArchiveSHA256 != "1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e" {
		t.Fatalf("unexpected release archive digest %q", ReleaseArchiveSHA256)
	}
	if ReleaseChecksumsSHA256 != "77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc" {
		t.Fatalf("unexpected release checksums digest %q", ReleaseChecksumsSHA256)
	}
	if ForecastSealProtocol != "forecast-seal/v2" || ForecastTargetProfile != "forecast-envelope/v2" {
		t.Fatalf("unexpected active cryptographic profiles %q %q", ForecastSealProtocol, ForecastTargetProfile)
	}
}

func TestEveryRetainedV2UpstreamFileDigest(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"LICENSE":            "7084b3fb14e3a306691af23e58ab0ccfa336b202853740f5e1ea0ebab39cacf2",
		"docs/data-model.md": "a2501928944685c9f43307d33bd314f3e046f480c609cefa1981242da7830d9b",
		"docs/forecast-verification-workflows.md":                 "417a2ddaadc54ca5fc019283c33455cd6df909879e2c3c4f4361506eb4b7ca1c",
		"examples/valid/empty-ledger.json":                        "a0d59a71176aeaa7c3ea8dd1ead78a842343b9bcfdea8551ecaeac362e98484d",
		"examples/valid/individual-ledger.json":                   "9d2a69b06085dae161b2772fdec27ee71dc78141e99c8d0fda55c0ffe10f4744",
		"examples/valid/question-without-forecasts.yaml":          "f67b9e622d8d1b2acaa1e2bd0eb33b847d636229d5c4e7041160c3b3b7875d6b",
		"examples/valid/team-ledger.yaml":                         "1e2b88f0e3ce9492a65179307d439523a75b728d247c0363f9615b29c67cf1d9",
		"schema/forecast-ledger.schema.json":                      SchemaSHA256,
		"tests/conformance/valid/relationships-and-datetime.json": "16f1ce02176ca01a90525abceaa23db73b5bcbc305d5185a861e0560d31459b3",
		"tests/invalid-cases.json":                                "fe7a68952565ab1ed7d22fe3a9e330216b01f3ccda2068e2f2c5fdda6bf81c1a",
		"tests/vectors/forecast-seal-v2.json":                     "7cd814473f3e84617660e704f1aea8dc48e4b4a32cc86b93fd46c1649a4728d5",
	}
	sourceRecord, err := fs.ReadFile(Conformance(), "SOURCE.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, digest := range expected {
		entry := "| `" + name + "` | `" + digest + "` |"
		if !strings.Contains(string(sourceRecord), entry) {
			t.Errorf("SOURCE.md does not record %s", entry)
		}
	}

	actualNames := make([]string, 0, len(expected))
	err = fs.WalkDir(Conformance(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path == "SOURCE.md" {
			return nil
		}
		expectedDigest, ok := expected[path]
		if !ok {
			return fmt.Errorf("retained upstream file %q has no pinned digest", path)
		}
		data, err := fs.ReadFile(Conformance(), path)
		if err != nil {
			return err
		}
		assertDigest(t, path, data, expectedDigest)
		actualNames = append(actualNames, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(actualNames)
	expectedNames := make([]string, 0, len(expected))
	for name := range expected {
		expectedNames = append(expectedNames, name)
	}
	sort.Strings(expectedNames)
	if !reflect.DeepEqual(actualNames, expectedNames) {
		t.Fatalf("retained file set mismatch\ngot:  %v\nwant: %v", actualNames, expectedNames)
	}

	assertDigest(t, "active contract", Contract(), SchemaSHA256)
	assertDigest(t, "active license", License(), expected["LICENSE"])
}

func assertDigest(t *testing.T, name string, data []byte, expected string) {
	t.Helper()
	actual := fmt.Sprintf("%x", sha256.Sum256(data))
	if actual != expected {
		t.Fatalf("%s SHA-256 = %s, want %s", name, actual, expected)
	}
}
