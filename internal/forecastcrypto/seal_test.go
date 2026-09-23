package forecastcrypto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"golang.org/x/crypto/chacha20poly1305"
)

type vectorEntropy struct{ reader io.Reader }

func (source vectorEntropy) ReadFull(_ context.Context, destination []byte) error {
	_, err := io.ReadFull(source.reader, destination)
	return err
}

type sealV3Vector struct {
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

func TestSealMatchesPinnedV3VectorByteForByte(t *testing.T) {
	vector := loadSealV3Vector(t)
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
	wantKeyFile := "{\"commitment_sha256\":\"" + string(vector.Expected.Commitment.CommitmentHash.Value) + "\",\"forecast_id\":\"" + vector.ForecastID + "\",\"key_hex\":\"" + vector.Material.KeyHex + "\",\"question_id\":\"" + vector.QuestionID + "\",\"question_revision_id\":\"" + vector.QuestionRevisionID + "\",\"schema\":\"forecast-key/v3\"}\n"
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
	if _, err := DecodeKeyFile(sealed.KeyFile, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), vector.Expected.Commitment.CommitmentHash.Value); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(sealed.KeyFile, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), sealed.Commitment)
	if err != nil || !reflect.DeepEqual(opened.Bundle, vector.Bundle) || string(opened.KeyHex) != vector.Material.KeyHex || string(opened.Plaintext) != vector.Expected.CanonicalPlaintext {
		t.Fatalf("opened = %#v, error = %v", opened, err)
	}
}

func TestKeyFileMatchesPinnedV3VectorByteForByte(t *testing.T) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-key-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		QuestionID         string `json:"question_id"`
		QuestionRevisionID string `json:"question_revision_id"`
		ForecastID         string `json:"forecast_id"`
		CommitmentSHA256   string `json:"commitment_sha256"`
		KeyHex             string `json:"key_hex"`
		Expected           struct {
			CanonicalFile string `json:"canonical_file"`
			FileSHA256    string `json:"file_sha256"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeKeyFile(ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), ledger.Hex32(vector.CommitmentSHA256), decodeHex(t, vector.KeyHex))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if string(encoded) != vector.Expected.CanonicalFile || hex.EncodeToString(digest[:]) != vector.Expected.FileSHA256 {
		t.Fatalf("key vector mismatch: %q sha=%x", encoded, digest)
	}
	if _, err := DecodeKeyFile(encoded, ledger.Slug(vector.QuestionID), ledger.Slug(vector.QuestionRevisionID), ledger.Slug(vector.ForecastID), ledger.Hex32(vector.CommitmentSHA256)); err != nil {
		t.Fatal(err)
	}
}

func TestSealMatchesPinnedPresenceVectorsByteForByte(t *testing.T) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-seal-v3-presence.json")
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
	digest := ledger.Hex32(strings.Repeat("a", 64))
	data, err := EncodeKeyFile("q-one", "r-one", "f-one", digest, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][3]ledger.Slug{{"q-two", "r-one", "f-one"}, {"q-one", "r-two", "f-one"}, {"q-one", "r-one", "f-two"}} {
		if _, err := DecodeKeyFile(data, ids[0], ids[1], ids[2], digest); err == nil {
			t.Fatalf("wrong binding accepted: %v", ids)
		}
	}
	nonCanonical := []byte("{\"schema\":\"forecast-key/v3\",\"question_id\":\"q-one\",\"question_revision_id\":\"r-one\",\"forecast_id\":\"f-one\",\"commitment_sha256\":\"" + string(digest) + "\",\"key_hex\":\"" + hex.EncodeToString(key) + "\"}\n")
	if _, err := DecodeKeyFile(nonCanonical, "q-one", "r-one", "f-one", digest); err == nil {
		t.Fatal("non-canonical key file accepted")
	}
}

func TestOpenRejectsRevisionTransplantWrongKeyAndTamperedCiphertext(t *testing.T) {
	bundle := testPrivateBundle()
	sealed, err := Seal(context.Background(), "q-one", "r-one", "f-one", bundle, "forecast-key:f-one", vectorEntropy{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := EncodeKeyFile("q-one", "r-one", "f-one", sealed.Commitment.CommitmentHash.Value, bytes.Repeat([]byte{0x24}, 32))
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

func TestOpenReportsClosedV3FailureStages(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, chacha20poly1305.KeySize)
	sealed, err := Seal(t.Context(), "q-one", "r-one", "f-one", testPrivateBundle(), "forecast-key:f-one", vectorEntropy{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 76))})
	if err != nil {
		t.Fatal(err)
	}
	assertStage := func(name string, keyFile []byte, commitment ledger.SealedCommitment, want FailureStage) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			_, err := Open(keyFile, "q-one", "r-one", "f-one", commitment)
			if got := FailureStageOf(err); got != want {
				t.Fatalf("stage = %q, want %q (error %v)", got, want, err)
			}
		})
	}

	old := sealed.Commitment
	old.Scheme = "forecast-seal/v2"
	assertStage("superseded algorithm", sealed.KeyFile, old, StageAlgorithmUnsupported)

	badNonce := sealed.Commitment
	badNonce.Encryption.Nonce = "not-base64"
	assertStage("nonce", sealed.KeyFile, badNonce, StageNonceEncodingInvalid)

	badCiphertext := sealed.Commitment
	badCiphertext.Encryption.Ciphertext = "not-base64"
	assertStage("ciphertext", sealed.KeyFile, badCiphertext, StageCiphertextEncodingInvalid)

	wrongKeyFile, err := EncodeKeyFile("q-one", "r-one", "f-one", sealed.Commitment.CommitmentHash.Value, bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	assertStage("authentication", wrongKeyFile, sealed.Commitment, StageAuthenticationFailed)

	profilePlaintext := []byte(`{"schema":"forecast-seal/v3","question_id":"q-one","question_revision_id":"r-one","forecast_id":"f-one","bundle":{"representations":[],"unknown":true},"salt":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	profileCommitment, profileKeyFile := authenticatedTestCommitment(t, key, profilePlaintext, "q-one", "r-one", "f-one")
	assertStage("closed profile", profileKeyFile, profileCommitment, StageBundleProfileMismatch)
}

func authenticatedTestCommitment(t *testing.T, key, plaintext []byte, questionID, revisionID, forecastID ledger.Slug) (ledger.SealedCommitment, []byte) {
	t.Helper()
	digest := sha256.Sum256(plaintext)
	digestHex := ledger.Hex32(hex.EncodeToString(digest[:]))
	aad, err := sealAAD(questionID, revisionID, forecastID, string(digestHex))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{0x33}, chacha20poly1305.NonceSize)
	ciphertext := cipher.Seal(nil, nonce, plaintext, aad)
	commitment := ledger.SealedCommitment{
		Scheme: SealScheme, CommitmentHash: ledger.Digest{Algorithm: "sha-256", Value: digestHex},
		Encryption: ledger.Encryption{Algorithm: EncryptionProfile, Nonce: ledger.Base64Nonce12(base64.StdEncoding.EncodeToString(nonce)), Ciphertext: ledger.Base64Ciphertext(base64.StdEncoding.EncodeToString(ciphertext))},
		KeyHint:    "forecast-key:f-one",
	}
	keyFile, err := EncodeKeyFile(questionID, revisionID, forecastID, digestHex, key)
	if err != nil {
		t.Fatal(err)
	}
	return commitment, keyFile
}

func loadSealV3Vector(t *testing.T) sealV3Vector {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-seal-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector sealV3Vector
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
	digest := ledger.Hex32(strings.Repeat("a", 64))
	valid, err := EncodeKeyFile("q-one", "r-one", "f-one", digest, bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"schema":"forecast-key/v3"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeKeyFile(data, "q-one", "r-one", "f-one", digest)
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
