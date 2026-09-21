// Package buildinfo exposes reproducible build and contract metadata.
package buildinfo

import (
	"runtime"

	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

const (
	BinaryName         = "forecast-ledger"
	MCPProtocolVersion = "2026-07-28"
)

var (
	version        = "dev"
	sourceRevision = "unknown"
)

// Schema identifies the exact embedded Forecast Ledger contract.
type Schema struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	SHA256  string `json:"sha256"`
}

type TimestampSupport struct {
	Protocol      string   `json:"protocol"`
	HashAlgorithm string   `json:"hash_algorithm"`
	Experimental  bool     `json:"experimental"`
	DefaultMode   string   `json:"default_mode"`
	Providers     []string `json:"providers"`
}

// Info is the stable machine-readable version result.
type Info struct {
	Binary         string           `json:"binary"`
	Version        string           `json:"version"`
	SourceRevision string           `json:"source_revision"`
	GoVersion      string           `json:"go_version"`
	Schema         Schema           `json:"schema"`
	MCPProtocol    string           `json:"mcp_protocol"`
	Timestamp      TimestampSupport `json:"timestamp"`
}

// Current returns metadata for the running binary.
func Current() Info {
	return Info{
		Binary:         BinaryName,
		Version:        version,
		SourceRevision: sourceRevision,
		GoVersion:      runtime.Version(),
		Schema: Schema{
			Version: ledgerschema.Version,
			Commit:  ledgerschema.Commit,
			SHA256:  ledgerschema.SchemaSHA256,
		},
		MCPProtocol: MCPProtocolVersion,
		Timestamp:   TimestampSupport{Protocol: "rfc3161", HashAlgorithm: "sha256", Experimental: true, DefaultMode: "auto", Providers: []string{"freetsa"}},
	}
}
