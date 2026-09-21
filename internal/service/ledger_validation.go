package service

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

// ValidateLedgerDocument performs the complete prospective validation used by
// every ledger transaction: exact version, embedded schema, domain decoding,
// and semantic/artifact checks.
func ValidateLedgerDocument(parsed *document.Document, artifacts fs.FS) error {
	if err := RequireSupportedSchemaVersion(parsed); err != nil {
		return err
	}
	structural, err := validation.DefaultStructuralValidator()
	if err != nil {
		return fmt.Errorf("load embedded ledger schema: %w", err)
	}
	issues, err := structural.Validate(parsed.Root.Any())
	if err != nil {
		return fmt.Errorf("run ledger schema validation: %w", err)
	}
	if len(issues) > 0 {
		return app.WithDetails(
			app.NewError(app.CodeInvalidData, "ledger does not match Forecast Ledger v2", nil),
			map[string]any{"issues": ledgerSchemaDiagnostics(parsed, issues)},
		)
	}
	model, err := validation.DecodeLedger(parsed)
	if err != nil {
		return fmt.Errorf("decode schema-valid ledger: %w", err)
	}
	semantic, err := validation.ValidateSemantics(model, artifacts)
	if err != nil {
		return fmt.Errorf("run ledger semantic validation: %w", err)
	}
	if len(semantic) > 0 {
		return app.WithDetails(
			app.NewError(app.CodeInvalidData, "ledger has semantic validation errors", nil),
			map[string]any{"issues": ledgerSemanticDiagnostics(parsed, semantic)},
		)
	}
	return nil
}

func validateProspectiveFileMutation(loaded *LoadedLedger, patches []document.PatchOperation) error {
	if loaded == nil || loaded.Document == nil || loaded.Path == "" {
		return app.NewError(app.CodeInternal, "prospective file validation has no ledger path", nil)
	}
	patched, err := document.ApplyPatch(loaded.Document, patches)
	if err != nil {
		return app.NewError(app.CodeInternal, "prospective file mutation cannot be applied", err)
	}
	var parsed *document.Document
	switch loaded.Document.Format {
	case document.FormatJSON:
		parsed, err = document.ParseJSON(bytes.NewReader(patched), document.DefaultLimits)
	case document.FormatYAML:
		parsed, err = document.ParseYAML(bytes.NewReader(patched), document.DefaultLimits)
	default:
		return app.NewError(app.CodeInternal, "prospective file format is not supported", nil)
	}
	if err != nil {
		return app.NewError(app.CodeInternal, "prospective file mutation cannot be parsed", err)
	}
	if err := ValidateLedgerDocument(parsed, os.DirFS(filepath.Dir(loaded.Path))); err != nil {
		return preserveValidationDetails(app.CodeInvalidData, "prospective ledger is not valid", err)
	}
	return nil
}

func ledgerSchemaDiagnostics(parsed *document.Document, issues []validation.SchemaIssue) []document.Diagnostic {
	result := make([]document.Diagnostic, 0, len(issues))
	for _, issue := range issues {
		result = append(result, document.Diagnostic{
			Code: issue.Code, Message: issue.Message, Location: ledgerIssueLocation(parsed, issue.InstanceLocation),
		})
	}
	return result
}

func ledgerSemanticDiagnostics(parsed *document.Document, issues []validation.SemanticIssue) []document.Diagnostic {
	result := make([]document.Diagnostic, 0, len(issues))
	for _, issue := range issues {
		result = append(result, document.Diagnostic{
			Code: issue.Code, Message: issue.Message, Location: ledgerIssueLocation(parsed, issue.Pointer),
		})
	}
	return result
}

func ledgerIssueLocation(parsed *document.Document, pointer string) document.SourceRef {
	if parsed != nil {
		if locations := parsed.Locations[pointer]; len(locations) > 0 {
			return locations[0]
		}
		for parent := pointer; parent != ""; {
			if index := strings.LastIndex(parent, "/"); index >= 0 {
				parent = parent[:index]
			} else {
				parent = ""
			}
			if locations := parsed.Locations[parent]; len(locations) > 0 {
				location := locations[0]
				location.Pointer = pointer
				return location
			}
		}
	}
	return document.SourceRef{Pointer: pointer}
}

func preserveValidationDetails(code app.ErrorCode, message string, cause error) error {
	wrapped := app.NewError(code, message, cause)
	var applicationErr *app.Error
	if errors.As(cause, &applicationErr) && len(applicationErr.Details) > 0 {
		return app.WithDetails(wrapped, applicationErr.Details)
	}
	return wrapped
}
