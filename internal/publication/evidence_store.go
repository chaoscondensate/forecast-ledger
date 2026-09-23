package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

type ManagedStoreLimits struct {
	MaxDepth      int
	MaxEntries    int
	MaxFileBytes  int64
	MaxTotalBytes int64
}

func DefaultManagedStoreLimits() ManagedStoreLimits {
	return ManagedStoreLimits{MaxDepth: 16, MaxEntries: MaxEvidenceEntries, MaxFileBytes: 16 << 20, MaxTotalBytes: 256 << 20}
}

type ManagedArtifact struct {
	Path   string
	Size   int64
	SHA256 string
	Bytes  []byte
}

type ManagedStore struct {
	Artifacts  map[string]ManagedArtifact
	IndexBytes []byte
	TotalBytes int64
}

func InspectManagedStore(root string, limits ManagedStoreLimits) (ManagedStore, error) {
	limits = normalizeManagedStoreLimits(limits)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return ManagedStore{}, err
	}
	result := ManagedStore{Artifacts: make(map[string]ManagedArtifact)}
	paths := make([]string, 0)
	for _, namespace := range []string{"proofs", "trust"} {
		absolute := filepath.Join(resolver.Root(), namespace)
		info, statErr := os.Lstat(absolute)
		if errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return ManagedStore{}, fmt.Errorf("inspect managed namespace: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ManagedStore{}, errors.New("managed namespace is not a regular directory")
		}
		err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(resolver.Root(), path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if relative == namespace {
				return nil
			}
			if len(relative) > MaxEvidencePathBytes || strings.Count(relative, "/")+1 > limits.MaxDepth {
				return errors.New("managed evidence path exceeds its limit")
			}
			if err := storage.ValidateRelativePath(relative); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("managed evidence must not contain symlinks")
			}
			if entry.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return errors.New("managed evidence contains a non-regular file")
			}
			if len(paths) >= limits.MaxEntries || info.Size() > limits.MaxFileBytes || result.TotalBytes > limits.MaxTotalBytes-info.Size() {
				return errors.New("managed evidence exceeds its size or entry budget")
			}
			resolved, err := resolver.Resolve(relative, true)
			if err != nil {
				return err
			}
			data, err := readManagedRegularFile(resolved, info, limits.MaxFileBytes)
			if err != nil {
				return err
			}
			result.TotalBytes += int64(len(data))
			paths = append(paths, relative)
			if relative == EvidenceIndexPath {
				result.IndexBytes = data
				return nil
			}
			digest := sha256.Sum256(data)
			result.Artifacts[relative] = ManagedArtifact{Path: relative, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), Bytes: data}
			return nil
		})
		if err != nil {
			return ManagedStore{}, err
		}
	}
	if err := storage.DetectPortablePathCollisions(paths); err != nil {
		return ManagedStore{}, err
	}
	return result, nil
}

func readManagedRegularFile(path string, expected fs.FileInfo, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	handleInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !handleInfo.Mode().IsRegular() || !os.SameFile(handleInfo, pathInfo) || !os.SameFile(expected, handleInfo) {
		return nil, errors.New("managed evidence changed while it was opened")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("managed evidence file exceeds its size limit")
	}
	return data, nil
}

func normalizeManagedStoreLimits(limits ManagedStoreLimits) ManagedStoreLimits {
	defaults := DefaultManagedStoreLimits()
	if limits.MaxDepth <= 0 {
		limits.MaxDepth = defaults.MaxDepth
	}
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = defaults.MaxEntries
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = defaults.MaxFileBytes
	}
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	return limits
}
