package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ForecastTarget struct {
	Scope            string       `json:"scope" yaml:"scope"`
	Canonicalization string       `json:"canonicalization" yaml:"canonicalization"`
	ArtifactPath     RelativePath `json:"artifact_path" yaml:"artifact_path"`
	Digest           Digest       `json:"digest" yaml:"digest"`
}

type LifecycleTarget struct {
	Scope            string       `json:"scope" yaml:"scope"`
	Canonicalization string       `json:"canonicalization" yaml:"canonicalization"`
	ArtifactPath     RelativePath `json:"artifact_path" yaml:"artifact_path"`
	Digest           Digest       `json:"digest" yaml:"digest"`
}

type RFC3161TimestampState string

const (
	RFC3161Pending  RFC3161TimestampState = "pending"
	RFC3161Verified RFC3161TimestampState = "verified"
)

type RFC3161Timestamp struct {
	Type          string                `json:"type" yaml:"type"`
	RequestPath   RelativePath          `json:"request_path" yaml:"request_path"`
	ResponsePath  RelativePath          `json:"response_path" yaml:"response_path"`
	TSAURL        string                `json:"tsa_url" yaml:"tsa_url"`
	HashAlgorithm string                `json:"hash_algorithm" yaml:"hash_algorithm"`
	State         RFC3161TimestampState `json:"state" yaml:"state"`
	GenTime       *Timestamp            `json:"gen_time,omitempty" yaml:"gen_time,omitempty"`
	PolicyOID     *string               `json:"policy_oid,omitempty" yaml:"policy_oid,omitempty"`
	SerialNumber  *string               `json:"serial_number,omitempty" yaml:"serial_number,omitempty"`
	CABundlePath  *RelativePath         `json:"ca_bundle_path,omitempty" yaml:"ca_bundle_path,omitempty"`
}

type ExternalAnchorKind string

const (
	AnchorGitCommit             ExternalAnchorKind = "git_commit"
	AnchorURL                   ExternalAnchorKind = "url"
	AnchorNostrEvent            ExternalAnchorKind = "nostr_event"
	AnchorBlockchainTransaction ExternalAnchorKind = "blockchain_transaction"
)

type ExternalAnchor struct {
	Kind       ExternalAnchorKind `json:"kind" yaml:"kind"`
	Value      string             `json:"value" yaml:"value"`
	ObservedAt *Timestamp         `json:"observed_at,omitempty" yaml:"observed_at,omitempty"`
}

type IntegrityStatus string

const (
	IntegrityUnanchored IntegrityStatus = "unanchored"
	IntegrityRetained   IntegrityStatus = "retained"
	IntegrityPending    IntegrityStatus = "pending"
	IntegrityVerified   IntegrityStatus = "verified"
	IntegrityFailed     IntegrityStatus = "failed"
)

type Integrity struct {
	Unanchored *UnanchoredIntegrity
	Retained   *RetainedIntegrity
	Pending    *PendingIntegrity
	Verified   *VerifiedIntegrity
	Failed     *FailedIntegrity
}

type UnanchoredIntegrity struct {
	Status IntegrityStatus `json:"status" yaml:"status"`
	Note   *string         `json:"note,omitempty" yaml:"note,omitempty"`
}

type RetainedIntegrity struct {
	Status IntegrityStatus `json:"status" yaml:"status"`
	Target ForecastTarget  `json:"target" yaml:"target"`
}

type PendingIntegrity struct {
	Status          IntegrityStatus    `json:"status" yaml:"status"`
	Target          ForecastTarget     `json:"target" yaml:"target"`
	Timestamps      []RFC3161Timestamp `json:"timestamps" yaml:"timestamps"`
	ExternalAnchors *[]ExternalAnchor  `json:"external_anchors,omitempty" yaml:"external_anchors,omitempty"`
}

type VerifiedIntegrity struct {
	Status          IntegrityStatus    `json:"status" yaml:"status"`
	Target          ForecastTarget     `json:"target" yaml:"target"`
	Timestamps      []RFC3161Timestamp `json:"timestamps" yaml:"timestamps"`
	VerifiedAt      Timestamp          `json:"verified_at" yaml:"verified_at"`
	ExternalAnchors *[]ExternalAnchor  `json:"external_anchors,omitempty" yaml:"external_anchors,omitempty"`
}

type FailedIntegrity struct {
	Status        IntegrityStatus     `json:"status" yaml:"status"`
	FailureReason string              `json:"failure_reason" yaml:"failure_reason"`
	Target        *ForecastTarget     `json:"target,omitempty" yaml:"target,omitempty"`
	Timestamps    *[]RFC3161Timestamp `json:"timestamps,omitempty" yaml:"timestamps,omitempty"`
}

// LifecycleIntegrity deliberately has no unanchored variant. An activity
// checkpoint exists only after a target has been retained or an attempted
// verification has failed.
type LifecycleIntegrity struct {
	Retained *RetainedLifecycleIntegrity
	Pending  *PendingLifecycleIntegrity
	Verified *VerifiedLifecycleIntegrity
	Failed   *FailedLifecycleIntegrity
}

type RetainedLifecycleIntegrity struct {
	Status IntegrityStatus `json:"status" yaml:"status"`
	Target LifecycleTarget `json:"target" yaml:"target"`
}

type PendingLifecycleIntegrity struct {
	Status          IntegrityStatus    `json:"status" yaml:"status"`
	Target          LifecycleTarget    `json:"target" yaml:"target"`
	Timestamps      []RFC3161Timestamp `json:"timestamps" yaml:"timestamps"`
	ExternalAnchors *[]ExternalAnchor  `json:"external_anchors,omitempty" yaml:"external_anchors,omitempty"`
}

type VerifiedLifecycleIntegrity struct {
	Status          IntegrityStatus    `json:"status" yaml:"status"`
	Target          LifecycleTarget    `json:"target" yaml:"target"`
	Timestamps      []RFC3161Timestamp `json:"timestamps" yaml:"timestamps"`
	VerifiedAt      Timestamp          `json:"verified_at" yaml:"verified_at"`
	ExternalAnchors *[]ExternalAnchor  `json:"external_anchors,omitempty" yaml:"external_anchors,omitempty"`
}

type FailedLifecycleIntegrity struct {
	Status        IntegrityStatus     `json:"status" yaml:"status"`
	FailureReason string              `json:"failure_reason" yaml:"failure_reason"`
	Target        *LifecycleTarget    `json:"target,omitempty" yaml:"target,omitempty"`
	Timestamps    *[]RFC3161Timestamp `json:"timestamps,omitempty" yaml:"timestamps,omitempty"`
}

func (v LifecycleIntegrity) MarshalJSON() ([]byte, error) {
	return marshalOne("lifecycle integrity", v.Retained, v.Pending, v.Verified, v.Failed)
}

func (v *LifecycleIntegrity) UnmarshalJSON(data []byte) error {
	*v = LifecycleIntegrity{}
	var discriminator struct {
		Status IntegrityStatus `json:"status"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Status {
	case IntegrityRetained:
		v.Retained = new(RetainedLifecycleIntegrity)
		return decodeClosed(data, v.Retained)
	case IntegrityPending:
		v.Pending = new(PendingLifecycleIntegrity)
		return decodeClosed(data, v.Pending)
	case IntegrityVerified:
		v.Verified = new(VerifiedLifecycleIntegrity)
		return decodeClosed(data, v.Verified)
	case IntegrityFailed:
		v.Failed = new(FailedLifecycleIntegrity)
		return decodeClosed(data, v.Failed)
	default:
		return fmt.Errorf("unknown lifecycle integrity status %q", discriminator.Status)
	}
}

func (v Integrity) MarshalJSON() ([]byte, error) {
	return marshalOne("integrity", v.Unanchored, v.Retained, v.Pending, v.Verified, v.Failed)
}

func (v *Integrity) UnmarshalJSON(data []byte) error {
	*v = Integrity{}
	var discriminator struct {
		Status IntegrityStatus `json:"status"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Status {
	case IntegrityUnanchored:
		v.Unanchored = new(UnanchoredIntegrity)
		return decodeClosed(data, v.Unanchored)
	case IntegrityRetained:
		v.Retained = new(RetainedIntegrity)
		return decodeClosed(data, v.Retained)
	case IntegrityPending:
		v.Pending = new(PendingIntegrity)
		return decodeClosed(data, v.Pending)
	case IntegrityVerified:
		v.Verified = new(VerifiedIntegrity)
		return decodeClosed(data, v.Verified)
	case IntegrityFailed:
		v.Failed = new(FailedIntegrity)
		return decodeClosed(data, v.Failed)
	default:
		return fmt.Errorf("unknown integrity status %q", discriminator.Status)
	}
}

type Encryption struct {
	Algorithm  string           `json:"algorithm" yaml:"algorithm"`
	Nonce      Base64Nonce12    `json:"nonce" yaml:"nonce"`
	Ciphertext Base64Ciphertext `json:"ciphertext" yaml:"ciphertext"`
}

type Commitment struct {
	Sealed   *SealedCommitment
	Revealed *RevealedCommitment
}

type SealedCommitment struct {
	Scheme         string     `json:"scheme" yaml:"scheme"`
	CommitmentHash Digest     `json:"commitment_hash" yaml:"commitment_hash"`
	Encryption     Encryption `json:"encryption" yaml:"encryption"`
	KeyHint        string     `json:"key_hint" yaml:"key_hint"`
}

type RevealedCommitment struct {
	Scheme         string     `json:"scheme" yaml:"scheme"`
	CommitmentHash Digest     `json:"commitment_hash" yaml:"commitment_hash"`
	Encryption     Encryption `json:"encryption" yaml:"encryption"`
	KeyHint        string     `json:"key_hint" yaml:"key_hint"`
	RevealedAt     Timestamp  `json:"revealed_at" yaml:"revealed_at"`
	RevealedKey    Hex32      `json:"revealed_key" yaml:"revealed_key"`
}

func (v Commitment) MarshalJSON() ([]byte, error) {
	return marshalOne("commitment", v.Sealed, v.Revealed)
}

func (v *Commitment) UnmarshalJSON(data []byte) error {
	*v = Commitment{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, hasAt := fields["revealed_at"]
	_, hasKey := fields["revealed_key"]
	if hasAt != hasKey {
		return errors.New("revealed commitment requires revealed_at and revealed_key together")
	}
	if hasAt {
		v.Revealed = new(RevealedCommitment)
		return decodeClosed(data, v.Revealed)
	}
	v.Sealed = new(SealedCommitment)
	return decodeClosed(data, v.Sealed)
}
