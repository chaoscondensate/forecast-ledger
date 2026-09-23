package publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"testing"

	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

type evidenceIndexVector struct {
	AllowEmpty bool          `json:"allow_empty"`
	Index      EvidenceIndex `json:"index"`
	Expected   struct {
		CanonicalJSON string `json:"canonical_json"`
		SHA256        string `json:"sha256"`
	} `json:"expected"`
}

func TestEvidenceIndexMatchesPublishedVectorsByteForByte(t *testing.T) {
	for _, name := range []string{"forecast-evidence-index-v1.json", "forecast-evidence-index-v1-empty.json"} {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/"+name)
			if err != nil {
				t.Fatal(err)
			}
			var vector evidenceIndexVector
			if err := json.Unmarshal(data, &vector); err != nil {
				t.Fatal(err)
			}
			encoded, err := EncodeEvidenceIndex(vector.Index, vector.AllowEmpty)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(encoded)
			if string(encoded) != vector.Expected.CanonicalJSON || hex.EncodeToString(digest[:]) != vector.Expected.SHA256 {
				t.Fatalf("index vector mismatch: %s sha=%x", encoded, digest)
			}
			decoded, err := DecodeEvidenceIndex(encoded, vector.AllowEmpty)
			if err != nil || decoded.LedgerID != vector.Index.LedgerID || len(decoded.Entries) != len(vector.Index.Entries) {
				t.Fatalf("decode = %#v, %v", decoded, err)
			}
			if len(vector.Index.Entries) == 0 {
				if _, err := DecodeEvidenceIndex(encoded, false); err == nil {
					t.Fatal("empty local evidence index accepted")
				}
			}
		})
	}
}

func TestEvidenceIndexRejectsNonCanonicalDuplicateAndUnknownJSON(t *testing.T) {
	vector := loadEvidenceIndexVector(t, "forecast-evidence-index-v1.json")
	encoded, err := EncodeEvidenceIndex(vector.Index, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"trailing LF":   append(bytes.Clone(encoded), '\n'),
		"duplicate key": bytes.Replace(encoded, []byte(`"ledger_id":"vector-ledger"`), []byte(`"ledger_id":"vector-ledger","ledger_id":"other"`), 1),
		"unknown field": bytes.Replace(encoded, []byte(`"ledger_id":"vector-ledger"`), []byte(`"ledger_id":"vector-ledger","unknown":true`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeEvidenceIndex(data, false); err == nil {
				t.Fatal("malformed evidence index accepted")
			}
		})
	}
}

func TestEvidenceIndexRejectsOrderingPathsRolesBindingsAndReferences(t *testing.T) {
	base := loadEvidenceIndexVector(t, "forecast-evidence-index-v1.json").Index
	tests := map[string]func(*EvidenceIndex){
		"wrong profile":  func(index *EvidenceIndex) { index.Schema = "forecast-evidence-index/v0" },
		"wrong contract": func(index *EvidenceIndex) { index.Contract.SchemaVersion = "2.1.0" },
		"unsorted":       func(index *EvidenceIndex) { index.Entries[0], index.Entries[1] = index.Entries[1], index.Entries[0] },
		"duplicate": func(index *EvidenceIndex) {
			index.Entries = append(index.Entries, index.Entries[0])
			SortEvidenceEntries(index.Entries)
		},
		"case collision": func(index *EvidenceIndex) {
			copy := index.Entries[0]
			copy.Path = "proofs/targets/F-vector.json"
			index.Entries = append(index.Entries, copy)
			SortEvidenceEntries(index.Entries)
		},
		"self index": func(index *EvidenceIndex) {
			index.Entries[0].Path = EvidenceIndexPath
			SortEvidenceEntries(index.Entries)
		},
		"outside namespace": func(index *EvidenceIndex) {
			index.Entries[0].Path = "other/target.json"
			SortEvidenceEntries(index.Entries)
		},
		"traversal": func(index *EvidenceIndex) {
			index.Entries[0].Path = "proofs/../target.json"
			SortEvidenceEntries(index.Entries)
		},
		"wrong digest":   func(index *EvidenceIndex) { index.Entries[0].Digest.Value = "ABC" },
		"negative size":  func(index *EvidenceIndex) { index.Entries[0].Size = -1 },
		"missing target": func(index *EvidenceIndex) { index.Entries[2].TargetRef.TargetPath = "proofs/targets/missing.json" },
		"imprint mismatch": func(index *EvidenceIndex) {
			index.Entries[2].Request.MessageImprintSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"missing request": func(index *EvidenceIndex) { index.Entries[3].Response.RequestPath = "proofs/timestamps/missing.tsq" },
		"missing trust": func(index *EvidenceIndex) {
			missing := "trust/missing.pem"
			index.Entries[3].Response.TrustPath = &missing
		},
		"wrong target profile": func(index *EvidenceIndex) { index.Entries[0].Forecast.Scope = "forecast-envelope/v2" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			index := cloneEvidenceIndex(t, base)
			mutate(&index)
			if err := ValidateEvidenceIndex(index, false); err == nil {
				t.Fatal("invalid evidence index accepted")
			}
		})
	}
}

func loadEvidenceIndexVector(t *testing.T, name string) evidenceIndexVector {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/"+name)
	if err != nil {
		t.Fatal(err)
	}
	var vector evidenceIndexVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	return vector
}

func cloneEvidenceIndex(t *testing.T, source EvidenceIndex) EvidenceIndex {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var result EvidenceIndex
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func FuzzEvidenceIndexDecode(f *testing.F) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-evidence-index-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var vector evidenceIndexVector
	if err := json.Unmarshal(data, &vector); err != nil {
		f.Fatal(err)
	}
	seed, err := EncodeEvidenceIndex(vector.Index, false)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{"schema":"forecast-evidence-index/v1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxEvidenceIndexBytes {
			return
		}
		_, _ = DecodeEvidenceIndex(data, false)
	})
}
