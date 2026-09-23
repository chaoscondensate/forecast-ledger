// Package schema embeds the exact supported Forecast Ledger contract and its
// upstream conformance material. Runtime code must never fetch a schema.
package schema

import (
	"embed"
	"io/fs"
)

const (
	Version                = "2.2.0"
	SchemaID               = "https://raw.githubusercontent.com/chaoscondensate/schema/v2.2.0/schema/forecast-ledger.schema.json"
	Commit                 = "ae02de9ebca3eb2ae87c480596bf620bdcdced11"
	AnnotatedTagObject     = "8feb323ce895a6a197183c4eb857a55463648873"
	SchemaSHA256           = "6a048928f2573519fd25988ba23f0006b6f8d84eb2f7179d7ff8edd7affa4f9e"
	ReleaseArchiveSHA256   = "e8f92450e7e73eb559e762878dd4188156968cdad6328c03ea94d6b0c01ff419"
	ReleaseChecksumsSHA256 = "7e1d79e6d8bd4df20a5877ef51c17f149cec7619d49ddfa2fff82899034989a0"
	ForecastSealProtocol   = "forecast-seal/v3"
	ForecastTargetProfile  = "forecast-envelope/v3"
	LifecycleTargetProfile = "forecast-lifecycle/v2"
)

//go:embed upstream/forecast-ledger/v2.2.0/schema/forecast-ledger.schema.json
var contract []byte

//go:embed upstream/forecast-ledger/v2.2.0/LICENSE
var license []byte

//go:embed upstream/forecast-ledger/v2.2.0
var upstream embed.FS

// Contract returns an independent copy of the active v2 schema bytes.
func Contract() []byte {
	return clone(contract)
}

// License returns an independent copy of the exact upstream v2 license bytes.
func License() []byte {
	return clone(license)
}

// Conformance returns the complete retained v2.2.0 upstream release subtree.
// Paths retain their upstream layout, for example examples/valid and
// tests/vectors.
func Conformance() fs.FS {
	return mustSub(upstream, "upstream/forecast-ledger/v2.2.0")
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
