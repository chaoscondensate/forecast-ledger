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

	if Version != "2.1.0" || Commit != "d6ceebe4d42eac9f9e6d6df18167dc0dbf253bd4" || AnnotatedTagObject != "6d74d483f7cb177470ba77f4fcb4063fe04f4003" {
		t.Fatalf("unexpected upstream identity %q %q %q", Version, Commit, AnnotatedTagObject)
	}
	if ReleaseArchiveSHA256 != "f417a40f1ddd0ef8448a983db8c6836241320cf914987554f077e90d6222ec17" {
		t.Fatalf("unexpected release archive digest %q", ReleaseArchiveSHA256)
	}
	if ReleaseChecksumsSHA256 != "6b7437ed7bd1834792039d8a0b360f72e70710eb9a707336ba016fad0874799b" {
		t.Fatalf("unexpected release checksums digest %q", ReleaseChecksumsSHA256)
	}
	if ForecastSealProtocol != "forecast-seal/v2" || ForecastTargetProfile != "forecast-envelope/v2" || LifecycleTargetProfile != "forecast-lifecycle/v1" {
		t.Fatalf("unexpected active cryptographic profiles %q %q %q", ForecastSealProtocol, ForecastTargetProfile, LifecycleTargetProfile)
	}
}

func TestEveryRetainedV2UpstreamFileDigest(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"LICENSE":                                                   "7084b3fb14e3a306691af23e58ab0ccfa336b202853740f5e1ea0ebab39cacf2",
		"docs/cryptographic-verification.md":                        "6783377ac3e66742367773e4674f4c271cdb04a4bd0da593e70bde4d73997096",
		"docs/data-model.md":                                        "b995d4a2f7789a744013d7cd36c2ec66f9de933b811f0e286a561c4a6331041a",
		"docs/forecast-verification-workflows.md":                   "4faaf71fbfc5b8d7cf9033d8c2bd1e6da6ff2c6f319965b3c7018435242e002e",
		"docs/releases/v2.1.0.md":                                   "c37f13093145516d2dabd00455fd98f370227789dca7bb65ce0d93c1dd349973",
		"examples/valid/empty-ledger.json":                          "57fd04a9b5020df49aaa9a6211557f5e72e745c35aed5a261d4da3ee9d960820",
		"examples/valid/individual-ledger.json":                     "827eaf3337c3ac1060353672948fed519ba639f844851e625585b4b76d8b27ab",
		"examples/valid/question-without-forecasts.yaml":            "57d7306e756e458604cbc494b4440f23120ea82369a1e1be264771dbd07860b9",
		"examples/valid/team-ledger.yaml":                           "edd008ff3ecc8eb0d111c86a29c660e32a080ce16b4d859d924af939904bf67b",
		"schema/forecast-ledger.schema.json":                        SchemaSHA256,
		"tests/conformance/valid/lifecycle-checkpoints.json":        "c6137dc595e268a3e73cd81cb913b61b3c0a943364f191905028f3aee316a128",
		"tests/conformance/valid/relationships-and-datetime.json":   "7fac440e78ebd003786e853f38062d81697b87c5ee91de80db577f167d2794dc",
		"tests/conformance/valid/revealed-representation-only.json": "96b2ed4f70a140b384e7d40a13f62081f6439b05b61317481480044079cbec5b",
		"tests/invalid-cases.json":                                  "a763383505dfec2be1f76ca1cb5d358f7ce7d433e031630708bde5ef160ebc70",
		"tests/vectors/forecast-envelope-v2-public-lifecycle.json":  "86de57752b213272e3c50c53c68e5528b77a867fa227400cbdc7a8921152557d",
		"tests/vectors/forecast-envelope-v2-sealed-lifecycle.json":  "d91d27ab08eb1b499cc1e2dcddb2c0517194d76adf548fdf48a15d44b1057cd4",
		"tests/vectors/forecast-lifecycle-v1.json":                  "afee5ca4928b564355b57b301be74d3bce3399d063562512b71303a16a5c0bc4",
		"tests/vectors/forecast-seal-v2-presence.json":              "5447c18880497d8366163acf847dcbca510093ada6111f80aa23e3585ad0467d",
		"tests/vectors/forecast-seal-v2.json":                       "9247e3d89d4ae39b6c7f97eb6023ba91e6c958f059d3c3c4cd9b2ea2603c054f",
		"tools/build_targets.py":                                    "fe7c72bbb99bf7f74fe4d310e3a85a70d17d86097b96c8d6c631b5ae92802b5a",
		"tools/forecast_crypto.py":                                  "b5ef1ed51ec0fbfbe1f0c8e4d04732fe267972a698886dd4bbac316cabaa930e",
		"tools/validate.py":                                         "52d512692d610b43314e4b3702898902108f4eea1014a90adee0c3fd78baa84c",
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
