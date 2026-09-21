package forecastcrypto

import (
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

func TestRevealRejectsInvalidKeyWithoutLeakingIt(t *testing.T) {
	const secret = "secret-not-hex"
	commitment := ledger.RevealedCommitment{RevealedKey: ledger.Hex32(secret)}
	_, err := Reveal("question", "revision", "forecast", commitment)
	if err == nil {
		t.Fatal("invalid key accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked key material: %q", err)
	}
}
