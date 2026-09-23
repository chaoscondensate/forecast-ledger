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

	if Version != "2.2.0" || Commit != "ae02de9ebca3eb2ae87c480596bf620bdcdced11" || AnnotatedTagObject != "8feb323ce895a6a197183c4eb857a55463648873" {
		t.Fatalf("unexpected upstream identity %q %q %q", Version, Commit, AnnotatedTagObject)
	}
	if ReleaseArchiveSHA256 != "e8f92450e7e73eb559e762878dd4188156968cdad6328c03ea94d6b0c01ff419" {
		t.Fatalf("unexpected release archive digest %q", ReleaseArchiveSHA256)
	}
	if ReleaseChecksumsSHA256 != "7e1d79e6d8bd4df20a5877ef51c17f149cec7619d49ddfa2fff82899034989a0" {
		t.Fatalf("unexpected release checksums digest %q", ReleaseChecksumsSHA256)
	}
	if ForecastSealProtocol != "forecast-seal/v3" || ForecastTargetProfile != "forecast-envelope/v3" || LifecycleTargetProfile != "forecast-lifecycle/v2" {
		t.Fatalf("unexpected active cryptographic profiles %q %q %q", ForecastSealProtocol, ForecastTargetProfile, LifecycleTargetProfile)
	}
}

func TestEveryRetainedV2UpstreamFileDigest(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"LICENSE":                                                                "7084b3fb14e3a306691af23e58ab0ccfa336b202853740f5e1ea0ebab39cacf2",
		"docs/compatibility-v1-v2.md":                                            "b84274e67ea473a4f24bbcce596258f43a83ea031720e2ef147af4cf69e71d31",
		"docs/cryptographic-verification.md":                                     "f5f13ac9aef479a7844a65bbb210eb68de750d526239fd5524becbd85e9771e7",
		"docs/data-model.md":                                                     "957dbb46b6274411a4e53fc0d92bed1c9390e2431a8be33266ca8978e9237207",
		"docs/evidence-and-publication.md":                                       "37fc99c93d114ae566d66001e453f96ad94a281e2dab733c6829140e2e9508e7",
		"docs/forecast-verification-workflows.md":                                "52b61d2d191724b1efd1faa504b62f0c2f33aabfc084db9512a99846035177b6",
		"docs/releases/v2.0.0.md":                                                "9400e8a23923037b43a95c5e4055743a0d6c87bc725319a88c59541c55031616",
		"docs/releases/v2.0.1.md":                                                "ff324ac1b6e75e463dc15518b8d0f36f72abc1836bef23a365c7cc7c7c6cfb60",
		"docs/releases/v2.1.0.md":                                                "c37f13093145516d2dabd00455fd98f370227789dca7bb65ce0d93c1dd349973",
		"docs/releases/v2.2.0.md":                                                "5ba75cec67398cab95a14d0e866c700b355dc527f6b791a53f09fd30de608d0b",
		"examples/valid/empty-ledger.json":                                       "b706b9fc55cbc329cc5dc7a553759b569604d929ebf4caff0fceb6f03c607862",
		"examples/valid/individual-ledger.json":                                  "ec00ee6146e5d3169d124392238e2cde6ce24c488673530def4698a390c57cbc",
		"examples/valid/question-without-forecasts.yaml":                         "aaf2631f7a408ae740817fda334c124eb97d631d24d1e5f7d02ba3ad2682dab8",
		"examples/valid/team-ledger.yaml":                                        "ef3d37813bba6e10e3607dad172b2027e68544c1131bd6f84f223cb69cef7b6a",
		"schema/forecast-evidence-index.schema.json":                             "49595795c1eca59a66c9cfd84005b8bc097454a967c3f5c6454302e2a0fe8ed9",
		"schema/forecast-key.schema.json":                                        "8fbf464b48d7e5d800b6cce6d07393c73eba9f8961dfa3df894d5c68fae12629",
		"schema/forecast-ledger-publication.schema.json":                         "f14ade6bb357952ecc8d2449fc213b5ee5f8b5a0f6423ecb08f9802c5e25c0a9",
		"schema/forecast-ledger.schema.json":                                     SchemaSHA256,
		"tests/conformance/valid/lifecycle-checkpoints.json":                     "ac6fc0dae3730779236ba2a006d445c692e792a3ce8eaf73861cc6e4fd139ee9",
		"tests/conformance/valid/relationships-and-datetime.json":                "eccaeb97f12f87d2d15f9bfaf239db077cef3fcac679be1488099f023c2f9dd4",
		"tests/conformance/valid/retained-forecast.json":                         "3464a1f0b93541d3f9f59dd77cba8d43f12ae9d6c11acc77667946777b0cb916",
		"tests/conformance/valid/revealed-representation-only.json":              "19b2f194cd155f5278a865484ceb36cd99e85f18009e2b5132e8b2f835f80ca7",
		"tests/invalid-cases.json":                                               "11652e866b5bbbb77ca1c30e9b85c79374145222c65ea62432077ee655c90fe8",
		"tests/vectors/forecast-envelope-v3-public-lifecycle.json":               "a6ac709e621f36a8039bb322efbb2838182a9c4df842b1d1bf75f9580d94a368",
		"tests/vectors/forecast-envelope-v3-sealed-lifecycle.json":               "f9cc18a107b344ef83c41614a806c57c24b5e3b0ac91a65277ab83f507a57e7b",
		"tests/vectors/forecast-evidence-index-v1-empty.json":                    "c9a3e7f7431455f96740416f0e724b858a9032c0f4c0257ff006ab0a968a9ded",
		"tests/vectors/forecast-evidence-index-v1.json":                          "14e25d700438fa3d025bb01951b54bef0c5ed24a1f8fb3dfd8144fec951ee59f",
		"tests/vectors/forecast-key-v3.json":                                     "a7bcefb220729adc4573e11f4d44784bce749d4fcaafc2077cff2fd99d9e015f",
		"tests/vectors/forecast-ledger-publication-v3-empty.json":                "1e6e35bb94076b49754de536ad2356f83e476d6856d9fd7d678d459e7d56506c",
		"tests/vectors/forecast-ledger-publication-v3.json":                      "db9356768c09fa2d99f9e00fbaa584746f3d5d7f0793933f96e63d56d91c68a1",
		"tests/vectors/forecast-lifecycle-v2.json":                               "18a6e1a3af2943b36c8c610f0ac1c667d75e618d750a4cae55ddd2ebda1713c8",
		"tests/vectors/forecast-seal-v3-presence.json":                           "11561296548258db800ef7a6bcfdff9de69c86e30c835a7a84f92ad99f005d3c",
		"tests/vectors/forecast-seal-v3.json":                                    "247c46adca3d50e2f118157c0a6faa1a0891d81abdda6fdd1e7e88a5f9aa4399",
		"tests/vectors/targets/f-central-bank-cut-001.forecast-envelope-v3.json": "1b7d5f39efa72e891b4ad573503b3665eea1c5aeddb74f4072e05aaa3c7a6f7b",
		"tests/vectors/targets/forecast-lifecycle-v2-reaffirmed.json":            "82df2e6642b33724f07e498b81775b031b4f52b66736d76f8374baa126277fcc",
		"tests/vectors/targets/forecast-lifecycle-v2-withdrawn.json":             "ac44035e919163f36c4815ed8e6927493ed5037cc9d560fc86262f1aae9406c4",
		"tools/build_targets.py":                                                 "6ecb41a0ff62b3f8945155df56d118e55944f342ff08446b4a276b978a2b8ee0",
		"tools/forecast_crypto.py":                                               "3d08d4b3cdc8a54237647c05caae78bbd801225082ca614676282fc2e438cf00",
		"tools/generate_vectors.py":                                              "63b7da28af01872ab1845403e6c4fa05018edcb23a8590ebc0cef30e55374d1d",
		"tools/run_diagnostic_tests.py":                                          "36d6f9a0c20ead1214181cdab6bfd6d53336fa0643ec77bff24461ab8cb9c39d",
		"tools/run_fixture_tests.py":                                             "c8640fb9f5f97b31460f573ee6ccfc09aa64510eb3c3f35dea6d4b15ad5eb26a",
		"tools/run_seal_tests.py":                                                "4865646e8427fe3684abc3f35a332f7758f4299714ae7eed5a878af6a6c3ca0c",
		"tools/run_sidecar_tests.py":                                             "b9e630bdc4b05d8ec4110278e570200081212b16fb105f63fd401a5875e61ab2",
		"tools/run_target_tests.py":                                              "d501745e339d3f0a6ec9a0a1dcb7a4290edbddfd7405356c9a516d9b6d5f046d",
		"tools/run_transition_tests.py":                                          "7ef8c5838abbf6d79bef814d6dc58605f7c9b800e9e4fa12dbc50faa84d94efc",
		"tools/sidecar_contracts.py":                                             "f73aa98758258b538710d8a41f40821e65aa350da112a5215ddad753a4d216e2",
		"tools/validate.py":                                                      "f0b322b5f55828a8d8127b9c9628032b1281d30df387032344d63f1a092f3344",
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
