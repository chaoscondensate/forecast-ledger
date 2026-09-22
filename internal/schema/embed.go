// Package schema embeds the exact supported Forecast Ledger contract and its
// upstream conformance material. Runtime code must never fetch a schema.
package schema

import (
	"embed"
	"io/fs"
)

const (
	Version                = "2.0.1"
	Commit                 = "55b1431d379128e1d75b9c30a3874398cea9ff0f"
	AnnotatedTagObject     = "ac69f718de92f2c087b44b1b02d325ba37945db6"
	SchemaSHA256           = "5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b"
	ReleaseArchiveSHA256   = "bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41"
	ReleaseChecksumsSHA256 = "fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6"
	ForecastSealProtocol   = "forecast-seal/v2"
	ForecastTargetProfile  = "forecast-envelope/v2"
)

//go:embed upstream/forecast-ledger/v2.0.1/schema/forecast-ledger.schema.json
var contract []byte

//go:embed upstream/forecast-ledger/v2.0.1/LICENSE
var license []byte

//go:embed upstream/forecast-ledger/v2.0.1
var upstream embed.FS

// Contract returns an independent copy of the active v2 schema bytes.
func Contract() []byte {
	return clone(contract)
}

// License returns an independent copy of the exact upstream v2 license bytes.
func License() []byte {
	return clone(license)
}

// Conformance returns the complete retained v2.0.1 upstream release subtree.
// Paths retain their upstream layout, for example examples/valid and
// tests/vectors.
func Conformance() fs.FS {
	return mustSub(upstream, "upstream/forecast-ledger/v2.0.1")
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
