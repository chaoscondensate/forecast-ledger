package service

import (
	"bytes"
	"encoding/json"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

// Optional distinguishes an omitted patch field from an explicit JSON null.
type Optional[T any] struct {
	Set   bool
	Null  bool
	Value T
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Null = true
		var zero T
		o.Value = zero
		return nil
	}
	o.Null = false
	return json.Unmarshal(data, &o.Value)
}

func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Null {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// RevisionInput is the complete immutable semantic meaning of a question at
// one point in time. Revision append always supplies the whole value; patches
// are deliberately unsupported.
type RevisionInput struct {
	ID                   ledger.Slug         `json:"id"`
	EffectiveAt          ledger.Timestamp    `json:"effective_at"`
	RecordedAt           *ledger.Timestamp   `json:"recorded_at,omitempty"`
	Title                string              `json:"title"`
	ResolutionCriteria   string              `json:"resolution_criteria"`
	ForecastingOpensAt   *ledger.Timestamp   `json:"forecasting_opens_at,omitempty"`
	ExpectedResolutionAt ledger.Timestamp    `json:"expected_resolution_at"`
	OutcomeSpace         ledger.OutcomeSpace `json:"outcome_space"`
	Domain               ledger.Domain       `json:"domain"`
	Provenance           *ledger.Provenance  `json:"provenance,omitempty"`
}

type InitialForecastInput struct {
	Visibility           ledger.ForecastVisibility       `json:"visibility"`
	ID                   ledger.Slug                     `json:"id"`
	ForecastedAt         ledger.Timestamp                `json:"forecasted_at"`
	RecordedAt           *ledger.Timestamp               `json:"recorded_at,omitempty"`
	Representations      []ledger.ForecastRepresentation `json:"representations"`
	Rationale            *string                         `json:"rationale,omitempty"`
	KeyFactors           *[]string                       `json:"key_factors,omitempty"`
	Comment              *string                         `json:"comment,omitempty"`
	PublicNote           *string                         `json:"public_note,omitempty"`
	Provenance           *ledger.Provenance              `json:"provenance,omitempty"`
	SupersedesForecastID *ledger.Slug                    `json:"supersedes_forecast_id,omitempty"`
}

type InitialQuestionInput struct {
	ID              ledger.Slug           `json:"id"`
	CreatedAt       *ledger.Timestamp     `json:"created_at,omitempty"`
	Revision        RevisionInput         `json:"revision"`
	Tags            *[]ledger.Slug        `json:"tags,omitempty"`
	Notes           *string               `json:"notes,omitempty"`
	InitialForecast *InitialForecastInput `json:"initial_forecast,omitempty"`
}

type InitInput struct {
	Title       *string                         `json:"title,omitempty"`
	Description *string                         `json:"description,omitempty"`
	CreatedAt   *ledger.Timestamp               `json:"created_at,omitempty"`
	Contact     *ledger.Contact                 `json:"contact,omitempty"`
	Profiles    *[]ledger.Profile               `json:"profiles,omitempty"`
	Members     *[]ledger.Member                `json:"members,omitempty"`
	Platforms   map[ledger.Slug]ledger.Platform `json:"platforms,omitempty"`
	Question    *InitialQuestionInput           `json:"question,omitempty"`
}

type ForecasterMetadataPatchInput struct {
	Kind     Optional[ledger.ForecasterKind] `json:"kind"`
	Name     Optional[string]                `json:"name"`
	Contact  Optional[ledger.Contact]        `json:"contact"`
	Profiles Optional[[]ledger.Profile]      `json:"profiles"`
	Members  Optional[[]ledger.Member]       `json:"members"`
}

type RootMetadataPatchInput struct {
	Title           Optional[string]                       `json:"title"`
	Description     Optional[string]                       `json:"description"`
	DefaultTimezone Optional[string]                       `json:"default_timezone"`
	Forecaster      Optional[ForecasterMetadataPatchInput] `json:"forecaster"`
}

type PlatformCreateInput struct {
	Name    string                  `json:"name"`
	Kind    ledger.PlatformKind     `json:"kind"`
	URL     *string                 `json:"url,omitempty"`
	Account *ledger.PlatformAccount `json:"account,omitempty"`
}

type PlatformAccountPatchInput struct {
	Username   Optional[string] `json:"username"`
	UserID     Optional[string] `json:"user_id"`
	ProfileURL Optional[string] `json:"profile_url"`
}

type PlatformPatchInput struct {
	Name    Optional[string]                    `json:"name"`
	Kind    Optional[ledger.PlatformKind]       `json:"kind"`
	URL     Optional[string]                    `json:"url"`
	Account Optional[PlatformAccountPatchInput] `json:"account"`
}

type QuestionAddInput struct {
	CreatedAt       *ledger.Timestamp     `json:"created_at,omitempty"`
	Revision        RevisionInput         `json:"revision"`
	Tags            *[]ledger.Slug        `json:"tags,omitempty"`
	Notes           *string               `json:"notes,omitempty"`
	InitialForecast *InitialForecastInput `json:"initial_forecast,omitempty"`
}

type NormalizedQuestionCreate struct {
	ID    ledger.Slug
	Input QuestionAddInput
}

func NormalizeInitialQuestion(input InitialQuestionInput) NormalizedQuestionCreate {
	return NormalizedQuestionCreate{ID: input.ID, Input: QuestionAddInput{
		CreatedAt: input.CreatedAt, Revision: input.Revision, Tags: input.Tags,
		Notes: input.Notes, InitialForecast: input.InitialForecast,
	}}
}

type InitialCreationShape string

const (
	CreationLedgerOnly     InitialCreationShape = "ledger_only"
	CreationQuestionOnly   InitialCreationShape = "question_only"
	CreationPublicForecast InitialCreationShape = "public_forecast"
	CreationSealedForecast InitialCreationShape = "sealed_forecast"
)

func ClassifyInitInput(input InitInput) (InitialCreationShape, error) {
	if input.Question == nil {
		return CreationLedgerOnly, nil
	}
	return classifyInitialForecast(input.Question.InitialForecast)
}

func ClassifyQuestionAddInput(input QuestionAddInput) (InitialCreationShape, error) {
	return classifyInitialForecast(input.InitialForecast)
}

func classifyInitialForecast(input *InitialForecastInput) (InitialCreationShape, error) {
	if input == nil {
		return CreationQuestionOnly, nil
	}
	switch input.Visibility {
	case ledger.VisibilityPublic:
		return CreationPublicForecast, nil
	case ledger.VisibilitySealed:
		return CreationSealedForecast, nil
	default:
		return "", invalidField("initial_forecast.visibility", "initial forecast visibility must be public or sealed")
	}
}

// QuestionPatchInput contains only mutable question-level metadata. Meaning is
// changed by appending RevisionInput, never by rewriting an older revision.
type QuestionPatchInput struct {
	Tags   Optional[[]ledger.Slug]         `json:"tags"`
	Notes  Optional[string]                `json:"notes"`
	Status Optional[ledger.QuestionStatus] `json:"status"`
}

type ForecastCreateInput struct {
	QuestionRevisionID   ledger.Slug                     `json:"question_revision_id"`
	ForecastedAt         ledger.Timestamp                `json:"forecasted_at"`
	RecordedAt           *ledger.Timestamp               `json:"recorded_at,omitempty"`
	Representations      []ledger.ForecastRepresentation `json:"representations"`
	Rationale            *string                         `json:"rationale,omitempty"`
	KeyFactors           *[]string                       `json:"key_factors,omitempty"`
	Comment              *string                         `json:"comment,omitempty"`
	PublicNote           *string                         `json:"public_note,omitempty"`
	Provenance           *ledger.Provenance              `json:"provenance,omitempty"`
	SupersedesForecastID *ledger.Slug                    `json:"supersedes_forecast_id,omitempty"`
}

type SealedForecastInput struct {
	QuestionRevisionID   ledger.Slug                     `json:"question_revision_id"`
	ForecastedAt         ledger.Timestamp                `json:"forecasted_at"`
	RecordedAt           *ledger.Timestamp               `json:"recorded_at,omitempty"`
	Representations      []ledger.ForecastRepresentation `json:"representations"`
	Rationale            *string                         `json:"rationale,omitempty"`
	KeyFactors           *[]string                       `json:"key_factors,omitempty"`
	Comment              *string                         `json:"comment,omitempty"`
	PublicNote           *string                         `json:"public_note,omitempty"`
	Provenance           *ledger.Provenance              `json:"provenance,omitempty"`
	SupersedesForecastID *ledger.Slug                    `json:"supersedes_forecast_id,omitempty"`
}

// SealedForecastPrivateInput is the only protected authoring input. All IDs,
// times, selectors, provenance and public notes remain direct public fields.
type SealedForecastPrivateInput struct {
	Representations []ledger.ForecastRepresentation `json:"representations"`
	Rationale       *string                         `json:"rationale,omitempty"`
	KeyFactors      *[]string                       `json:"key_factors,omitempty"`
	Comment         *string                         `json:"comment,omitempty"`
}

type KeyHintUpdateInput struct {
	KeyHint string `json:"key_hint"`
}

type EvidenceSourceInput struct {
	Title         string            `json:"title"`
	URL           string            `json:"url"`
	RetrievedAt   ledger.Timestamp  `json:"retrieved_at"`
	Publisher     *string           `json:"publisher,omitempty"`
	PublishedAt   *ledger.Timestamp `json:"published_at,omitempty"`
	ContentDigest *ledger.Digest    `json:"content_digest,omitempty"`
}

type ResolutionInput struct {
	QuestionRevisionID ledger.Slug           `json:"question_revision_id"`
	Outcome            ledger.ScalarValue    `json:"outcome"`
	OutcomeKnownAt     ledger.Timestamp      `json:"outcome_known_at"`
	RecordedAt         *ledger.Timestamp     `json:"recorded_at,omitempty"`
	Sources            []EvidenceSourceInput `json:"sources"`
	Notes              *string               `json:"notes,omitempty"`
}

type UnresolvedResolutionInput struct {
	Reason     string                `json:"reason"`
	RecordedAt *ledger.Timestamp     `json:"recorded_at,omitempty"`
	Sources    []EvidenceSourceInput `json:"sources,omitempty"`
}

type NotApplicableInput struct {
	RelationshipID ledger.Slug       `json:"relationship_id"`
	Reason         string            `json:"reason"`
	RecordedAt     *ledger.Timestamp `json:"recorded_at,omitempty"`
}

type GroupCreateInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
}

type GroupPatchInput struct {
	Title       Optional[string] `json:"title"`
	Description Optional[string] `json:"description"`
}

type RelationshipInput struct {
	Relationship ledger.Relationship `json:"relationship"`
}

type LifecycleInput struct {
	ID          ledger.Slug        `json:"id"`
	EffectiveAt ledger.Timestamp   `json:"effective_at"`
	RecordedAt  *ledger.Timestamp  `json:"recorded_at,omitempty"`
	Reason      *string            `json:"reason,omitempty"`
	Provenance  *ledger.Provenance `json:"provenance,omitempty"`
}

type PublicationBuildInput struct {
	File   string `json:"file"`
	Output string `json:"output"`
	DryRun bool   `json:"dry_run,omitempty"`
}

type PublicationVerifyInput struct {
	File     string `json:"file"`
	Manifest string `json:"manifest"`
}
