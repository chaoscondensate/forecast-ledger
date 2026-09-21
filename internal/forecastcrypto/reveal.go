package forecastcrypto

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"golang.org/x/crypto/chacha20poly1305"
)

var ErrRevealVerification = errors.New("revealed forecast verification failed")

type RevealedPayload struct {
	Schema             string
	QuestionID         ledger.Slug
	QuestionRevisionID ledger.Slug
	ForecastID         ledger.Slug
	Bundle             PrivateBundle
	Plaintext          []byte
}

// Reveal verifies and decrypts a published v2 commitment. Public mirror and
// retained-target comparisons intentionally happen in the semantic verifier
// after this cryptographic boundary succeeds.
func Reveal(questionID, revisionID, forecastID ledger.Slug, commitment ledger.RevealedCommitment) (*RevealedPayload, error) {
	key, err := hex.DecodeString(string(commitment.RevealedKey))
	if err != nil || len(key) != chacha20poly1305.KeySize {
		clear(key)
		return nil, fmt.Errorf("%w: invalid revealed key", ErrRevealVerification)
	}
	defer clear(key)
	sealed := ledger.SealedCommitment{
		Scheme: commitment.Scheme, CommitmentHash: commitment.CommitmentHash,
		Encryption: commitment.Encryption, KeyHint: commitment.KeyHint,
	}
	bundle, plaintext, err := openWithKey(key, questionID, revisionID, forecastID, sealed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRevealVerification, err)
	}
	return &RevealedPayload{
		Schema: SealScheme, QuestionID: questionID, QuestionRevisionID: revisionID,
		ForecastID: forecastID, Bundle: bundle, Plaintext: plaintext,
	}, nil
}
