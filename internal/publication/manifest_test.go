package publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestManifestPublishedVectors(t *testing.T) {
	for _, name := range []string{"forecast-ledger-publication-v3.json", "forecast-ledger-publication-v3-empty.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "schema", "upstream", "forecast-ledger", "v2.2.0", "tests", "vectors", name))
			if err != nil {
				t.Fatal(err)
			}
			var vector struct {
				Manifest Manifest `json:"manifest"`
				Expected struct {
					Canonical string `json:"canonical_json"`
					SHA256    string `json:"sha256"`
				} `json:"expected"`
			}
			if err := json.Unmarshal(raw, &vector); err != nil {
				t.Fatal(err)
			}
			encoded, err := Encode(vector.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != vector.Expected.Canonical {
				t.Fatal("manifest canonical bytes differ from the published vector")
			}
			digest := sha256.Sum256(encoded)
			if hex.EncodeToString(digest[:]) != vector.Expected.SHA256 {
				t.Fatalf("manifest digest = %x", digest)
			}
		})
	}
}

func TestManifestCanonicalRoundTripAndClosedRoles(t *testing.T) {
	manifest := testManifest("ledger/example.yaml")
	encoded, err := Encode(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || decoded.LedgerPath != manifest.LedgerPath {
		t.Fatalf("Decode = %#v, %v", decoded, err)
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(RoleLedger), []byte("secret"), 1)); err == nil {
		t.Fatal("unknown manifest role accepted")
	}
}

func TestManifestRejectsUnsafePathsRolesAndCollisions(t *testing.T) {
	base := Manifest{
		Profile: ManifestProfile, Contract: CurrentContractIdentity(), LedgerPath: "ledger/example.yaml", EvidenceIndexPath: EvidenceIndexPath,
		Entries: []Entry{{
			Role: RoleLedger, Path: "ledger/example.yaml", Size: 10,
			Digest: Digest{Algorithm: "sha-256", Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		}, {Role: RoleEvidenceIndex, Path: EvidenceIndexPath, Size: 20, Digest: Digest{Algorithm: "sha-256", Value: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}}},
	}
	tests := map[string]func(*Manifest){
		"traversal":     func(manifest *Manifest) { manifest.Entries[0].Path = "ledger/../example.yaml" },
		"wrong ledger":  func(manifest *Manifest) { manifest.LedgerPath = "ledger/other.yaml" },
		"wrong role":    func(manifest *Manifest) { manifest.Entries[0].Role = RoleTarget },
		"unknown role":  func(manifest *Manifest) { manifest.Entries[0].Role = "secret" },
		"wrong digest":  func(manifest *Manifest) { manifest.Entries[0].Digest.Value = "ABC" },
		"negative size": func(manifest *Manifest) { manifest.Entries[0].Size = -1 },
		"duplicate":     func(manifest *Manifest) { manifest.Entries = append(manifest.Entries, manifest.Entries[0]) },
		"case collision": func(manifest *Manifest) {
			manifest.Entries = append(manifest.Entries, Entry{Role: RoleTarget, Path: "Ledger/Example.yaml", Digest: manifest.Entries[0].Digest})
			slices.SortFunc(manifest.Entries, func(a, b Entry) int {
				if a.Path < b.Path {
					return -1
				}
				if a.Path > b.Path {
					return 1
				}
				return 0
			})
		},
		"request path": func(manifest *Manifest) {
			manifest.Entries = append(manifest.Entries, Entry{Role: RoleRequest, Path: "proofs/targets/request.tsq", Digest: manifest.Entries[0].Digest})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := base
			manifest.Entries = append([]Entry(nil), base.Entries...)
			mutate(&manifest)
			if err := Validate(manifest); err == nil {
				t.Fatal("unsafe manifest succeeded")
			}
		})
	}
}

func FuzzManifestDecode(f *testing.F) {
	manifest := testManifest("ledger/example.json")
	seed, _ := Encode(manifest)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxManifestBytes {
			return
		}
		_, _ = Decode(data)
	})
}

func testManifest(ledgerPath string) Manifest {
	return Manifest{Profile: ManifestProfile, Contract: CurrentContractIdentity(), LedgerPath: ledgerPath, EvidenceIndexPath: EvidenceIndexPath, Entries: []Entry{
		{Role: RoleLedger, Path: ledgerPath, Size: 10, Digest: Digest{Algorithm: "sha-256", Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		{Role: RoleEvidenceIndex, Path: EvidenceIndexPath, Size: 20, Digest: Digest{Algorithm: "sha-256", Value: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}},
	}}
}
