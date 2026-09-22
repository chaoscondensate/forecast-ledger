package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var ErrUnsafePath = errors.New("path is not safe")

// ResolveLedgerPath resolves an explicitly supplied ledger filename. Existing
// ledgers must be regular files and may not themselves be symlinks or reparse
// points. Ancestor symlinks are canonicalized before an artifact root is made.
func ResolveLedgerPath(input string, mustExist bool) (string, error) {
	return resolveFilePath(input, mustExist, "ledger file")
}

// ResolveExistingFilePath resolves an explicitly supplied existing regular
// file and uses a bounded, non-secret purpose label in safe diagnostics.
func ResolveExistingFilePath(input, label string) (string, error) {
	return resolveFilePath(input, true, filePurpose(label))
}

func resolveFilePath(input string, mustExist bool, label string) (string, error) {
	label = filePurpose(label)
	if strings.TrimSpace(input) == "" {
		if label == "ledger file" {
			return "", app.NewError(app.CodeUsage, "--file is required", ErrUnsafePath)
		}
		return "", app.NewError(app.CodeUsage, label+" path is required", ErrUnsafePath)
	}
	if strings.IndexByte(input, 0) >= 0 {
		return "", unsafePathError(label + " path contains a NUL byte")
	}
	if isWindowsDevicePath(input) || hasWindowsReservedComponent(input) {
		return "", unsafePathError("Windows device paths and reserved device names are not allowed")
	}
	if runtime.GOOS != "windows" && looksLikeWindowsAbsolute(input) {
		return "", unsafePathError("Windows drive and UNC paths are not valid on this platform")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", app.NewError(app.CodeUsage, label+" path is not valid", err)
	}
	abs = filepath.Clean(abs)
	info, statErr := os.Lstat(abs)
	if statErr == nil {
		if isLinkOrReparse(info) {
			return "", unsafePathError(label + " path must not be a symlink or junction")
		}
		if !info.Mode().IsRegular() {
			return "", unsafePathError(label + " path must be a regular file")
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", app.NewError(app.CodeIO, label+" path cannot be resolved", err)
		}
		return filepath.Clean(resolved), nil
	}
	if !errors.Is(statErr, fs.ErrNotExist) {
		return "", app.NewError(app.CodeIO, label+" path cannot be inspected", statErr)
	}
	if mustExist {
		return "", app.NewError(app.CodeNotFound, label+" does not exist", statErr)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", app.NewError(app.CodeNotFound, label+" parent directory does not exist", err)
		}
		return "", app.NewError(app.CodeIO, label+" parent directory cannot be resolved", err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func filePurpose(label string) string {
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 64 {
		return "file"
	}
	for _, character := range label {
		if character == ' ' || character == '-' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' {
			continue
		}
		return "file"
	}
	return label
}

// ResolveNewFilePath resolves an explicit destination whose parent already
// exists. Every existing destination type is a conflict; ancestor links are
// canonicalized and the final name is checked for portable case collisions.
func ResolveNewFilePath(input, label string) (string, error) {
	if strings.TrimSpace(label) == "" {
		label = "output"
	}
	if strings.TrimSpace(input) == "" || strings.IndexByte(input, 0) >= 0 {
		return "", app.NewError(app.CodeUsage, label+" path is required and must not contain NUL", ErrUnsafePath)
	}
	if isWindowsDevicePath(input) || hasWindowsReservedComponent(input) {
		return "", unsafePathError("Windows device paths and reserved device names are not allowed")
	}
	if runtime.GOOS != "windows" && looksLikeWindowsAbsolute(input) {
		return "", unsafePathError("Windows drive and UNC paths are not valid on this platform")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", app.NewError(app.CodeUsage, label+" path is not valid", err)
	}
	abs = filepath.Clean(abs)
	if info, statErr := os.Lstat(abs); statErr == nil {
		return "", app.WithDetails(app.NewError(app.CodeConflict, label+" already exists", nil), map[string]any{"kind": info.Mode().Type().String()})
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return "", app.NewError(app.CodeIO, label+" path cannot be inspected", statErr)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", app.NewError(app.CodeNotFound, label+" parent directory does not exist", err)
		}
		return "", app.NewError(app.CodeIO, label+" parent directory cannot be resolved", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", app.NewError(app.CodeUsage, label+" parent is not a directory", err)
	}
	if collision, err := caseFoldSibling(parent, filepath.Base(abs)); err != nil {
		return "", err
	} else if collision != "" {
		return "", app.WithDetails(app.NewError(app.CodeConflict, label+" collides with an existing entry after case folding", nil), map[string]any{"existing_name": collision})
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

type PathResolver struct {
	root     string
	rootInfo fs.FileInfo
}

func NewPathResolver(root string) (*PathResolver, error) {
	if strings.TrimSpace(root) == "" {
		return nil, unsafePathError("artifact root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, unsafePathError("artifact root is not valid")
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, app.NewError(app.CodeNotFound, "artifact root does not exist", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, app.NewError(app.CodeIO, "artifact root cannot be inspected", err)
	}
	if !info.IsDir() {
		return nil, unsafePathError("artifact root must be a directory")
	}
	identity, err := statPathIdentity(canonical)
	if err != nil {
		return nil, app.NewError(app.CodeIO, "artifact root identity cannot be read", err)
	}
	return &PathResolver{root: filepath.Clean(canonical), rootInfo: identity}, nil
}

func (r *PathResolver) Root() string { return r.root }

// Resolve confines a schema relativePath under Root and rejects existing
// symlinks, junctions, and other reparse points in every descendant component.
func (r *PathResolver) Resolve(relative string, mustExist bool) (string, error) {
	return r.ResolveLabeled(relative, mustExist, "artifact")
}

// ResolveLabeled applies Resolve's confinement checks and uses label only for
// safe diagnostics about the requested artifact purpose.
func (r *PathResolver) ResolveLabeled(relative string, mustExist bool, label string) (string, error) {
	label = filePurpose(label)
	if r == nil || r.root == "" || r.rootInfo == nil {
		return "", app.NewError(app.CodeInternal, "path resolver is not initialized", nil)
	}
	current, err := statPathIdentity(r.root)
	if err != nil {
		return "", app.NewError(app.CodeConflict, "configured root changed after startup", err)
	}
	if !current.IsDir() || !os.SameFile(r.rootInfo, current) {
		return "", app.NewError(app.CodeConflict, "configured root identity changed after startup", nil)
	}
	if err := ValidateRelativePath(relative); err != nil {
		return "", err
	}
	candidate := filepath.Join(r.root, filepath.FromSlash(relative))
	contained, err := pathContained(r.root, candidate)
	if err != nil || !contained {
		return "", unsafePathError("path escapes the allowed root")
	}
	if err := rejectDescendantLinks(r.root, candidate, mustExist, label); err != nil {
		return "", err
	}
	return candidate, nil
}

// ResolveForCreate confines a future file while allowing any missing suffix of
// parent directories. Existing path components still receive the same
// identity, link, reparse-point, and portable-collision checks as Resolve.
func (r *PathResolver) ResolveForCreate(relative string) (string, error) {
	if r == nil || r.root == "" || r.rootInfo == nil {
		return "", app.NewError(app.CodeInternal, "path resolver is not initialized", nil)
	}
	current, err := statPathIdentity(r.root)
	if err != nil || !current.IsDir() || !os.SameFile(r.rootInfo, current) {
		return "", app.NewError(app.CodeConflict, "configured root changed after startup", err)
	}
	if err := ValidateRelativePath(relative); err != nil {
		return "", err
	}
	candidate := filepath.Join(r.root, filepath.FromSlash(relative))
	contained, err := pathContained(r.root, candidate)
	if err != nil || !contained {
		return "", unsafePathError("path escapes the allowed root")
	}
	if err := rejectDescendantLinksForCreate(r.root, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

// statPathIdentity derives FileInfo from an open handle instead of a pathname.
// On Windows, os.Stat may defer loading the file ID until os.SameFile runs; if
// the path was replaced in between, both FileInfo values can then describe the
// replacement. File.Stat snapshots the identity from the opened object.
func statPathIdentity(path string) (fs.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func ValidateRelativePath(path string) error {
	if path == "" || path == "." || strings.IndexByte(path, 0) >= 0 {
		return unsafePathError("relative path is empty or invalid")
	}
	if strings.Contains(path, "\\") {
		return unsafePathError("relative paths must use forward slashes")
	}
	if strings.Contains(path, ":") || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || looksLikeWindowsAbsolute(path) {
		return unsafePathError("absolute, drive, UNC, and alternate-stream paths are not allowed")
	}
	if !fs.ValidPath(path) {
		return unsafePathError("relative path contains an empty, dot, or parent segment")
	}
	if isWindowsDevicePath(path) || hasWindowsReservedComponent(path) {
		return unsafePathError("Windows device paths and reserved device names are not allowed")
	}
	return nil
}

// DetectPortablePathCollisions rejects paths that would alias after Unicode
// normalization and case folding on a supported case-insensitive filesystem.
func DetectPortablePathCollisions(paths []string) error {
	type originalPath struct{ value string }
	seen := make(map[string]originalPath, len(paths))
	folder := cases.Fold()
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	for _, path := range sorted {
		if err := ValidateRelativePath(path); err != nil {
			return err
		}
		key := folder.String(norm.NFC.String(path))
		if first, exists := seen[key]; exists && first.value != path {
			return app.WithDetails(app.NewError(app.CodeConflict, "portable paths collide after case folding", nil), map[string]any{
				"first": first.value, "second": path,
			})
		}
		seen[key] = originalPath{value: path}
	}
	return nil
}

func pathContained(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative), nil
}

func rejectDescendantLinks(root, candidate string, mustExist bool, label string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return unsafePathError("path cannot be made relative to its root")
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		collision, collisionErr := caseFoldSibling(filepath.Dir(current), part)
		if collisionErr != nil {
			return collisionErr
		}
		if collision != "" {
			return app.WithDetails(app.NewError(app.CodeConflict, "path collides with an existing entry after case folding", nil), map[string]any{"existing_name": collision})
		}
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				if mustExist || index < len(parts)-1 {
					return app.NewError(app.CodeNotFound, label+" does not exist", err)
				}
				return nil
			}
			return app.NewError(app.CodeIO, "path component cannot be inspected", err)
		}
		if isLinkOrReparse(info) {
			return unsafePathError("symlinks, junctions, and reparse points are not allowed inside the artifact root")
		}
		if info.Name() != part && strings.EqualFold(norm.NFC.String(info.Name()), norm.NFC.String(part)) {
			return app.WithDetails(app.NewError(app.CodeConflict, "path collides with an existing entry after case folding", nil), map[string]any{"existing_name": info.Name()})
		}
		if index < len(parts)-1 && !info.IsDir() {
			return unsafePathError("intermediate path component is not a directory")
		}
	}
	return nil
}

func rejectDescendantLinksForCreate(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return unsafePathError("path cannot be made relative to its root")
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		collision, collisionErr := caseFoldSibling(filepath.Dir(current), part)
		if collisionErr != nil {
			return collisionErr
		}
		if collision != "" {
			return app.WithDetails(app.NewError(app.CodeConflict, "path collides with an existing entry after case folding", nil), map[string]any{"existing_name": collision})
		}
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return app.NewError(app.CodeIO, "path component cannot be inspected", statErr)
		}
		if isLinkOrReparse(info) {
			return unsafePathError("symlinks, junctions, and reparse points are not allowed inside the artifact root")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return unsafePathError("intermediate path component is not a directory")
		}
	}
	return nil
}

func caseFoldSibling(directory, requested string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", app.NewError(app.CodeIO, "path parent cannot be read for collision checks", err)
	}
	for _, entry := range entries {
		if entry.Name() != requested && strings.EqualFold(norm.NFC.String(entry.Name()), norm.NFC.String(requested)) {
			return entry.Name(), nil
		}
	}
	return "", nil
}

func looksLikeWindowsAbsolute(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' ||
		strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `//`)
}

func isWindowsDevicePath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	return strings.HasPrefix(normalized, `\\?\`) || strings.HasPrefix(normalized, `\\.\`) ||
		strings.HasPrefix(normalized, `\??\`) || strings.Contains(normalized, `\globalroot\`)
}

func hasWindowsReservedComponent(path string) bool {
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, component := range strings.Split(normalized, "/") {
		component = strings.TrimRight(component, " .")
		base := component
		if dot := strings.IndexByte(base, '.'); dot >= 0 {
			base = base[:dot]
		}
		upper := strings.ToUpper(base)
		switch upper {
		case "CON", "PRN", "AUX", "NUL", "CLOCK$":
			return true
		}
		if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
			return true
		}
	}
	return false
}

func unsafePathError(message string) error {
	return app.NewError(app.CodeUsage, message, ErrUnsafePath)
}

func (r *PathResolver) String() string {
	if r == nil {
		return "<nil>"
	}
	return fmt.Sprintf("PathResolver(%s)", r.root)
}
