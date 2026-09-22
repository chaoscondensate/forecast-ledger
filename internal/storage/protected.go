package storage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

// CreateProtectedFile exclusively creates and durably writes a secret file.
// Platform implementations establish owner-only protection before bytes are
// written and fail closed when that protection cannot be verified.
func CreateProtectedFile(path string, data []byte) error {
	return createProtectedFile(path, data)
}

// CheckProtectedFile verifies the native owner-only protection contract for an
// existing regular file without changing its permissions or ACL.
func CheckProtectedFile(path string) error {
	return CheckProtectedFileAs(path, "protected key file")
}

// CheckProtectedFileAs checks native protection while using a bounded purpose
// label in the returned application error.
func CheckProtectedFileAs(path, label string) error {
	label = filePurpose(label)
	err := checkProtectedFile(path)
	if err == nil {
		return nil
	}
	var applicationErr *app.Error
	if !errors.As(err, &applicationErr) {
		return err
	}
	message := strings.ReplaceAll(applicationErr.Message, "protected key file", label)
	details := make(map[string]any, len(applicationErr.Details))
	for key, value := range applicationErr.Details {
		details[key] = value
	}
	return app.WithDetails(app.NewError(applicationErr.Code, message, applicationErr.Cause), details)
}

// ReadProtectedFile validates owner-only native protection and reads one
// bounded regular file without accepting a link swap between inspection and
// the opened file handle.
func ReadProtectedFile(path string, maxBytes int64, label string) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	label = filePurpose(label)
	resolved, err := ResolveExistingFilePath(path, label)
	if err != nil {
		return nil, err
	}
	if err := CheckProtectedFileAs(resolved, label); err != nil {
		return nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, app.NewError(app.CodeIO, label+" cannot be opened", err)
	}
	defer file.Close()
	handleInfo, err := file.Stat()
	if err != nil {
		return nil, app.NewError(app.CodeIO, label+" handle cannot be inspected", err)
	}
	pathInfo, err := os.Lstat(resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, app.NewError(app.CodeConflict, label+" changed while it was opened", err)
		}
		return nil, app.NewError(app.CodeIO, label+" cannot be inspected", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !handleInfo.Mode().IsRegular() || !os.SameFile(handleInfo, pathInfo) {
		return nil, app.NewError(app.CodeConflict, label+" changed or is not a regular file", nil)
	}
	if handleInfo.Size() > maxBytes {
		return nil, app.NewError(app.CodeInvalidData, label+" exceeds its size limit", nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, app.NewError(app.CodeIO, label+" cannot be read", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, app.NewError(app.CodeInvalidData, label+" exceeds its size limit", nil)
	}
	return data, nil
}
