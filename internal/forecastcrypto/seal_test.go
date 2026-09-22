package forecastcrypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"reflect"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

type vectorEntropy struct{ reader io.Reader }

func (source vectorEntropy) ReadFull(_ context.Context, destination []byte) error {
	_, err := io.ReadFull(source.reader, destination)
	return err
}

type sealV2Vector struct {
	QuestionID         string        `json:"question_id"`
	QuestionRevisionID string        `json:"question_revision_id"`
	ForecastID         string        `json:"forecast_id"`
	Bundle             PrivateBundle `json:"bundle"`
	Material           struct {
		SaltHex  string `json:"salt_hex"`
		KeyHex   string `json:"key_hex"`
		NonceHex string `json:"nonce_hex"`
	} `json:"material"`
	Expected struct {
		CanonicalPlaintext string `json:"canonical_plaintext"`
		Commitment         struct {
			Scheme         string            `json:"scheme"`
			CommitmentHash ledger.Digest     `json:"commitment_hash"`
			Encryption     ledger.Encryption `json:"encryption"`
			KeyHint        string            `json:"key_hint"`
		} `json:"commitment"`
	} `json:"expected"`
}

type sealPresenceVector struct {
	QuestionID         string `json:"question_id"`
	QuestionRevisionID string `json:"question_revision_id"`
	ForecastID         string `json:"forecast_id"`
	KeyHint            string `json:"key_hint"`
	Material           struct {
		SaltHex  string `json:"salt_hex"`
		KeyHex   string `json:"key_hex"`
		NonceHex string `json:"nonce_hex"`
	} `json:"material"`
	Cases []struct {
		Name     string        `json:"name"`
		Bundle   PrivateBundle `json:"bundle"`
		Expected struct {
			CanonicalPlaintext string `json:"canonical_plaintext"`
			CommitmentSHA256   string `json:"commitment_sha256"`
			CiphertextBase64   string `json:"ciphertext_base64"`
		} `json:"expected"`
	} `json:"cases"`
}

func TestSealMatchesPinnedV2VectorByteForByte(t *testing.T) {
	vector := loadSealV2Vector(t)
	material := append(decodeHex(t, vector.Material.SaltHex), decodeHex(t, vector.Material.KeyHex)...)
	material = append(material, decodeHex(t, vector.Material.NonceHex)...)
	sealed, err := Seal(context.Background(), ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), vector.Bundle, vector.Expected.Commitment.KeyHint, vectorEntropy{reader: bytes.NewReader(material)})
	if err != nil {
		t.Fatal(err)
	}
	wantCommitment := ledger.SealedCommitment{
		Scheme: vector.Expected.Commitment.Scheme, CommitmentHash: vector.Expected.Commitment.CommitmentHash,
		Encryption: vector.Expected.Commitment.Encryption, KeyHint: vector.Expected.Commitment.KeyHint,
	}
	if !reflect.DeepEqual(sealed.Commitment, wantCommitment) {
		t.Fatalf("commitment = %#v, want %#v", sealed.Commitment, wantCommitment)
	}
	wantKeyFile := "{\"forecast_id\":\"" + vector.ForecastID + "\",\"key_hex\":\"" + vector.Material.KeyHex + "\",\"question_id\":\"" + vector.QuestionID + "\",\"question_revision_id\":\"" + vector.QuestionRevisionID + "\",\"schema\":\"forecast-key/v2\"}\n"
	if string(sealed.KeyFile) != wantKeyFile {
		t.Fatalf("key file = %q, want %q", sealed.KeyFile, wantKeyFile)
	}
	revealed := ledger.RevealedCommitment{
		Scheme: sealed.Commitment.Scheme, CommitmentHash: sealed.Commitment.CommitmentHash,
		Encryption: sealed.Commitment.Encryption, KeyHint: sealed.Commitment.KeyHint,
		RevealedKey: ledger.Hex32(vector.Material.KeyHex),
	}
	payload, err := Reveal(ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), revealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.Plaintext) != vector.Expected.CanonicalPlaintext || !reflect.DeepEqual(payload.Bundle, vector.Bundle) {
		t.Fatalf("revealed payload differs: %#v", payload)
	}
	if _, err := DecodeKeyFile(sealed.KeyFile, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID)); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(sealed.KeyFile, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), sealed.Commitment)
	if err != nil || !reflect.DeepEqual(opened.Bundle, vector.Bundle) || string(opened.KeyHex) != vector.Material.KeyHex || string(opened.Plaintext) != vector.Expected.CanonicalPlaintext {
		t.Fatalf("opened = %#v, error = %v", opened, err)
	}
}

func TestSealMatchesPinnedPresenceVectorsByteForByte(t *testing.T) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-seal-v2-presence.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector sealPresenceVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range vector.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			material := append(decodeHex(t, vector.Material.SaltHex), decodeHex(t, vector.Material.KeyHex)...)
			material = append(material, decodeHex(t, vector.Material.NonceHex)...)
			sealed, err := Seal(t.Context(), ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), testCase.Bundle, vector.KeyHint, vectorEntropy{reader: bytes.NewReader(material)})
			if err != nil {
				t.Fatal(err)
			}
			if string(sealed.Commitment.CommitmentHash.Value) != testCase.Expected.CommitmentSHA256 || string(sealed.Commitment.Encryption.Ciphertext) != testCase.Expected.CiphertextBase64 {
				t.Fatalf("commitment differs: %#v", sealed.Commitment)
			}
			opened, err := Open(sealed.KeyFile, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), sealed.Commitment)
			if err != nil || string(opened.Plaintext) != testCase.Expected.CanonicalPlaintext || !reflect.DeepEqual(opened.Bundle, testCase.Bundle) {
				t.Fatalf("presence round trip = %#v, %v", opened.Bundle, err)
			}
		})
	}
}

func TestKeyFileRejectsNonCanonicalAndWrongThreeIDBinding(t *testing.T) {
	key := bytes.Repeat([]byte{0xab}, 32)
	data, err := EncodeKeyFile("q-one", "r-one", "f-one", key)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][3]ledger.Slug{{"q-two", "r-one", "f-one"}, {"q-one", "r-two", "f-one"}, {"q-one", "r-one", "f-two"}} {
		if _, err := DecodeKeyFile(data, ids[0], ids[1], ids[2]); err == nil {
			t.Fatalf("wrong binding accepted: %v", ids)
		}
	}
	nonCanonical := []byte("{\"schema\":\"forecast-key/v2\",\"question_id\":\"q-one\",\"question_revision_id\":\"r-one\",\"forecast_id\":\"f-one\",\"key_hex\":\"" + hex.EncodeToString(key) + "\"}\n")
	if _, err := DecodeKeyFile(nonCanonical, "q-one", "r-one", "f-one"); err == nil {
		t.Fatal("non-canonical key file accepted")
	}
}

func TestOpenRejectsRevisionTransplantWrongKeyAndTamperedCiphertext(t *testing.T) {
	bundle := testPrivateBundle()
	sealed, err := Seal(context.Background(), "q-one", "r-one", "f-one", bundle, "forecast-key:f-one", vectorEntropy{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := EncodeKeyFile("q-one", "r-one", "f-one", bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(wrong, "q-one", "r-one", "f-one", sealed.Commitment); err == nil {
		t.Fatal("wrong key authenticated")
	}
	if _, err := Open(sealed.KeyFile, "q-one", "r-two", "f-one", sealed.Commitment); err == nil {
		t.Fatal("revision transplant authenticated")
	}
	tampered := sealed.Commitment
	ciphertext, err := base64.StdEncoding.DecodeString(string(tampered.Encryption.Ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	tampered.Encryption.Ciphertext = ledger.Base64Ciphertext(base64.StdEncoding.EncodeToString(ciphertext))
	if _, err := Open(sealed.KeyFile, "q-one", "r-one", "f-one", tampered); err == nil {
		t.Fatal("tampered ciphertext authenticated")
	}
}

func loadSealV2Vector(t *testing.T) sealV2Vector {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-seal-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector sealV2Vector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	return vector
}

func testPrivateBundle() PrivateBundle {
	rationale, factors, comment := "private", []string{}, "private"
	return PrivateBundle{
		Representations: []ledger.ForecastRepresentation{{Probability: &ledger.ProbabilityRepresentation{Kind: ledger.RepresentationProbability, Outcome: true, Probability: "0.5"}}},
		Rationale:       &rationale, KeyFactors: &factors, Comment: &comment,
	}
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func FuzzDecodeKeyFile(f *testing.F) {
	valid, err := EncodeKeyFile("q-one", "r-one", "f-one", bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"schema":"forecast-key/v2"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeKeyFile(data, "q-one", "r-one", "f-one")
	})
}

func FuzzOpenSealedForecast(f *testing.F) {
	sealed, err := Seal(context.Background(), "q-one", "r-one", "f-one", testPrivateBundle(), "forecast-key:f-one", vectorEntropy{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(sealed.KeyFile, string(sealed.Commitment.Encryption.Ciphertext))
	f.Add([]byte("not-a-key"), "not-base64")
	f.Fuzz(func(t *testing.T, keyFile []byte, ciphertext string) {
		commitment := sealed.Commitment
		commitment.Encryption.Ciphertext = ledger.Base64Ciphertext(ciphertext)
		_, _ = Open(keyFile, "q-one", "r-one", "f-one", commitment)
	})
}
