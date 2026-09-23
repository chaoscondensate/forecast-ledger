package publication

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"

	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

const (
	ManifestProfile   = "forecast-ledger-publication/v3"
	RoleLedger        = "ledger"
	RoleEvidenceIndex = "evidence_index"
	RoleTarget        = RoleForecastTarget
	RoleLifecycle     = RoleLifecycleTarget
	RoleRequest       = RoleRFC3161Request
	RoleResponse      = RoleRFC3161Response
	RoleCABundle      = RoleX509CABundle
	MaxManifestBytes  = 8 << 20
)

type Digest struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type Entry struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Digest Digest `json:"digest"`
}

type Manifest struct {
	Profile           string           `json:"profile"`
	Contract          ContractIdentity `json:"contract"`
	LedgerPath        string           `json:"ledger_path"`
	EvidenceIndexPath string           `json:"evidence_index_path"`
	Entries           []Entry          `json:"entries"`
}

func Encode(manifest Manifest) ([]byte, error) {
	if err := Validate(manifest); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return nil, err
	}
	result, err := canonical.Marshal(parsed.Root.Any())
	if err != nil {
		return nil, err
	}
	return result, nil
}

func Decode(data []byte) (Manifest, error) {
	if len(data) == 0 || len(data) > MaxManifestBytes {
		return Manifest{}, errors.New("publication manifest is empty or too large")
	}
	parsed, err := document.ParseJSON(bytes.NewReader(data), document.Limits{MaxBytes: MaxManifestBytes, MaxDepth: 16, MaxNodes: MaxEvidenceEntries * 8, MaxScalarBytes: MaxEvidencePathBytes})
	if err != nil {
		return Manifest{}, err
	}
	canonicalBytes, err := canonical.Marshal(parsed.Root.Any())
	if err != nil || !bytes.Equal(canonicalBytes, data) {
		return Manifest{}, errors.New("publication manifest is not exact RFC 8785 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Validate(manifest Manifest) error {
	if manifest.Profile != ManifestProfile || manifest.Contract != CurrentContractIdentity() || manifest.LedgerPath == "" || manifest.EvidenceIndexPath != EvidenceIndexPath || len(manifest.Entries) < 2 {
		return errors.New("publication manifest profile or ledger path is invalid")
	}
	paths := make([]string, len(manifest.Entries))
	ledgerCount, indexCount := 0, 0
	previous := ""
	for index, entry := range manifest.Entries {
		if err := storage.ValidateRelativePath(entry.Path); err != nil {
			return err
		}
		if index > 0 && entry.Path <= previous {
			return errors.New("publication manifest entries must be strictly sorted by path")
		}
		previous = entry.Path
		paths[index] = entry.Path
		if entry.Size < 0 || entry.Digest.Algorithm != "sha-256" || !lowerSHA256(entry.Digest.Value) {
			return errors.New("publication manifest entry digest or size is invalid")
		}
		switch entry.Role {
		case RoleLedger:
			ledgerCount++
			if entry.Path != manifest.LedgerPath {
				return errors.New("ledger entry path does not match ledger_path")
			}
		case RoleEvidenceIndex:
			indexCount++
			if entry.Path != manifest.EvidenceIndexPath {
				return errors.New("evidence index entry path does not match evidence_index_path")
			}
		case RoleTarget, RoleLifecycle:
			if len(entry.Path) < len("proofs/targets/") || entry.Path[:len("proofs/targets/")] != "proofs/targets/" {
				return errors.New("forecast target has an invalid package path")
			}
		case RoleRequest, RoleResponse:
			if len(entry.Path) < len("proofs/timestamps/") || entry.Path[:len("proofs/timestamps/")] != "proofs/timestamps/" {
				return errors.New("RFC 3161 request or response has an invalid package path")
			}
		case RoleCABundle:
			// CA bundles follow the exact safe relative path retained by the ledger.
		default:
			return errors.New("publication manifest entry role is not supported")
		}
	}
	if ledgerCount != 1 || indexCount != 1 {
		return errors.New("publication manifest must contain exactly one ledger and one evidence index")
	}
	if err := storage.DetectPortablePathCollisions(paths); err != nil {
		return err
	}
	return nil
}

func SortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
}

func lowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
