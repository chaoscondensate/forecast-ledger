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

	if Version != "2.0.1" || Commit != "55b1431d379128e1d75b9c30a3874398cea9ff0f" || AnnotatedTagObject != "ac69f718de92f2c087b44b1b02d325ba37945db6" {
		t.Fatalf("unexpected upstream identity %q %q %q", Version, Commit, AnnotatedTagObject)
	}
	if ReleaseArchiveSHA256 != "bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41" {
		t.Fatalf("unexpected release archive digest %q", ReleaseArchiveSHA256)
	}
	if ReleaseChecksumsSHA256 != "fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6" {
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
		"docs/data-model.md": "ad3f80da79ccae28453a06bcb135ecd69dde2f752f48eb8b7aadf10b292eb72a",
		"docs/forecast-verification-workflows.md":                  "12735d2d33b21c22a62fb02ba919712d585aae122e7f9bf6fecb87c9dccacb30",
		"examples/valid/empty-ledger.json":                         "2be6a175b7bb06c44b661b9133c8987554bfd061cb906aee4cf42f0091f82de2",
		"examples/valid/individual-ledger.json":                    "0c3cc6588c4a8d14a0dcf5481b2983bfe5da79f51a8988e17de0fe7c7734bfc2",
		"examples/valid/question-without-forecasts.yaml":           "b440e8ca967cdacef321dd5be9a47f3a1abcfdf92376e7cb29d3391b913ce7b3",
		"examples/valid/team-ledger.yaml":                          "075d21d6a0353683c092ef8a76ae44abf0a2bc3bced18787d75c426185e3247a",
		"schema/forecast-ledger.schema.json":                       SchemaSHA256,
		"tests/conformance/valid/relationships-and-datetime.json":  "a3b1ba78c212eda277c0c08e76bc9703368b2cfe3031024dc1af8ac04f6bc056",
		"tests/invalid-cases.json":                                 "fe7a68952565ab1ed7d22fe3a9e330216b01f3ccda2068e2f2c5fdda6bf81c1a",
		"tests/vectors/forecast-envelope-v2-public-lifecycle.json": "03ad733808e3b0809af0d6a7d8e4e3f9d39906b79e3318fb9b2f95ad4d1cdc99",
		"tests/vectors/forecast-envelope-v2-sealed-lifecycle.json": "3b03922755895fba91d2d19cbafa11ba5a232ca91b38f46bbc269688e8ee3b3d",
		"tests/vectors/forecast-seal-v2.json":                      "7cd814473f3e84617660e704f1aea8dc48e4b4a32cc86b93fd46c1649a4728d5",
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
