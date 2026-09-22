// Package service coordinates transport-neutral Forecast Ledger operations.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

type LoadedLedger struct {
	Path     string
	Document *document.Document
	Model    *ledger.Ledger
}

type LedgerStatus struct {
	LedgerID          ledger.Slug `json:"ledger_id"`
	SchemaVersion     string      `json:"schema_version"`
	Questions         int         `json:"questions"`
	Forecasts         int         `json:"forecasts"`
	PublicForecasts   int         `json:"public_forecasts"`
	SealedForecasts   int         `json:"sealed_forecasts"`
	RevealedForecasts int         `json:"revealed_forecasts"`
	Unanchored        int         `json:"unanchored"`
	Pending           int         `json:"pending"`
	Verified          int         `json:"verified"`
	Failed            int         `json:"failed"`
}

func LoadAndValidateLedger(ctx context.Context, filename string, stdin io.Reader) (*LoadedLedger, error) {
	return loadAndValidateLedger(ctx, filename, stdin, "", true)
}

// loadAndValidateLedgerForEvidence validates the ledger contract and semantic
// relationships without requiring declared evidence files to be present. The
// evidence operation then classifies missing, unreadable, or mismatched bytes
// in its own result layer instead of collapsing them into document validity.
func loadAndValidateLedgerForEvidence(ctx context.Context, filename string) (*LoadedLedger, error) {
	return loadAndValidateLedger(ctx, filename, nil, "", false)
}

// LoadAndValidateLedgerWithArtifactRoot is used for portable packages whose
// byte-exact ledger lives under ledger/ while its stable proofs/ paths are
// rooted at the package directory.
func LoadAndValidateLedgerWithArtifactRoot(ctx context.Context, filename, artifactRoot string) (*LoadedLedger, error) {
	return loadAndValidateLedger(ctx, filename, nil, artifactRoot, true)
}

func loadAndValidateLedger(ctx context.Context, filename string, stdin io.Reader, artifactRoot string, validateArtifacts bool) (*LoadedLedger, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, app.NewError(app.CodeInterrupted, "operation was interrupted", ctx.Err())
	}
	var parsed *document.Document
	var path string
	var artifacts fs.FS
	var err error
	if filename == "-" {
		if stdin == nil {
			return nil, app.NewError(app.CodeUsage, "stdin is not available", nil)
		}
		parsed, err = parseStdin(stdin)
	} else {
		resolved, resolveErr := storage.ResolveLedgerPath(filename, true)
		if resolveErr != nil {
			return nil, resolveErr
		}
		file, openErr := os.Open(resolved)
		if openErr != nil {
			return nil, app.NewError(app.CodeIO, "ledger file cannot be opened", openErr)
		}
		defer file.Close()
		path = resolved
		if !validateArtifacts {
			artifacts = nil
		} else if artifactRoot == "" {
			artifacts = os.DirFS(filepath.Dir(resolved))
		} else {
			resolver, resolverErr := storage.NewPathResolver(artifactRoot)
			if resolverErr != nil {
				return nil, resolverErr
			}
			artifacts = os.DirFS(resolver.Root())
		}
		var format document.Format
		switch strings.ToLower(filepath.Ext(resolved)) {
		case ".json":
			format = document.FormatJSON
		case ".yaml", ".yml":
			format = document.FormatYAML
		default:
			return nil, app.NewError(app.CodeUsage, "ledger filename must end in .json, .yaml, or .yml", nil)
		}
		parsed, err = parseLedgerReader(file, format)
	}
	if err != nil {
		applicationErr := app.NewError(app.CodeInvalidData, "ledger cannot be parsed", err)
		var parseErr *document.ParseError
		if errors.As(err, &parseErr) {
			return nil, app.WithDetails(applicationErr, map[string]any{"issues": []document.Diagnostic{parseErr.Diagnostic}})
		}
		return nil, applicationErr
	}
	if err := RequireSupportedSchemaVersion(parsed); err != nil {
		return nil, err
	}
	structural, err := validation.DefaultStructuralValidator()
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "embedded ledger schema cannot be loaded", err)
	}
	schemaIssues, err := structural.Validate(parsed.Root.Any())
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "ledger schema validation could not run", err)
	}
	if len(schemaIssues) > 0 {
		return nil, app.WithDetails(app.NewError(app.CodeInvalidData, "ledger does not match Forecast Ledger v2", nil), map[string]any{"issues": ledgerSchemaDiagnostics(parsed, schemaIssues)})
	}
	model, err := validation.DecodeLedger(parsed)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "validated ledger cannot be mapped to the domain model", err)
	}
	semanticIssues, err := validation.ValidateSemantics(model, artifacts)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "ledger semantic validation could not run", err)
	}
	if len(semanticIssues) > 0 {
		return nil, app.WithDetails(app.NewError(app.CodeInvalidData, "ledger has semantic validation errors", nil), map[string]any{"issues": ledgerSemanticDiagnostics(parsed, semanticIssues)})
	}
	return &LoadedLedger{Path: path, Document: parsed, Model: model}, nil
}

// RequireSupportedSchemaVersion is the first check after bounded parsing for
// every existing-ledger operation. It intentionally runs before full schema,
// domain, artifact, crypto, or network work.
func RequireSupportedSchemaVersion(parsed *document.Document) error {
	if parsed == nil || parsed.Root == nil || parsed.Root.Kind != document.ValueObject {
		return unsupportedSchemaVersionError("")
	}
	for _, member := range parsed.Root.Object {
		if member.Key != "schema_version" {
			continue
		}
		if member.Value == nil || member.Value.Kind != document.ValueString {
			return unsupportedSchemaVersionError("")
		}
		if member.Value.String != ledgerschema.Version {
			return unsupportedSchemaVersionError(member.Value.String)
		}
		return nil
	}
	return unsupportedSchemaVersionError("")
}

func unsupportedSchemaVersionError(found string) error {
	details := map[string]any{"supported_schema_version": ledgerschema.Version}
	if found != "" && len(found) <= 128 {
		details["declared_schema_version"] = found
	}
	return app.WithDetails(
		app.NewError(app.CodeUnsupportedSchemaVersion, "ledger schema version is not supported; only "+ledgerschema.Version+" is accepted", nil),
		details,
	)
}

func StatusForLedger(loaded *LoadedLedger) (LedgerStatus, error) {
	if loaded == nil || loaded.Model == nil {
		return LedgerStatus{}, errors.New("loaded ledger is nil")
	}
	status := LedgerStatus{
		LedgerID: loaded.Model.LedgerID, SchemaVersion: string(loaded.Model.SchemaVersion), Questions: len(loaded.Model.Questions),
	}
	for _, question := range loaded.Model.Questions {
		for _, forecast := range question.Forecasts {
			status.Forecasts++
			switch forecast.Visibility {
			case ledger.VisibilityPublic:
				status.PublicForecasts++
			case ledger.VisibilitySealed:
				status.SealedForecasts++
			case ledger.VisibilityRevealed:
				status.RevealedForecasts++
			}
			switch {
			case forecast.Integrity.Unanchored != nil:
				status.Unanchored++
			case forecast.Integrity.Pending != nil:
				status.Pending++
			case forecast.Integrity.Verified != nil:
				status.Verified++
			case forecast.Integrity.Failed != nil:
				status.Failed++
			}
		}
	}
	return status, nil
}

func parseStdin(reader io.Reader) (*document.Document, error) {
	limit := document.DefaultLimits.MaxBytes
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read ledger from stdin: %w", err)
	}
	if int64(len(raw)) > limit {
		// Let the normal parser return its stable size-limit diagnostic.
		return document.ParseYAML(bytes.NewReader(raw), document.DefaultLimits)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return document.ParseJSON(bytes.NewReader(raw), document.DefaultLimits)
	}
	return document.ParseYAML(bytes.NewReader(raw), document.DefaultLimits)
}

func parseLedgerReader(reader io.Reader, format document.Format) (*document.Document, error) {
	switch format {
	case document.FormatJSON:
		return document.ParseJSON(reader, document.DefaultLimits)
	case document.FormatYAML:
		return document.ParseYAML(reader, document.DefaultLimits)
	default:
		return nil, fmt.Errorf("unknown ledger format %q", format)
	}
}
