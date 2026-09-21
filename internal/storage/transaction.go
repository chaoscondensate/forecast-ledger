package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
)

type TransactionStage string

const (
	StageTempCreated     TransactionStage = "temp_created"
	StageTempPermissions TransactionStage = "temp_permissions"
	StageTempWritten     TransactionStage = "temp_written"
	StageTempSynced      TransactionStage = "temp_synced"
	StageJournalCreated  TransactionStage = "journal_created"
	StageJournalWritten  TransactionStage = "journal_written"
	StageJournalSynced   TransactionStage = "journal_synced"
	StageBeforeReplace   TransactionStage = "before_replace"
	StageReplaced        TransactionStage = "replaced"
	StageDirectorySynced TransactionStage = "directory_synced"
	StageBeforeCleanup   TransactionStage = "before_cleanup"
	StageJournalRemoved  TransactionStage = "journal_removed"
	StageCleanupSynced   TransactionStage = "cleanup_synced"
)

type ValidateDocumentFunc func(*document.Document) error
type MutateDocumentFunc func(*document.Document) ([]byte, error)

type TransactionOptions struct {
	LockWait time.Duration
	Validate ValidateDocumentFunc
	Mutate   MutateDocumentFunc
	// Fault is a deterministic crash-injection seam for storage tests.
	Fault func(TransactionStage) error
}

type recoveryJournal struct {
	Version        int    `json:"version"`
	LedgerBase     string `json:"ledger_base"`
	TempBase       string `json:"temp_base"`
	OriginalSHA256 string `json:"original_sha256"`
	ExpectedSHA256 string `json:"expected_sha256"`
	CreatedAt      string `json:"created_at"`
}

func JournalPath(ledgerPath string) string {
	return filepath.Join(filepath.Dir(ledgerPath), "."+filepath.Base(ledgerPath)+".forecast-ledger-journal.json")
}

// UpdateLedger executes lock, parse, validate, mutate, reparse, revalidate,
// durable sibling write, journal, and safe replacement in that order.
func UpdateLedger(ctx context.Context, ledgerPath string, options TransactionOptions) error {
	resolved, err := ResolveLedgerPath(ledgerPath, false)
	if err != nil {
		return err
	}
	// Reject an unsupported or malformed ledger before creating a persistent
	// lock file. When a recovery journal exists, recovery must run under the
	// lock first because the visible ledger may be the interrupted old state.
	if options.Validate != nil {
		if _, journalErr := os.Stat(JournalPath(resolved)); errors.Is(journalErr, fs.ErrNotExist) {
			original, readErr := os.ReadFile(resolved)
			if readErr != nil {
				if errors.Is(readErr, fs.ErrNotExist) {
					return app.NewError(app.CodeNotFound, "ledger file does not exist", readErr)
				}
				return app.NewError(app.CodeIO, "ledger file cannot be read", readErr)
			}
			parsed, parseErr := parseDocument(original, detectFormat(resolved, original))
			if parseErr != nil {
				return app.NewError(app.CodeInvalidData, "ledger cannot be parsed", parseErr)
			}
			if validateErr := options.Validate(parsed); validateErr != nil {
				return validationFailure("ledger failed pre-mutation validation", validateErr)
			}
		}
	}
	lock, err := AcquireLedgerLock(ctx, resolved, options.LockWait)
	if err != nil {
		return err
	}
	defer lock.Release()
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, err := recoverLedgerLocked(ctx, resolved, options.Validate); err != nil {
		return err
	}

	original, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return app.NewError(app.CodeNotFound, "ledger file does not exist", err)
		}
		return app.NewError(app.CodeIO, "ledger file cannot be read", err)
	}
	format := detectFormat(resolved, original)
	parsed, err := parseDocument(original, format)
	if err != nil {
		return app.NewError(app.CodeInvalidData, "ledger cannot be parsed", err)
	}
	if options.Validate != nil {
		if err := options.Validate(parsed); err != nil {
			return validationFailure("ledger failed pre-mutation validation", err)
		}
	}
	if options.Mutate == nil {
		return app.NewError(app.CodeInternal, "transaction has no mutation", nil)
	}
	updated, err := options.Mutate(parsed)
	if err != nil {
		return err
	}
	if bytes.Equal(updated, original) {
		return nil
	}
	post, err := parseDocument(updated, format)
	if err != nil {
		return app.NewError(app.CodeInvalidData, "mutation produced an invalid document", err)
	}
	if options.Validate != nil {
		if err := options.Validate(post); err != nil {
			return validationFailure("ledger failed post-mutation validation", err)
		}
	}
	if err := contextError(ctx); err != nil {
		return err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return app.NewError(app.CodeIO, "ledger permissions cannot be read", err)
	}
	tempPath, err := writeSiblingTemp(resolved, updated, info.Mode().Perm(), options)
	if err != nil {
		return err
	}
	journalWritten := false
	cleanupTemp := true
	defer func() {
		if cleanupTemp && !journalWritten {
			_ = os.Remove(tempPath)
		}
	}()
	journal := recoveryJournal{
		Version:        1,
		LedgerBase:     filepath.Base(resolved),
		TempBase:       filepath.Base(tempPath),
		OriginalSHA256: sha256Hex(original),
		ExpectedSHA256: sha256Hex(updated),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeJournalExclusive(JournalPath(resolved), journal, options); err != nil {
		return err
	}
	journalWritten = true
	cleanupTemp = false
	if err := injectFault(options, StageJournalSynced); err != nil {
		return err
	}
	if err := injectFault(options, StageBeforeReplace); err != nil {
		return err
	}
	if err := safeReplace(tempPath, resolved); err != nil {
		return app.NewError(app.CodeIO, "ledger replacement failed; recovery journal was retained", err)
	}
	if err := injectFault(options, StageReplaced); err != nil {
		return err
	}
	if err := syncParentDirectory(filepath.Dir(resolved)); err != nil {
		return app.NewError(app.CodeIO, "ledger directory flush failed; recovery journal was retained", err)
	}
	if err := injectFault(options, StageDirectorySynced); err != nil {
		return err
	}
	if err := injectFault(options, StageBeforeCleanup); err != nil {
		return err
	}
	if err := os.Remove(JournalPath(resolved)); err != nil {
		return app.NewError(app.CodeIO, "ledger was replaced but recovery journal could not be removed", err)
	}
	if err := injectFault(options, StageJournalRemoved); err != nil {
		return err
	}
	if err := syncParentDirectory(filepath.Dir(resolved)); err != nil {
		return app.NewError(app.CodeIO, "recovery journal removal could not be flushed", err)
	}
	if err := injectFault(options, StageCleanupSynced); err != nil {
		return err
	}
	return nil
}

func validationFailure(message string, cause error) error {
	if app.ErrorCodeOf(cause) == app.CodeUnsupportedSchemaVersion {
		return cause
	}
	wrapped := app.NewError(app.CodeInvalidData, message, cause)
	var applicationErr *app.Error
	if errors.As(cause, &applicationErr) && len(applicationErr.Details) > 0 {
		return app.WithDetails(wrapped, applicationErr.Details)
	}
	return wrapped
}

// RecoverLedger completes a journaled replacement only when the original or
// already-replaced target digest and the synced temp digest match the journal.
func RecoverLedger(ctx context.Context, ledgerPath string, lockWait time.Duration, validate ValidateDocumentFunc) error {
	resolved, err := ResolveLedgerPath(ledgerPath, false)
	if err != nil {
		return err
	}
	lock, err := AcquireLedgerLock(ctx, resolved, lockWait)
	if err != nil {
		return err
	}
	defer lock.Release()
	recovered, err := recoverLedgerLocked(ctx, resolved, validate)
	if err != nil {
		return err
	}
	if !recovered {
		return app.NewError(app.CodeNotFound, "no recovery journal exists", fs.ErrNotExist)
	}
	return nil
}

// recoverLedgerLocked completes or cleans one journal while the caller holds
// the ledger writer lock. It returns false when no journal exists.
func recoverLedgerLocked(ctx context.Context, resolved string, validate ValidateDocumentFunc) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	journalPath := JournalPath(resolved)
	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, app.NewError(app.CodeIO, "recovery journal cannot be read", err)
	}
	journal, err := decodeRecoveryJournal(journalBytes, resolved)
	if err != nil {
		return false, err
	}
	current, err := os.ReadFile(resolved)
	targetMissing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !targetMissing {
		return false, app.NewError(app.CodeIO, "ledger cannot be read during recovery", err)
	}
	currentDigest := sha256Hex(current)
	tempPath := filepath.Join(filepath.Dir(resolved), journal.TempBase)
	if !targetMissing && currentDigest == journal.ExpectedSHA256 {
		if err := validateRecoveryDocument(resolved, current, validate, "recovered ledger"); err != nil {
			return false, err
		}
		if err := removeMatchingRecoveryTemp(tempPath, journal.ExpectedSHA256); err != nil {
			return false, err
		}
		if err := contextError(ctx); err != nil {
			return false, err
		}
		return true, removeJournalAndSync(journalPath, filepath.Dir(resolved))
	}
	if !targetMissing && currentDigest != journal.OriginalSHA256 {
		return false, app.NewError(app.CodeConflict, "ledger changed after the recovery journal was created", nil)
	}
	temp, err := os.ReadFile(tempPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, app.NewError(app.CodeConflict, "recovery temporary file is missing", err)
		}
		return false, app.NewError(app.CodeIO, "recovery temporary file cannot be read", err)
	}
	if sha256Hex(temp) != journal.ExpectedSHA256 {
		return false, app.NewError(app.CodeConflict, "recovery temporary file digest does not match the journal", nil)
	}
	if err := validateRecoveryDocument(resolved, temp, validate, "recovery temporary file"); err != nil {
		return false, err
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	if err := safeReplace(tempPath, resolved); err != nil {
		return false, app.NewError(app.CodeIO, "recovery replacement failed", err)
	}
	if err := syncParentDirectory(filepath.Dir(resolved)); err != nil {
		return false, app.NewError(app.CodeIO, "recovery replacement could not be flushed", err)
	}
	if err := removeJournalAndSync(journalPath, filepath.Dir(resolved)); err != nil {
		return false, err
	}
	return true, nil
}

func decodeRecoveryJournal(data []byte, resolved string) (recoveryJournal, error) {
	var journal recoveryJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return recoveryJournal{}, app.NewError(app.CodeConflict, "recovery journal is invalid", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return recoveryJournal{}, app.NewError(app.CodeConflict, "recovery journal has trailing data", nil)
	} else if !errors.Is(err, io.EOF) {
		return recoveryJournal{}, app.NewError(app.CodeConflict, "recovery journal has trailing data", err)
	}
	expectedTempPrefix := "." + filepath.Base(resolved) + ".forecast-ledger-"
	validTemp := filepath.Base(journal.TempBase) == journal.TempBase && strings.HasPrefix(journal.TempBase, expectedTempPrefix) && strings.HasSuffix(journal.TempBase, ".tmp")
	_, originalErr := hex.DecodeString(journal.OriginalSHA256)
	_, expectedErr := hex.DecodeString(journal.ExpectedSHA256)
	_, createdErr := time.Parse(time.RFC3339Nano, journal.CreatedAt)
	if journal.Version != 1 || journal.LedgerBase != filepath.Base(resolved) || !validTemp || len(journal.OriginalSHA256) != sha256.Size*2 || len(journal.ExpectedSHA256) != sha256.Size*2 || originalErr != nil || expectedErr != nil || createdErr != nil || journal.OriginalSHA256 == journal.ExpectedSHA256 {
		return recoveryJournal{}, app.NewError(app.CodeConflict, "recovery journal is invalid", nil)
	}
	return journal, nil
}

func validateRecoveryDocument(resolved string, data []byte, validate ValidateDocumentFunc, label string) error {
	parsed, err := parseDocument(data, detectFormat(resolved, data))
	if err != nil {
		return app.NewError(app.CodeInvalidData, label+" cannot be parsed", err)
	}
	if validate != nil {
		if err := validate(parsed); err != nil {
			return validationFailure(label+" is not valid", err)
		}
	}
	return nil
}

func removeMatchingRecoveryTemp(path, expectedDigest string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return app.NewError(app.CodeIO, "recovery temporary file cannot be read", err)
	}
	if sha256Hex(data) != expectedDigest {
		return app.NewError(app.CodeConflict, "recovery temporary file digest does not match the completed ledger", nil)
	}
	if err := os.Remove(path); err != nil {
		return app.NewError(app.CodeIO, "recovery temporary file cannot be removed", err)
	}
	return nil
}

func writeSiblingTemp(ledgerPath string, data []byte, mode fs.FileMode, options TransactionOptions) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(ledgerPath), "."+filepath.Base(ledgerPath)+".forecast-ledger-*.tmp")
	if err != nil {
		return "", app.NewError(app.CodeIO, "temporary ledger file cannot be created", err)
	}
	path := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := injectFault(options, StageTempCreated); err != nil {
		return "", err
	}
	if err := temp.Chmod(mode); err != nil {
		return "", app.NewError(app.CodeIO, "temporary ledger permissions cannot be set", err)
	}
	if err := injectFault(options, StageTempPermissions); err != nil {
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		return "", app.NewError(app.CodeIO, "temporary ledger cannot be written", err)
	}
	if err := injectFault(options, StageTempWritten); err != nil {
		return "", err
	}
	if err := temp.Sync(); err != nil {
		return "", app.NewError(app.CodeIO, "temporary ledger cannot be flushed", err)
	}
	if err := injectFault(options, StageTempSynced); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", app.NewError(app.CodeIO, "temporary ledger cannot be closed", err)
	}
	ok = true
	return path, nil
}

func writeJournalExclusive(path string, journal recoveryJournal, options TransactionOptions) error {
	encoded, err := json.Marshal(journal)
	if err != nil {
		return app.NewError(app.CodeInternal, "recovery journal cannot be encoded", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return app.NewError(app.CodeConflict, "recovery journal already exists", err)
		}
		return app.NewError(app.CodeIO, "recovery journal cannot be created", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := injectFault(options, StageJournalCreated); err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return app.NewError(app.CodeIO, "recovery journal cannot be written", err)
	}
	if err := injectFault(options, StageJournalWritten); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return app.NewError(app.CodeIO, "recovery journal cannot be flushed", err)
	}
	if err := file.Close(); err != nil {
		return app.NewError(app.CodeIO, "recovery journal cannot be closed", err)
	}
	ok = true
	return nil
}

func parseDocument(data []byte, format document.Format) (*document.Document, error) {
	switch format {
	case document.FormatJSON:
		return document.ParseJSON(bytes.NewReader(data), document.DefaultLimits)
	case document.FormatYAML:
		return document.ParseYAML(bytes.NewReader(data), document.DefaultLimits)
	default:
		return nil, errors.New("unknown ledger format")
	}
}

func detectFormat(path string, data []byte) document.Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return document.FormatJSON
	case ".yaml", ".yml":
		return document.FormatYAML
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return document.FormatJSON
	}
	return document.FormatYAML
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func injectFault(options TransactionOptions, stage TransactionStage) error {
	if options.Fault == nil {
		return nil
	}
	if err := options.Fault(stage); err != nil {
		return app.NewError(app.CodeIO, "transaction stopped at an injected failure point", err)
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return app.NewError(app.CodeInterrupted, "operation was interrupted", ctx.Err())
}

func removeJournalAndSync(journalPath, directory string) error {
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return app.NewError(app.CodeIO, "recovery journal cannot be removed", err)
	}
	if err := syncParentDirectory(directory); err != nil {
		return app.NewError(app.CodeIO, "recovery journal removal cannot be flushed", err)
	}
	return nil
}

func (journal recoveryJournal) String() string {
	return fmt.Sprintf("recoveryJournal(v%d,%s)", journal.Version, journal.LedgerBase)
}
