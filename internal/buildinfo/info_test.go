package buildinfo

import (
	"strings"
	"testing"

	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestCurrentIncludesContractPins(t *testing.T) {
	t.Parallel()

	info := Current()
	if info.Binary != BinaryName || info.Version == "" || info.SourceRevision == "" {
		t.Fatalf("incomplete binary metadata: %#v", info)
	}
	if !strings.HasPrefix(info.GoVersion, "go1.") {
		t.Fatalf("unexpected Go version %q", info.GoVersion)
	}
	if info.Schema.Version != ledgerschema.Version ||
		info.Schema.Commit != ledgerschema.Commit ||
		info.Schema.SHA256 != ledgerschema.SchemaSHA256 {
		t.Fatalf("unexpected schema metadata: %#v", info.Schema)
	}
	if info.MCPProtocol != MCPProtocolVersion {
		t.Fatalf("unexpected MCP protocol %q", info.MCPProtocol)
	}
	if info.Timestamp.Protocol != "rfc3161" || info.Timestamp.HashAlgorithm != "sha256" || !info.Timestamp.Experimental {
		t.Fatalf("unexpected timestamp support: %#v", info.Timestamp)
	}
}
