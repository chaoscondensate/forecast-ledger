// Package schema embeds the exact supported Forecast Ledger contract and its
// upstream conformance material. Runtime code must never fetch a schema.
package schema

import (
	"embed"
	"io/fs"
)

const (
	Version                = "2.0.0"
	Commit                 = "1d3b186a15136bc5aff38647cb59fbef475dbe55"
	AnnotatedTagObject     = "7b4a9e85e0df9350750828a57b03ff729f704ee4"
	SchemaSHA256           = "efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21"
	ReleaseArchiveSHA256   = "1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e"
	ReleaseChecksumsSHA256 = "77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc"
	ForecastSealProtocol   = "forecast-seal/v2"
	ForecastTargetProfile  = "forecast-envelope/v2"
)

//go:embed upstream/forecast-ledger/v2.0.0/schema/forecast-ledger.schema.json
var contract []byte

//go:embed upstream/forecast-ledger/v2.0.0/LICENSE
var license []byte

//go:embed upstream/forecast-ledger/v2.0.0
var upstream embed.FS

// Contract returns an independent copy of the active v2 schema bytes.
func Contract() []byte {
	return clone(contract)
}

// License returns an independent copy of the exact upstream v2 license bytes.
func License() []byte {
	return clone(license)
}

// Conformance returns the complete retained v2.0.0 upstream release subtree.
// Paths retain their upstream layout, for example examples/valid and
// tests/vectors.
func Conformance() fs.FS {
	return mustSub(upstream, "upstream/forecast-ledger/v2.0.0")
}

// ValidExamples returns the v2 valid-example directory.
func ValidExamples() fs.FS {
	return mustSub(Conformance(), "examples/valid")
}

func clone(source []byte) []byte {
	result := make([]byte, len(source))
	copy(result, source)
	return result
}

func mustSub(source fs.FS, directory string) fs.FS {
	result, err := fs.Sub(source, directory)
	if err != nil {
		panic(err)
	}
	return result
}
