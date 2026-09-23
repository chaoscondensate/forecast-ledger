package storage

import (
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
	"sort"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

type ResourceKind string

const (
	ResourceLedger            ResourceKind = "ledger"
	ResourceTarget            ResourceKind = "target"
	ResourceEvidenceIndex     ResourceKind = "evidence_index"
	ResourceTimestampRequest  ResourceKind = "timestamp_request"
	ResourceTimestampResponse ResourceKind = "timestamp_response"
	ResourceTimestampTrust    ResourceKind = "timestamp_trust"
	ResourceKey               ResourceKind = "key"
	ResourcePackage           ResourceKind = "package"
)

type ResourceType string

const (
	ResourceFile      ResourceType = "file"
	ResourceDirectory ResourceType = "directory"
)

type ResourceRollback string

const (
	ResourceRollbackNone         ResourceRollback = "none"
	ResourceRollbackRemoveOwned  ResourceRollback = "remove_owned"
	ResourceRollbackRetainSecret ResourceRollback = "retain_secret"
)

type ResourceState string

const (
	ResourcePlanned   ResourceState = "planned"
	ResourceCreated   ResourceState = "created"
	ResourceReplaced  ResourceState = "replaced"
	ResourceCommitted ResourceState = "committed"
)

type ResourceEntry struct {
	Kind         ResourceKind     `json:"kind"`
	Type         ResourceType     `json:"type"`
	Path         string           `json:"path"`
	Owned        bool             `json:"owned"`
	Rollback     ResourceRollback `json:"rollback"`
	State        ResourceState    `json:"state"`
	BeforeSHA256 string           `json:"before_sha256,omitempty"`
	SHA256       string           `json:"after_sha256,omitempty"`
}

type resourceJournal struct {
	Schema    string          `json:"schema"`
	Operation string          `json:"operation"`
	CreatedAt string          `json:"created_at"`
	Resources []ResourceEntry `json:"resources"`
}

type ResourcePlan struct {
	journalPath string
	journal     resourceJournal
}

func NewResourcePlan(journalPath, operation string, resources []ResourceEntry) (*ResourcePlan, error) {
	if strings.TrimSpace(operation) == "" {
		return nil, app.NewError(app.CodeInternal, "resource plan operation is empty", nil)
	}
	resolvedJournal, err := validateResourcePath(journalPath)
	if err != nil {
		return nil, err
	}
	entries := append([]ResourceEntry(nil), resources...)
	seen := make(map[string]struct{}, len(entries))
	for index := range entries {
		entry := &entries[index]
		resolved, pathErr := validateResourcePath(entry.Path)
		if pathErr != nil {
			return nil, pathErr
		}
		entry.Path = resolved
		if _, duplicate := seen[resolved]; duplicate {
			return nil, app.NewError(app.CodeConflict, "resource plan contains a duplicate path", nil)
		}
		seen[resolved] = struct{}{}
		if err := validateResourceEntry(*entry); err != nil {
			return nil, err
		}
	}
	if err := validateResourceSequence(entries); err != nil {
		return nil, err
	}
	return &ResourcePlan{
		journalPath: resolvedJournal,
		journal: resourceJournal{
			Schema: "forecast-resource-journal/v1", Operation: operation,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Resources: entries,
		},
	}, nil
}

func (p *ResourcePlan) Begin() error {
	if p == nil {
		return app.NewError(app.CodeInternal, "resource plan is nil", nil)
	}
	emptyDigest := ResourceDigest(nil)
	for _, entry := range p.journal.Resources {
		if entry.BeforeSHA256 == "" {
			continue
		}
		info, err := os.Lstat(entry.Path)
		if errors.Is(err, fs.ErrNotExist) && entry.BeforeSHA256 == emptyDigest {
			continue
		}
		if err != nil || isLinkOrReparse(info) || !info.Mode().IsRegular() {
			return app.NewError(app.CodeConflict, "resource before identity cannot be validated", err)
		}
		digest, err := fileSHA256(entry.Path)
		if err != nil || digest != entry.BeforeSHA256 {
			return app.NewError(app.CodeConflict, "resource changed before its plan began", err)
		}
	}
	encoded, err := encodeResourceJournal(p.journal)
	if err != nil {
		return err
	}
	return CreateExclusive(p.journalPath, encoded, 0o600)
}

func (p *ResourcePlan) MarkCreated(path string, digest string) error {
	return p.update(path, ResourceCreated, digest)
}

func (p *ResourcePlan) MarkReplaced(path string, digest string) error {
	return p.update(path, ResourceReplaced, digest)
}

func (p *ResourcePlan) MarkCommitted(path string) error {
	return p.update(path, ResourceCommitted, "")
}

func (p *ResourcePlan) update(path string, state ResourceState, digest string) error {
	if p == nil {
		return app.NewError(app.CodeInternal, "resource plan is nil", nil)
	}
	resolved, err := validateResourcePath(path)
	if err != nil {
		return err
	}
	index := -1
	for position := range p.journal.Resources {
		if p.journal.Resources[position].Path == resolved {
			index = position
			break
		}
	}
	if index < 0 {
		return app.NewError(app.CodeNotFound, "resource is not part of the side-effect plan", nil)
	}
	entry := p.journal.Resources[index]
	if state == ResourceCreated && !entry.Owned {
		return app.NewError(app.CodeConflict, "an unowned resource cannot be marked as created", nil)
	}
	if digest != "" && !validDigest(digest) {
		return app.NewError(app.CodeInvalidData, "resource digest is not lowercase SHA-256", nil)
	}
	entry.State = state
	if digest != "" {
		entry.SHA256 = digest
	}
	p.journal.Resources[index] = entry
	return replaceResourceJournal(p.journalPath, p.journal)
}

func (p *ResourcePlan) Finish() error {
	if p == nil {
		return nil
	}
	return removeJournalAndSync(p.journalPath, filepath.Dir(p.journalPath))
}

type ResourceRecovery struct {
	Removed  []string
	Retained []string
}

func RecoverResourcePlan(ctx context.Context, journalPath string) (ResourceRecovery, error) {
	var report ResourceRecovery
	resolvedJournal, err := validateResourcePath(journalPath)
	if err != nil {
		return report, err
	}
	journal, err := readResourceJournal(resolvedJournal)
	if err != nil {
		return report, err
	}
	allCommitted := len(journal.Resources) > 0
	for _, entry := range journal.Resources {
		allCommitted = allCommitted && entry.State == ResourceCommitted
	}
	if allCommitted {
		for _, entry := range journal.Resources {
			info, statErr := os.Lstat(entry.Path)
			if statErr != nil || isLinkOrReparse(info) {
				return report, app.NewError(app.CodeConflict, "committed resource identity cannot be validated", statErr)
			}
			if entry.Type == ResourceDirectory {
				if !info.IsDir() {
					return report, app.NewError(app.CodeConflict, "committed resource changed type", nil)
				}
			} else if !info.Mode().IsRegular() {
				return report, app.NewError(app.CodeConflict, "committed resource changed type", nil)
			} else if entry.SHA256 != "" {
				digest, digestErr := fileSHA256(entry.Path)
				if digestErr != nil || digest != entry.SHA256 {
					return report, app.NewError(app.CodeConflict, "committed resource changed after replacement", digestErr)
				}
			}
			report.Retained = append(report.Retained, entry.Path)
		}
		sort.Strings(report.Retained)
		if err := removeJournalAndSync(resolvedJournal, filepath.Dir(resolvedJournal)); err != nil {
			return report, err
		}
		return report, nil
	}
	for _, entry := range journal.Resources {
		if entry.State == ResourceReplaced || entry.State == ResourceCommitted {
			return report, app.NewError(app.CodeConflict, "resource recovery preserved a partially committed replacement for explicit recovery", nil)
		}
	}
	entries := append([]ResourceEntry(nil), journal.Resources...)
	sort.SliceStable(entries, func(i, j int) bool {
		return len(entries[i].Path) > len(entries[j].Path)
	})
	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return report, app.NewError(app.CodeInterrupted, "resource recovery was interrupted", ctx.Err())
		}
		if entry.State != ResourceCreated || !entry.Owned || entry.Rollback != ResourceRollbackRemoveOwned {
			if entry.State != ResourcePlanned {
				report.Retained = append(report.Retained, entry.Path)
			}
			continue
		}
		removed, removeErr := removeOwnedResource(entry)
		if removeErr != nil {
			return report, removeErr
		}
		if removed {
			report.Removed = append(report.Removed, entry.Path)
		}
	}
	sort.Strings(report.Removed)
	sort.Strings(report.Retained)
	if err := removeJournalAndSync(resolvedJournal, filepath.Dir(resolvedJournal)); err != nil {
		return report, err
	}
	return report, nil
}

func removeOwnedResource(entry ResourceEntry) (bool, error) {
	info, err := os.Lstat(entry.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, app.NewError(app.CodeIO, "owned resource cannot be inspected during recovery", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, app.NewError(app.CodeConflict, "owned resource became a link and was retained", nil)
	}
	switch entry.Type {
	case ResourceDirectory:
		if !info.IsDir() {
			return false, app.NewError(app.CodeConflict, "owned directory changed type and was retained", nil)
		}
		if err := os.Remove(entry.Path); err != nil {
			if errors.Is(err, fs.ErrExist) || strings.Contains(strings.ToLower(err.Error()), "not empty") {
				return false, app.NewError(app.CodeConflict, "owned directory is not empty and was retained", nil)
			}
			return false, app.NewError(app.CodeIO, "owned directory cannot be removed during recovery", err)
		}
		return true, nil
	case ResourceFile:
		if !info.Mode().IsRegular() {
			return false, app.NewError(app.CodeConflict, "owned file changed type and was retained", nil)
		}
		if entry.SHA256 == "" {
			return false, app.NewError(app.CodeConflict, "owned file has no recorded digest and was retained", nil)
		}
		digest, digestErr := fileSHA256(entry.Path)
		if digestErr != nil {
			return false, digestErr
		}
		if digest != entry.SHA256 {
			return false, app.NewError(app.CodeConflict, "owned file changed after creation and was retained", nil)
		}
		if err := os.Remove(entry.Path); err != nil {
			return false, app.NewError(app.CodeIO, "owned file cannot be removed during recovery", err)
		}
		return true, nil
	default:
		return false, app.NewError(app.CodeConflict, "resource has an unsupported type and was retained", nil)
	}
}

func readResourceJournal(path string) (resourceJournal, error) {
	var journal resourceJournal
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return journal, app.NewError(app.CodeNotFound, "resource recovery journal does not exist", err)
		}
		return journal, app.NewError(app.CodeIO, "resource recovery journal cannot be inspected", err)
	}
	if !info.Mode().IsRegular() {
		return journal, app.NewError(app.CodeConflict, "resource recovery journal is not a regular file", nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return journal, app.NewError(app.CodeIO, "resource recovery journal cannot be opened", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return journal, app.NewError(app.CodeConflict, "resource recovery journal is invalid", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return journal, app.NewError(app.CodeConflict, "resource recovery journal contains extra data", err)
	}
	if journal.Schema != "forecast-resource-journal/v1" || journal.Operation == "" {
		return journal, app.NewError(app.CodeConflict, "resource recovery journal contract is invalid", nil)
	}
	for _, entry := range journal.Resources {
		if err := validateResourceEntry(entry); err != nil {
			return journal, err
		}
		resolved, pathErr := validateResourcePath(entry.Path)
		if pathErr != nil || resolved != entry.Path {
			return journal, app.NewError(app.CodeConflict, "resource recovery path is invalid", pathErr)
		}
	}
	if err := validateResourceSequence(journal.Resources); err != nil {
		return journal, app.NewError(app.CodeConflict, "resource recovery ordering is invalid", err)
	}
	return journal, nil
}

func encodeResourceJournal(journal resourceJournal) ([]byte, error) {
	encoded, err := json.Marshal(journal)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "resource recovery journal cannot be encoded", err)
	}
	return append(encoded, '\n'), nil
}

func replaceResourceJournal(path string, journal resourceJournal) error {
	encoded, err := encodeResourceJournal(journal)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".forecast-resource-journal-*.tmp")
	if err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal temporary file cannot be created", err)
	}
	tempPath := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal permissions cannot be set", err)
	}
	if _, err := temp.Write(encoded); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal cannot be written", err)
	}
	if err := temp.Sync(); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal cannot be flushed", err)
	}
	if err := temp.Close(); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal cannot be closed", err)
	}
	if err := safeReplace(tempPath, path); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal cannot be replaced", err)
	}
	if err := syncParentDirectory(filepath.Dir(path)); err != nil {
		return app.NewError(app.CodeIO, "resource recovery journal directory cannot be flushed", err)
	}
	ok = true
	return nil
}

func validateResourceEntry(entry ResourceEntry) error {
	switch entry.Kind {
	case ResourceLedger, ResourceTarget, ResourceEvidenceIndex, ResourceTimestampRequest, ResourceTimestampResponse, ResourceTimestampTrust, ResourceKey, ResourcePackage:
	default:
		return app.NewError(app.CodeInvalidData, "resource plan kind is invalid", nil)
	}
	if entry.Type != ResourceFile && entry.Type != ResourceDirectory {
		return app.NewError(app.CodeInvalidData, "resource plan type is invalid", nil)
	}
	if entry.Rollback != ResourceRollbackNone && entry.Rollback != ResourceRollbackRemoveOwned && entry.Rollback != ResourceRollbackRetainSecret {
		return app.NewError(app.CodeInvalidData, "resource rollback class is invalid", nil)
	}
	if entry.State == "" {
		entry.State = ResourcePlanned
	}
	if entry.State != ResourcePlanned && entry.State != ResourceCreated && entry.State != ResourceReplaced && entry.State != ResourceCommitted {
		return app.NewError(app.CodeInvalidData, "resource state is invalid", nil)
	}
	if entry.Rollback == ResourceRollbackRetainSecret && entry.Kind != ResourceKey {
		return app.NewError(app.CodeInvalidData, "retain-secret rollback is valid only for key resources", nil)
	}
	if entry.Rollback == ResourceRollbackRemoveOwned && !entry.Owned {
		return app.NewError(app.CodeInvalidData, "only owned resources may use remove-owned rollback", nil)
	}
	if entry.SHA256 != "" && !validDigest(entry.SHA256) {
		return app.NewError(app.CodeInvalidData, "resource digest is not lowercase SHA-256", nil)
	}
	if entry.BeforeSHA256 != "" && !validDigest(entry.BeforeSHA256) {
		return app.NewError(app.CodeInvalidData, "resource before digest is not lowercase SHA-256", nil)
	}
	return nil
}

func validateResourceSequence(entries []ResourceEntry) error {
	indexPosition, ledgerPosition := -1, -1
	for position, entry := range entries {
		if entry.Kind == ResourceEvidenceIndex {
			indexPosition = position
		}
		if entry.Kind == ResourceLedger {
			ledgerPosition = position
		}
	}
	if indexPosition >= 0 && ledgerPosition >= 0 {
		if indexPosition > ledgerPosition || entries[indexPosition].BeforeSHA256 == "" || entries[ledgerPosition].BeforeSHA256 == "" {
			return app.NewError(app.CodeInvalidData, "evidence index replacement must precede the ledger and record both before identities", nil)
		}
		for _, position := range []int{indexPosition, ledgerPosition} {
			if entries[position].State == ResourceReplaced && entries[position].SHA256 == "" {
				return app.NewError(app.CodeInvalidData, "replaced evidence resources must record exact after identities", nil)
			}
		}
	}
	return nil
}

func validateResourcePath(path string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return "", app.NewError(app.CodeUsage, "resource plan path must be an explicit absolute path", nil)
	}
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	if cleaned == string(filepath.Separator) || cleaned == volume+string(filepath.Separator) {
		return "", app.NewError(app.CodeUsage, "resource plan path is too broad", nil)
	}
	return cleaned, nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", app.NewError(app.CodeIO, "resource cannot be opened for digest verification", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, 1<<30)); err != nil {
		return "", app.NewError(app.CodeIO, "resource digest cannot be computed", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ResourceDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (p *ResourcePlan) String() string {
	if p == nil {
		return "ResourcePlan(<nil>)"
	}
	return fmt.Sprintf("ResourcePlan(%s,%d)", p.journal.Operation, len(p.journal.Resources))
}
