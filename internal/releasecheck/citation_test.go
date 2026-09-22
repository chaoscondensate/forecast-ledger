package releasecheck

import (
	"path/filepath"
	"testing"
)

func TestCurrentCitation(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "CITATION.cff")
	if err := CheckCitation(path, "0.9.1-rc.2", "2026-09-22"); err != nil {
		t.Fatal(err)
	}
}

func TestCitationRejectsReleaseMismatch(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "CITATION.cff")
	if err := CheckCitation(path, "9.9.9", "2026-08-25"); err == nil {
		t.Fatal("CheckCitation accepted a mismatched release version")
	}
}
