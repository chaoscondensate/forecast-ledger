package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// String scalar aliases retain the exact source spelling needed by v2
// canonicalization and domain checks.
type Timestamp string
type Date string
type Decimal string
type Probability string
type OpenProbability string
type DayStep string
type SecondStep string
type Slug string
type RelativePath string
type Hex32 string
type Base64Nonce12 string
type Base64Ciphertext string
type SchemaVersion string

type Ledger struct {
	Schema          *string           `json:"$schema,omitempty" yaml:"$schema,omitempty"`
	SchemaVersion   SchemaVersion     `json:"schema_version" yaml:"schema_version"`
	LedgerID        Slug              `json:"ledger_id" yaml:"ledger_id"`
	Title           *string           `json:"title,omitempty" yaml:"title,omitempty"`
	Description     *string           `json:"description,omitempty" yaml:"description,omitempty"`
	CreatedAt       Timestamp         `json:"created_at" yaml:"created_at"`
	DefaultTimezone string            `json:"default_timezone" yaml:"default_timezone"`
	Forecaster      Forecaster        `json:"forecaster" yaml:"forecaster"`
	Publication     *Publication      `json:"publication,omitempty" yaml:"publication,omitempty"`
	Platforms       map[Slug]Platform `json:"platforms" yaml:"platforms"`
	Groups          *[]Group          `json:"groups,omitempty" yaml:"groups,omitempty"`
	Relationships   *[]Relationship   `json:"relationships,omitempty" yaml:"relationships,omitempty"`
	Questions       []Question        `json:"questions" yaml:"questions"`
}

func (v *Ledger) UnmarshalJSON(data []byte) error {
	type plain Ledger
	return decodeClosed(data, (*plain)(v))
}

type Publication struct {
	History       string       `json:"history" yaml:"history"`
	RepositoryURL string       `json:"repository_url" yaml:"repository_url"`
	DefaultBranch string       `json:"default_branch" yaml:"default_branch"`
	LedgerPath    RelativePath `json:"ledger_path" yaml:"ledger_path"`
}

type ForecasterKind string

const (
	ForecasterIndividual ForecasterKind = "individual"
	ForecasterTeam       ForecasterKind = "team"
)

type Forecaster struct {
	ID       Slug           `json:"id" yaml:"id"`
	Kind     ForecasterKind `json:"kind" yaml:"kind"`
	Name     string         `json:"name" yaml:"name"`
	Contact  *Contact       `json:"contact,omitempty" yaml:"contact,omitempty"`
	Profiles *[]Profile     `json:"profiles,omitempty" yaml:"profiles,omitempty"`
	Members  *[]Member      `json:"members,omitempty" yaml:"members,omitempty"`
}

type Contact struct {
	Email   *string `json:"email,omitempty" yaml:"email,omitempty"`
	Website *string `json:"website,omitempty" yaml:"website,omitempty"`
}

type Profile struct {
	Service  string  `json:"service" yaml:"service"`
	Username *string `json:"username,omitempty" yaml:"username,omitempty"`
	URL      string  `json:"url" yaml:"url"`
}

type Member struct {
	ID       Slug       `json:"id" yaml:"id"`
	Name     string     `json:"name" yaml:"name"`
	Role     *string    `json:"role,omitempty" yaml:"role,omitempty"`
	Profiles *[]Profile `json:"profiles,omitempty" yaml:"profiles,omitempty"`
}

type PlatformKind string

const (
	PlatformScoringMarket PlatformKind = "scoring_platform"
	PlatformPrediction    PlatformKind = "prediction_market"
	PlatformSelfHosted    PlatformKind = "self_hosted"
	PlatformInternal      PlatformKind = "internal"
	PlatformInformal      PlatformKind = "informal"
)

type Platform struct {
	Name    string           `json:"name" yaml:"name"`
	Kind    PlatformKind     `json:"kind" yaml:"kind"`
	URL     *string          `json:"url,omitempty" yaml:"url,omitempty"`
	Account *PlatformAccount `json:"account,omitempty" yaml:"account,omitempty"`
}

type PlatformAccount struct {
	Username   *string `json:"username,omitempty" yaml:"username,omitempty"`
	UserID     *string `json:"user_id,omitempty" yaml:"user_id,omitempty"`
	ProfileURL *string `json:"profile_url,omitempty" yaml:"profile_url,omitempty"`
}

type Digest struct {
	Algorithm string `json:"algorithm" yaml:"algorithm"`
	Value     Hex32  `json:"value" yaml:"value"`
}

type ArtifactSnapshot struct {
	ArtifactPath RelativePath `json:"artifact_path" yaml:"artifact_path"`
	MediaType    string       `json:"media_type" yaml:"media_type"`
	Digest       Digest       `json:"digest" yaml:"digest"`
}

type Importer struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

type Provenance struct {
	Platform            Slug              `json:"platform" yaml:"platform"`
	RemoteObjectID      string            `json:"remote_object_id" yaml:"remote_object_id"`
	RemoteObjectVersion *string           `json:"remote_object_version,omitempty" yaml:"remote_object_version,omitempty"`
	URL                 *string           `json:"url,omitempty" yaml:"url,omitempty"`
	SourceCreatedAt     *Timestamp        `json:"source_created_at,omitempty" yaml:"source_created_at,omitempty"`
	SourceUpdatedAt     *Timestamp        `json:"source_updated_at,omitempty" yaml:"source_updated_at,omitempty"`
	RetrievedAt         Timestamp         `json:"retrieved_at" yaml:"retrieved_at"`
	Snapshot            *ArtifactSnapshot `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`
	Importer            *Importer         `json:"importer,omitempty" yaml:"importer,omitempty"`
}

type ScalarValue struct {
	Boolean *bool
	String  *string
}

func (v ScalarValue) MarshalJSON() ([]byte, error) {
	return marshalOne("scalar value", v.Boolean, v.String)
}

func (v *ScalarValue) UnmarshalJSON(data []byte) error {
	*v = ScalarValue{}
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		v.Boolean = new(bool)
		return json.Unmarshal(trimmed, v.Boolean)
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		v.String = new(string)
		if err := json.Unmarshal(trimmed, v.String); err != nil {
			return err
		}
		if *v.String == "" {
			return errors.New("scalar value string must not be empty")
		}
		return nil
	}
	return errors.New("scalar value must be a boolean or non-empty string")
}

func marshalOne(name string, variants ...any) ([]byte, error) {
	var selected any
	for _, variant := range variants {
		if isNil(variant) {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("%s has more than one variant", name)
		}
		selected = variant
	}
	if selected == nil {
		return nil, fmt.Errorf("%s has no variant", name)
	}
	return json.Marshal(selected)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func decodeClosed(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("expected one JSON value")
		}
		return err
	}
	return nil
}
