package forecastcrypto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	SealScheme        = "forecast-seal/v3"
	KeyFileSchema     = "forecast-key/v3"
	EncryptionProfile = "chacha20-poly1305"
)

type FailureStage string

const (
	StageKeyFileInvalid            FailureStage = "reveal.key_file_invalid"
	StageKeyFileBindingFailed      FailureStage = "reveal.key_file_binding_failed"
	StageCommitmentMalformed       FailureStage = "reveal.commitment_malformed"
	StageAlgorithmUnsupported      FailureStage = "reveal.algorithm_unsupported"
	StageNonceEncodingInvalid      FailureStage = "reveal.nonce_encoding_invalid"
	StageCiphertextEncodingInvalid FailureStage = "reveal.ciphertext_encoding_invalid"
	StageAuthenticationFailed      FailureStage = "reveal.authentication_failed"
	StageCommitmentDigestMismatch  FailureStage = "reveal.commitment_digest_mismatch"
	StageBundleProfileMismatch     FailureStage = "reveal.bundle_profile_mismatch"
)

type Failure struct{ Stage FailureStage }

func (e *Failure) Error() string { return string(e.Stage) }

func failure(stage FailureStage) error { return &Failure{Stage: stage} }

func FailureStageOf(err error) FailureStage {
	var typed *Failure
	if errors.As(err, &typed) {
		return typed.Stage
	}
	return ""
}

type EntropySource interface {
	ReadFull(context.Context, []byte) error
}

// PrivateBundle is the closed forecast-seal/v3 private value disclosed by a
// reveal. Optional fields are pointers so an absent property remains distinct
// from a present empty string or array.
type PrivateBundle struct {
	Representations []ledger.ForecastRepresentation `json:"representations"`
	Rationale       *string                         `json:"rationale,omitempty"`
	KeyFactors      *[]string                       `json:"key_factors,omitempty"`
	Comment         *string                         `json:"comment,omitempty"`
}

type SealResult struct {
	Commitment ledger.SealedCommitment
	KeyFile    []byte
}

type OpenResult struct {
	Bundle    PrivateBundle
	Plaintext []byte
	KeyHex    ledger.Hex32
}

func Seal(ctx context.Context, questionID, revisionID, forecastID ledger.Slug, bundle PrivateBundle, keyHint string, entropy EntropySource) (SealResult, error) {
	var result SealResult
	if entropy == nil {
		return result, errors.New("entropy source is not configured")
	}
	if revisionID == "" {
		return result, errors.New("sealed bundle question revision is missing from seal context")
	}
	if len(bundle.Representations) == 0 {
		return result, errors.New("sealed bundle representations are required")
	}
	if bundle.KeyFactors != nil {
		for _, factor := range *bundle.KeyFactors {
			if factor == "" {
				return result, errors.New("sealed bundle key factors must be non-empty strings")
			}
		}
	}
	salt := make([]byte, 32)
	key := make([]byte, chacha20poly1305.KeySize)
	nonce := make([]byte, chacha20poly1305.NonceSize)
	defer clear(key)
	defer clear(salt)
	defer clear(nonce)
	for _, destination := range [][]byte{salt, key, nonce} {
		if err := entropy.ReadFull(ctx, destination); err != nil {
			return result, fmt.Errorf("read sealing entropy: %w", err)
		}
	}

	plaintext, err := canonicalJSON(sealedPlaintext{
		Schema: SealScheme, QuestionID: questionID, QuestionRevisionID: revisionID,
		ForecastID: forecastID, Bundle: bundle, Salt: hex.EncodeToString(salt),
	})
	if err != nil {
		return result, fmt.Errorf("canonicalize sealed forecast: %w", err)
	}
	digest := sha256.Sum256(plaintext)
	digestHex := hex.EncodeToString(digest[:])
	aad, err := sealAAD(questionID, revisionID, forecastID, digestHex)
	if err != nil {
		return result, err
	}
	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return result, fmt.Errorf("initialize sealing cipher: %w", err)
	}
	ciphertext := cipher.Seal(nil, nonce, plaintext, aad)
	keyFile, err := EncodeKeyFile(questionID, revisionID, forecastID, ledger.Hex32(digestHex), key)
	if err != nil {
		return result, err
	}
	result.Commitment = ledger.SealedCommitment{
		Scheme:         SealScheme,
		CommitmentHash: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(digestHex)},
		Encryption: ledger.Encryption{
			Algorithm:  EncryptionProfile,
			Nonce:      ledger.Base64Nonce12(base64.StdEncoding.EncodeToString(nonce)),
			Ciphertext: ledger.Base64Ciphertext(base64.StdEncoding.EncodeToString(ciphertext)),
		},
		KeyHint: keyHint,
	}
	result.KeyFile = keyFile
	return result, nil
}

type KeyFile struct {
	Schema             string `json:"schema"`
	QuestionID         string `json:"question_id"`
	QuestionRevisionID string `json:"question_revision_id"`
	ForecastID         string `json:"forecast_id"`
	CommitmentSHA256   string `json:"commitment_sha256"`
	KeyHex             string `json:"key_hex"`
}

func EncodeKeyFile(questionID, revisionID, forecastID ledger.Slug, commitmentSHA256 ledger.Hex32, key []byte) ([]byte, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, errors.New("key must contain exactly 32 bytes")
	}
	if !validHex32(string(commitmentSHA256)) {
		return nil, errors.New("commitment digest must be lowercase SHA-256 hex")
	}
	encoded, err := canonicalJSON(KeyFile{
		Schema: KeyFileSchema, QuestionID: string(questionID), QuestionRevisionID: string(revisionID),
		ForecastID: string(forecastID), CommitmentSHA256: string(commitmentSHA256), KeyHex: hex.EncodeToString(key),
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize key file: %w", err)
	}
	return append(encoded, '\n'), nil
}

func DecodeKeyFile(data []byte, questionID, revisionID, forecastID ledger.Slug, commitmentSHA256 ledger.Hex32) (KeyFile, error) {
	var result KeyFile
	if len(data) == 0 || len(data) > 4096 || data[len(data)-1] != '\n' || bytes.Contains(data[:len(data)-1], []byte{'\n'}) {
		return result, failure(StageKeyFileInvalid)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(data[:len(data)-1]), document.Limits{MaxBytes: 4096, MaxDepth: 8, MaxNodes: 16, MaxScalarBytes: 256})
	if err != nil {
		return result, failure(StageKeyFileInvalid)
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil || !bytes.Equal(canonicalBytes, data[:len(data)-1]) {
		return result, failure(StageKeyFileInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(data[:len(data)-1]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return KeyFile{}, failure(StageKeyFileInvalid)
	}
	decoded, err := hex.DecodeString(result.KeyHex)
	defer clear(decoded)
	if err != nil || len(decoded) != chacha20poly1305.KeySize || result.KeyHex != hex.EncodeToString(decoded) || !validHex32(result.CommitmentSHA256) {
		return KeyFile{}, failure(StageKeyFileInvalid)
	}
	if result.Schema != KeyFileSchema || result.QuestionID != string(questionID) || result.QuestionRevisionID != string(revisionID) || result.ForecastID != string(forecastID) || result.CommitmentSHA256 != string(commitmentSHA256) {
		return KeyFile{}, failure(StageKeyFileBindingFailed)
	}
	return result, nil
}

func Open(keyFileBytes []byte, questionID, revisionID, forecastID ledger.Slug, commitment ledger.SealedCommitment) (OpenResult, error) {
	var result OpenResult
	keyFile, err := DecodeKeyFile(keyFileBytes, questionID, revisionID, forecastID, commitment.CommitmentHash.Value)
	if err != nil {
		return result, err
	}
	key, err := hex.DecodeString(keyFile.KeyHex)
	if err != nil || len(key) != chacha20poly1305.KeySize {
		clear(key)
		return result, failure(StageKeyFileInvalid)
	}
	defer clear(key)
	bundle, plaintext, err := openWithKey(key, questionID, revisionID, forecastID, commitment)
	if err != nil {
		return result, err
	}
	result.Bundle = bundle
	result.Plaintext = plaintext
	result.KeyHex = ledger.Hex32(keyFile.KeyHex)
	return result, nil
}

type sealedPlaintext struct {
	Schema             string        `json:"schema"`
	QuestionID         ledger.Slug   `json:"question_id"`
	QuestionRevisionID ledger.Slug   `json:"question_revision_id"`
	ForecastID         ledger.Slug   `json:"forecast_id"`
	Bundle             PrivateBundle `json:"bundle"`
	Salt               string        `json:"salt"`
}

func openWithKey(key []byte, questionID, revisionID, forecastID ledger.Slug, commitment ledger.SealedCommitment) (PrivateBundle, []byte, error) {
	var empty PrivateBundle
	if len(key) != chacha20poly1305.KeySize {
		return empty, nil, failure(StageKeyFileInvalid)
	}
	if !validHex32(string(commitment.CommitmentHash.Value)) {
		return empty, nil, failure(StageCommitmentMalformed)
	}
	if commitment.Scheme != SealScheme || commitment.CommitmentHash.Algorithm != "sha-256" || commitment.Encryption.Algorithm != EncryptionProfile {
		return empty, nil, failure(StageAlgorithmUnsupported)
	}
	nonce, err := base64.StdEncoding.Strict().DecodeString(string(commitment.Encryption.Nonce))
	if err != nil || len(nonce) != chacha20poly1305.NonceSize {
		clear(nonce)
		return empty, nil, failure(StageNonceEncodingInvalid)
	}
	defer clear(nonce)
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(string(commitment.Encryption.Ciphertext))
	if err != nil || len(ciphertext) < chacha20poly1305.Overhead || int64(len(ciphertext)) > document.DefaultLimits.MaxBytes {
		clear(ciphertext)
		return empty, nil, failure(StageCiphertextEncodingInvalid)
	}
	defer clear(ciphertext)
	aad, err := sealAAD(questionID, revisionID, forecastID, string(commitment.CommitmentHash.Value))
	if err != nil {
		return empty, nil, err
	}
	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return empty, nil, fmt.Errorf("initialize sealing cipher: %w", err)
	}
	plaintext, err := cipher.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return empty, nil, failure(StageAuthenticationFailed)
	}
	digest := sha256.Sum256(plaintext)
	expected, err := hex.DecodeString(string(commitment.CommitmentHash.Value))
	if err != nil || len(expected) != sha256.Size || subtle.ConstantTimeCompare(digest[:], expected) != 1 {
		clear(plaintext)
		return empty, nil, failure(StageCommitmentDigestMismatch)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(plaintext), document.DefaultLimits)
	if err != nil {
		clear(plaintext)
		return empty, nil, failure(StageBundleProfileMismatch)
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil || !bytes.Equal(canonicalBytes, plaintext) {
		clear(plaintext)
		return empty, nil, failure(StageBundleProfileMismatch)
	}
	var sealed sealedPlaintext
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sealed); err != nil {
		clear(plaintext)
		return empty, nil, failure(StageBundleProfileMismatch)
	}
	reencoded, err := canonicalJSON(sealed)
	if err != nil || !bytes.Equal(reencoded, plaintext) {
		clear(plaintext)
		return empty, nil, failure(StageBundleProfileMismatch)
	}
	salt, saltErr := hex.DecodeString(sealed.Salt)
	defer clear(salt)
	if sealed.Schema != SealScheme || sealed.QuestionID != questionID || sealed.QuestionRevisionID != revisionID || sealed.ForecastID != forecastID || len(sealed.Bundle.Representations) == 0 || saltErr != nil || len(salt) != 32 || sealed.Salt != hex.EncodeToString(salt) {
		clear(plaintext)
		return empty, nil, failure(StageBundleProfileMismatch)
	}
	if sealed.Bundle.KeyFactors != nil {
		for _, factor := range *sealed.Bundle.KeyFactors {
			if factor == "" {
				clear(plaintext)
				return empty, nil, failure(StageBundleProfileMismatch)
			}
		}
	}
	return sealed.Bundle, bytes.Clone(plaintext), nil
}

func validHex32(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}

func sealAAD(questionID, revisionID, forecastID ledger.Slug, digest string) ([]byte, error) {
	result, err := canonical.Marshal(map[string]any{
		"scheme": SealScheme, "question_id": string(questionID), "question_revision_id": string(revisionID),
		"forecast_id": string(forecastID), "commitment_sha256": digest,
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize seal associated data: %w", err)
	}
	return result, nil
}

func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return nil, err
	}
	return canonical.Marshal(parsed.Root.Any())
}
