// Package schema embeds the exact supported Forecast Ledger contract and its
// upstream conformance material. Runtime code must never fetch a schema.
package schema

import (
	"embed"
	"io/fs"
)

const (
	Version                = "2.1.0"
	Commit                 = "d6ceebe4d42eac9f9e6d6df18167dc0dbf253bd4"
	AnnotatedTagObject     = "6d74d483f7cb177470ba77f4fcb4063fe04f4003"
	SchemaSHA256           = "cb4ba1d3c71c9fd824fb49d2e08defa01a3fa69bd70edd8139ce808d31cd5107"
	ReleaseArchiveSHA256   = "f417a40f1ddd0ef8448a983db8c6836241320cf914987554f077e90d6222ec17"
	ReleaseChecksumsSHA256 = "6b7437ed7bd1834792039d8a0b360f72e70710eb9a707336ba016fad0874799b"
	ForecastSealProtocol   = "forecast-seal/v2"
	ForecastTargetProfile  = "forecast-envelope/v2"
	LifecycleTargetProfile = "forecast-lifecycle/v1"
)

//go:embed upstream/forecast-ledger/v2.1.0/schema/forecast-ledger.schema.json
var contract []byte

//go:embed upstream/forecast-ledger/v2.1.0/LICENSE
var license []byte

//go:embed upstream/forecast-ledger/v2.1.0
var upstream embed.FS

// Contract returns an independent copy of the active v2 schema bytes.
func Contract() []byte {
	return clone(contract)
}

// License returns an independent copy of the exact upstream v2 license bytes.
func License() []byte {
	return clone(license)
}

// Conformance returns the complete retained v2.1.0 upstream release subtree.
// Paths retain their upstream layout, for example examples/valid and
// tests/vectors.
func Conformance() fs.FS {
	return mustSub(upstream, "upstream/forecast-ledger/v2.1.0")
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
