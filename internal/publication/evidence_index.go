package publication

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

const (
	EvidenceIndexProfile  = "forecast-evidence-index/v1"
	EvidenceIndexPath     = "proofs/evidence-index.json"
	RoleForecastTarget    = "forecast_target"
	RoleLifecycleTarget   = "lifecycle_target"
	RoleRFC3161Request    = "rfc3161_request"
	RoleRFC3161Response   = "rfc3161_response"
	RoleX509CABundle      = "x509_ca_bundle"
	PEMCertificateBundle  = "pem-certificate-bundle"
	MaxEvidenceIndexBytes = 8 << 20
	MaxEvidenceEntries    = 4096
	MaxEvidencePathBytes  = 1024
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

type ContractIdentity struct {
	SchemaID      string `json:"schema_id"`
	SchemaVersion string `json:"schema_version"`
	SchemaSHA256  string `json:"schema_sha256"`
}

func CurrentContractIdentity() ContractIdentity {
	return ContractIdentity{SchemaID: schema.SchemaID, SchemaVersion: schema.Version, SchemaSHA256: schema.SchemaSHA256}
}

type EvidenceIndex struct {
	Schema   string           `json:"schema"`
	LedgerID string           `json:"ledger_id"`
	Contract ContractIdentity `json:"contract"`
	Entries  []EvidenceEntry  `json:"entries"`
}

type ForecastTargetBinding struct {
	QuestionID string `json:"question_id"`
	ForecastID string `json:"forecast_id"`
	Scope      string `json:"scope"`
}

type LifecycleTargetBinding struct {
	QuestionID   string `json:"question_id"`
	ForecastID   string `json:"forecast_id"`
	CheckpointID string `json:"checkpoint_id"`
	HeadEventID  string `json:"head_event_id"`
	Scope        string `json:"scope"`
}

type TargetReference struct {
	TargetPath string `json:"target_path"`
}

type ResponseReferences struct {
	TargetPath  string  `json:"target_path"`
	RequestPath string  `json:"request_path"`
	TrustPath   *string `json:"trust_path,omitempty"`
}

type RequestBinding struct {
	HashAlgorithm        string `json:"hash_algorithm"`
	MessageImprintSHA256 string `json:"message_imprint_sha256"`
}

type ResponseBinding struct {
	TSAURL string `json:"tsa_url"`
}

type EvidenceEntry struct {
	Role      string                  `json:"role"`
	Path      string                  `json:"path"`
	Size      int64                   `json:"size"`
	Digest    Digest                  `json:"digest"`
	Forecast  *ForecastTargetBinding  `json:"-"`
	Lifecycle *LifecycleTargetBinding `json:"-"`
	TargetRef *TargetReference        `json:"-"`
	Response  *ResponseReferences     `json:"-"`
	Request   *RequestBinding         `json:"-"`
	TSA       *ResponseBinding        `json:"-"`
	Format    *string                 `json:"-"`
}

func (entry EvidenceEntry) MarshalJSON() ([]byte, error) {
	base := map[string]any{"role": entry.Role, "path": entry.Path, "size": entry.Size, "digest": entry.Digest}
	switch entry.Role {
	case RoleForecastTarget:
		base["binding"] = entry.Forecast
	case RoleLifecycleTarget:
		base["binding"] = entry.Lifecycle
	case RoleRFC3161Request:
		base["references"], base["binding"] = entry.TargetRef, entry.Request
	case RoleRFC3161Response:
		base["references"], base["binding"] = entry.Response, entry.TSA
	case RoleX509CABundle:
		if entry.Format != nil {
			base["format"] = *entry.Format
		}
	}
	return json.Marshal(base)
}

func (entry *EvidenceEntry) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	type artifact struct {
		Role   string `json:"role"`
		Path   string `json:"path"`
		Size   int64  `json:"size"`
		Digest Digest `json:"digest"`
	}
	decode := func(destination any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		return decoder.Decode(destination)
	}
	*entry = EvidenceEntry{}
	switch discriminator.Role {
	case RoleForecastTarget:
		var value struct {
			artifact
			Binding ForecastTargetBinding `json:"binding"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		entry.Role, entry.Path, entry.Size, entry.Digest, entry.Forecast = value.Role, value.Path, value.Size, value.Digest, &value.Binding
	case RoleLifecycleTarget:
		var value struct {
			artifact
			Binding LifecycleTargetBinding `json:"binding"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		entry.Role, entry.Path, entry.Size, entry.Digest, entry.Lifecycle = value.Role, value.Path, value.Size, value.Digest, &value.Binding
	case RoleRFC3161Request:
		var value struct {
			artifact
			References TargetReference `json:"references"`
			Binding    RequestBinding  `json:"binding"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		entry.Role, entry.Path, entry.Size, entry.Digest, entry.TargetRef, entry.Request = value.Role, value.Path, value.Size, value.Digest, &value.References, &value.Binding
	case RoleRFC3161Response:
		var value struct {
			artifact
			References ResponseReferences `json:"references"`
			Binding    ResponseBinding    `json:"binding"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		entry.Role, entry.Path, entry.Size, entry.Digest, entry.Response, entry.TSA = value.Role, value.Path, value.Size, value.Digest, &value.References, &value.Binding
	case RoleX509CABundle:
		var value struct {
			artifact
			Format string `json:"format"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		entry.Role, entry.Path, entry.Size, entry.Digest, entry.Format = value.Role, value.Path, value.Size, value.Digest, &value.Format
	default:
		return errors.New("evidence index entry role is not supported")
	}
	return nil
}

func EncodeEvidenceIndex(index EvidenceIndex, allowEmpty bool) ([]byte, error) {
	if err := ValidateEvidenceIndex(index, allowEmpty); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		return nil, err
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.Limits{MaxBytes: MaxEvidenceIndexBytes, MaxDepth: 16, MaxNodes: MaxEvidenceEntries * 24, MaxScalarBytes: MaxEvidencePathBytes})
	if err != nil {
		return nil, err
	}
	return canonical.Marshal(parsed.Root.Any())
}

func DecodeEvidenceIndex(data []byte, allowEmpty bool) (EvidenceIndex, error) {
	if len(data) == 0 || len(data) > MaxEvidenceIndexBytes {
		return EvidenceIndex{}, errors.New("evidence index is empty or too large")
	}
	parsed, err := document.ParseJSON(bytes.NewReader(data), document.Limits{MaxBytes: MaxEvidenceIndexBytes, MaxDepth: 16, MaxNodes: MaxEvidenceEntries * 24, MaxScalarBytes: MaxEvidencePathBytes})
	if err != nil {
		return EvidenceIndex{}, err
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil || !bytes.Equal(canonicalBytes, data) {
		return EvidenceIndex{}, errors.New("evidence index is not exact RFC 8785 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var index EvidenceIndex
	if err := decoder.Decode(&index); err != nil {
		return EvidenceIndex{}, fmt.Errorf("evidence index shape is invalid: %w", err)
	}
	if err := ValidateEvidenceIndex(index, allowEmpty); err != nil {
		return EvidenceIndex{}, err
	}
	return index, nil
}

func ValidateEvidenceIndex(index EvidenceIndex, allowEmpty bool) error {
	if index.Schema != EvidenceIndexProfile || index.Contract != CurrentContractIdentity() {
		return errors.New("evidence index profile or contract identity is invalid")
	}
	if !validSlug(index.LedgerID) {
		return errors.New("evidence index ledger_id is invalid")
	}
	if len(index.Entries) > MaxEvidenceEntries || !allowEmpty && len(index.Entries) == 0 {
		return errors.New("evidence index entry count is invalid")
	}
	byPath := make(map[string]EvidenceEntry, len(index.Entries))
	paths := make([]string, len(index.Entries))
	previous := ""
	for position, entry := range index.Entries {
		if position > 0 && entry.Path <= previous {
			return errors.New("evidence index entries must have unique paths in strict ascending order")
		}
		previous, paths[position] = entry.Path, entry.Path
		if err := validateEvidenceEntry(entry); err != nil {
			return fmt.Errorf("evidence index entry %q: %w", entry.Path, err)
		}
		byPath[entry.Path] = entry
	}
	if err := storage.DetectPortablePathCollisions(paths); err != nil {
		return err
	}
	for _, entry := range index.Entries {
		switch entry.Role {
		case RoleRFC3161Request:
			target, ok := byPath[entry.TargetRef.TargetPath]
			if !ok || !isTargetRole(target.Role) || entry.Request.MessageImprintSHA256 != target.Digest.Value {
				return errors.New("RFC 3161 request references a missing or mismatched target")
			}
		case RoleRFC3161Response:
			target, targetOK := byPath[entry.Response.TargetPath]
			request, requestOK := byPath[entry.Response.RequestPath]
			if !targetOK || !isTargetRole(target.Role) || !requestOK || request.Role != RoleRFC3161Request || request.TargetRef == nil || request.TargetRef.TargetPath != entry.Response.TargetPath {
				return errors.New("RFC 3161 response references missing or inconsistent evidence")
			}
			if entry.Response.TrustPath != nil {
				trust, ok := byPath[*entry.Response.TrustPath]
				if !ok || trust.Role != RoleX509CABundle {
					return errors.New("RFC 3161 response references a missing trust bundle")
				}
			}
		}
	}
	return nil
}

func validateEvidenceEntry(entry EvidenceEntry) error {
	if len(entry.Path) > MaxEvidencePathBytes || entry.Path == EvidenceIndexPath || entry.Size < 0 || entry.Digest.Algorithm != "sha-256" || !lowerSHA256(entry.Digest.Value) {
		return errors.New("artifact path, size, or digest is invalid")
	}
	if err := storage.ValidateRelativePath(entry.Path); err != nil {
		return err
	}
	proof := strings.HasPrefix(entry.Path, "proofs/") && entry.Path != EvidenceIndexPath
	trust := strings.HasPrefix(entry.Path, "trust/")
	switch entry.Role {
	case RoleForecastTarget:
		if !proof || entry.Forecast == nil || !validSlug(entry.Forecast.QuestionID) || !validSlug(entry.Forecast.ForecastID) || entry.Forecast.Scope != schema.ForecastTargetProfile {
			return errors.New("forecast target binding is invalid")
		}
	case RoleLifecycleTarget:
		if !proof || entry.Lifecycle == nil || !validSlug(entry.Lifecycle.QuestionID) || !validSlug(entry.Lifecycle.ForecastID) || !validSlug(entry.Lifecycle.CheckpointID) || !validSlug(entry.Lifecycle.HeadEventID) || entry.Lifecycle.Scope != schema.LifecycleTargetProfile {
			return errors.New("lifecycle target binding is invalid")
		}
	case RoleRFC3161Request:
		if !proof || entry.TargetRef == nil || entry.Request == nil || !validProofPath(entry.TargetRef.TargetPath) || entry.Request.HashAlgorithm != "sha256" || !lowerSHA256(entry.Request.MessageImprintSHA256) {
			return errors.New("RFC 3161 request binding is invalid")
		}
	case RoleRFC3161Response:
		if !proof || entry.Response == nil || entry.TSA == nil || !validProofPath(entry.Response.TargetPath) || !validProofPath(entry.Response.RequestPath) || entry.Response.TrustPath != nil && !validTrustPath(*entry.Response.TrustPath) || !validURL(entry.TSA.TSAURL) {
			return errors.New("RFC 3161 response binding is invalid")
		}
	case RoleX509CABundle:
		if !trust || entry.Format == nil || *entry.Format != PEMCertificateBundle {
			return errors.New("CA bundle format or path is invalid")
		}
	default:
		return errors.New("entry role is not supported")
	}
	return nil
}

func SortEvidenceEntries(entries []EvidenceEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
}

func validSlug(value string) bool { return len(value) <= 128 && slugPattern.MatchString(value) }
func validProofPath(value string) bool {
	return value != EvidenceIndexPath && strings.HasPrefix(value, "proofs/") && storage.ValidateRelativePath(value) == nil
}
func validTrustPath(value string) bool {
	return strings.HasPrefix(value, "trust/") && storage.ValidateRelativePath(value) == nil
}
func isTargetRole(role string) bool { return role == RoleForecastTarget || role == RoleLifecycleTarget }
func validURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.Host != ""
}
