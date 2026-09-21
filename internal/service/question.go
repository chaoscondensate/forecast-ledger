package service

import (
	"sort"
	"strconv"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

type QuestionMutation struct {
	Ledger               *ledger.Ledger
	Patches              []document.PatchOperation
	TargetCoveredChanged bool
	AffectedForecastIDs  []ledger.Slug
	PriorStatus          ledger.QuestionStatus
}

type IntegrityCounts struct {
	Unanchored int `json:"unanchored"`
	Pending    int `json:"pending"`
	Verified   int `json:"verified"`
	Failed     int `json:"failed"`
}

type QuestionSummary struct {
	ID                   ledger.Slug           `json:"id"`
	Title                string                `json:"title"`
	OutcomeKind          ledger.OutcomeKind    `json:"outcome_kind"`
	Status               ledger.QuestionStatus `json:"status"`
	CurrentRevisionID    ledger.Slug           `json:"current_revision_id"`
	RevisionCount        int                   `json:"revision_count"`
	ExpectedResolutionAt ledger.Timestamp      `json:"expected_resolution_at"`
	ForecastCount        int                   `json:"forecast_count"`
	Integrity            IntegrityCounts       `json:"integrity"`
}

type QuestionForecastSummary struct {
	Summary    ForecastSummary `json:"summary"`
	PublicNote *string         `json:"public_note,omitempty"`
	Commitment *CommitmentView `json:"commitment,omitempty"`
}

type QuestionView struct {
	ID                ledger.Slug               `json:"id"`
	Status            ledger.QuestionStatus     `json:"status"`
	CreatedAt         ledger.Timestamp          `json:"created_at"`
	CurrentRevisionID ledger.Slug               `json:"current_revision_id"`
	Revisions         []ledger.QuestionRevision `json:"revisions"`
	Tags              *[]ledger.Slug            `json:"tags,omitempty"`
	Notes             *string                   `json:"notes,omitempty"`
	Resolution        *ledger.Resolution        `json:"resolution,omitempty"`
	Forecasts         []QuestionForecastSummary `json:"forecasts"`
}

func BuildQuestionAddPublic(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionMutation, error) {
	prospective, err := BuildQuestionWithInitialPublicForecast(model, input, observedAt)
	if err != nil {
		return QuestionMutation{}, err
	}
	question := prospective.Questions[len(prospective.Questions)-1]
	value, err := jsonPatchValue(question)
	if err != nil {
		return QuestionMutation{}, err
	}
	return QuestionMutation{Ledger: prospective, Patches: []document.PatchOperation{{Kind: document.PatchAdd, Pointer: "/questions/-", Value: value}}}, nil
}

func BuildQuestionAddEmpty(model *ledger.Ledger, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionMutation, error) {
	return BuildQuestionWithoutForecast(model, input, observedAt)
}

// BuildQuestionUpdate changes only question-level metadata and a nonterminal
// workflow status. Revision content is immutable.
func BuildQuestionUpdate(model *ledger.Ledger, id ledger.Slug, input QuestionPatchInput) (QuestionMutation, error) {
	position, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionMutation{}, err
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return QuestionMutation{}, err
	}
	updated := &prospective.Questions[position]
	base := "/questions/" + strconv.Itoa(position)
	patches := []document.PatchOperation{}
	if input.Tags.Set {
		if input.Tags.Null {
			updated.Tags = nil
		} else {
			values := append([]ledger.Slug(nil), input.Tags.Value...)
			updated.Tags = &values
		}
		patches = append(patches, optionalFieldPatch(base+"/tags", question.Tags != nil, updated.Tags))
	}
	if input.Notes.Set {
		if input.Notes.Null {
			updated.Notes = nil
		} else {
			value := input.Notes.Value
			updated.Notes = &value
		}
		patches = append(patches, optionalFieldPatch(base+"/notes", question.Notes != nil, updated.Notes))
	}
	if input.Status.Set {
		if input.Status.Null || !isNonterminalStatus(input.Status.Value) {
			return QuestionMutation{}, invalidField("status", "question update accepts only open, closed, or awaiting_resolution")
		}
		if !isNonterminalStatus(question.Status) || question.Resolution != nil {
			return QuestionMutation{}, app.NewError(app.CodeConflict, "terminal status must be set through a resolution operation", nil)
		}
		if input.Status.Value != question.Status {
			updated.Status = input.Status.Value
			patches = append(patches, replacePatch(base+"/status", input.Status.Value))
		}
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return QuestionMutation{}, err
	}
	return QuestionMutation{Ledger: prospective, Patches: patches, PriorStatus: question.Status}, nil
}

func BuildQuestionRevise(model *ledger.Ledger, id ledger.Slug, input RevisionInput, observedAt ledger.Timestamp) (QuestionMutation, error) {
	position, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionMutation{}, err
	}
	if !isNonterminalStatus(question.Status) {
		return QuestionMutation{}, app.NewError(app.CodeConflict, "a terminal question cannot be revised", nil)
	}
	revision, err := buildRevision(input, observedAt)
	if err != nil {
		return QuestionMutation{}, err
	}
	for _, existing := range question.Revisions {
		if existing.ID == revision.ID {
			return QuestionMutation{}, app.NewError(app.CodeConflict, "question revision ID already exists", nil)
		}
	}
	last := question.Revisions[len(question.Revisions)-1]
	if err := ValidateChronology(last.EffectiveAt, "previous.effective_at", revision.EffectiveAt, "effective_at", false); err != nil {
		return QuestionMutation{}, err
	}
	if err := ValidateChronology(last.RecordedAt, "previous.recorded_at", revision.RecordedAt, "recorded_at", true); err != nil {
		return QuestionMutation{}, err
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return QuestionMutation{}, err
	}
	prospective.Questions[position].Revisions = append(prospective.Questions[position].Revisions, revision)
	prospective.Questions[position].CurrentRevisionID = revision.ID
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return QuestionMutation{}, err
	}
	value, err := jsonPatchValue(revision)
	if err != nil {
		return QuestionMutation{}, err
	}
	base := "/questions/" + strconv.Itoa(position)
	return QuestionMutation{Ledger: prospective, PriorStatus: question.Status, Patches: []document.PatchOperation{
		{Kind: document.PatchAdd, Pointer: base + "/revisions/-", Value: value}, replacePatch(base+"/current_revision_id", revision.ID),
	}}, nil
}

func ListQuestions(model *ledger.Ledger) ([]QuestionSummary, error) {
	if model == nil {
		return nil, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	items := make([]QuestionSummary, len(model.Questions))
	for i, question := range model.Questions {
		items[i] = summarizeQuestion(question)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func ShowQuestion(model *ledger.Ledger, id ledger.Slug) (QuestionView, error) {
	_, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionView{}, err
	}
	view := QuestionView{ID: question.ID, Status: question.Status, CreatedAt: question.CreatedAt, CurrentRevisionID: question.CurrentRevisionID, Revisions: append([]ledger.QuestionRevision(nil), question.Revisions...), Tags: cloneSlugs(question.Tags), Notes: cloneString(question.Notes), Resolution: cloneResolution(question.Resolution), Forecasts: make([]QuestionForecastSummary, len(question.Forecasts))}
	for i, forecast := range question.Forecasts {
		view.Forecasts[i] = QuestionForecastSummary{Summary: summarizeForecast(forecast), PublicNote: cloneString(forecast.PublicNote), Commitment: commitmentView(forecast.Commitment)}
		if view.Forecasts[i].Commitment != nil {
			view.Forecasts[i].Commitment.Encryption.Nonce, view.Forecasts[i].Commitment.Encryption.Ciphertext = "", ""
		}
	}
	return view, nil
}

func BuildQuestionResolve(model *ledger.Ledger, id ledger.Slug, input ResolutionInput, observedAt ledger.Timestamp) (QuestionMutation, error) {
	position, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionMutation{}, err
	}
	if question.Status != ledger.QuestionClosed && question.Status != ledger.QuestionAwaitingResolution && question.Status != ledger.QuestionDisputed {
		return QuestionMutation{}, app.NewError(app.CodeConflict, "question must be closed, awaiting resolution, or disputed before resolution", nil)
	}
	if len(input.Sources) == 0 {
		return QuestionMutation{}, invalidField("sources", "at least one resolution source is required")
	}
	recorded := observedAt
	if input.RecordedAt != nil {
		recorded = *input.RecordedAt
	}
	resolution := ledger.Resolution{Resolved: &ledger.ResolvedResolution{Status: ledger.ResolutionResolved, QuestionRevisionID: input.QuestionRevisionID, Outcome: input.Outcome, OutcomeKnownAt: input.OutcomeKnownAt, RecordedAt: recorded, Sources: resolutionSources(input.Sources), Notes: cloneString(input.Notes)}}
	return buildQuestionTerminalMutation(model, position, question, ledger.QuestionResolved, resolution)
}

func BuildQuestionUnresolved(model *ledger.Ledger, id ledger.Slug, status ledger.ResolutionStatus, input UnresolvedResolutionInput, observedAt ledger.Timestamp) (QuestionMutation, error) {
	position, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionMutation{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return QuestionMutation{}, invalidField("reason", "resolution reason must not be empty")
	}
	var questionStatus ledger.QuestionStatus
	switch status {
	case ledger.ResolutionAmbiguous:
		questionStatus = ledger.QuestionAmbiguous
	case ledger.ResolutionVoid:
		questionStatus = ledger.QuestionVoid
	case ledger.ResolutionDisputed:
		questionStatus = ledger.QuestionDisputed
	default:
		return QuestionMutation{}, invalidField("status", "unsupported unresolved terminal status")
	}
	if status != ledger.ResolutionDisputed && question.Status != ledger.QuestionClosed && question.Status != ledger.QuestionAwaitingResolution {
		return QuestionMutation{}, app.NewError(app.CodeConflict, "question must be closed or awaiting resolution", nil)
	}
	if status == ledger.ResolutionDisputed && isNonterminalStatus(question.Status) {
		return QuestionMutation{}, app.NewError(app.CodeConflict, "only a terminal question can be disputed", nil)
	}
	recorded := observedAt
	if input.RecordedAt != nil {
		recorded = *input.RecordedAt
	}
	sources := resolutionSources(input.Sources)
	var optionalSources *[]ledger.ResolutionSource
	if len(sources) > 0 {
		optionalSources = &sources
	}
	resolution := ledger.Resolution{Unresolved: &ledger.UnresolvedResolution{Status: status, Reason: input.Reason, RecordedAt: recorded, Sources: optionalSources}}
	return buildQuestionTerminalMutation(model, position, question, questionStatus, resolution)
}

func BuildQuestionNotApplicable(model *ledger.Ledger, id ledger.Slug, input NotApplicableInput, observedAt ledger.Timestamp) (QuestionMutation, error) {
	position, question, err := selectQuestion(model, id)
	if err != nil {
		return QuestionMutation{}, err
	}
	recorded := observedAt
	if input.RecordedAt != nil {
		recorded = *input.RecordedAt
	}
	resolution := ledger.Resolution{NotApplicable: &ledger.NotApplicableResolution{Status: ledger.ResolutionNotApplicable, RelationshipID: input.RelationshipID, Reason: input.Reason, RecordedAt: recorded}}
	return buildQuestionTerminalMutation(model, position, question, ledger.QuestionNotApplicable, resolution)
}

func buildQuestionTerminalMutation(model *ledger.Ledger, position int, question ledger.Question, status ledger.QuestionStatus, resolution ledger.Resolution) (QuestionMutation, error) {
	prospective, err := cloneLedger(model)
	if err != nil {
		return QuestionMutation{}, err
	}
	prospective.Questions[position].Status, prospective.Questions[position].Resolution = status, &resolution
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return QuestionMutation{}, err
	}
	value, err := jsonPatchValue(resolution)
	if err != nil {
		return QuestionMutation{}, err
	}
	base := "/questions/" + strconv.Itoa(position)
	patches := []document.PatchOperation{replacePatch(base+"/status", status)}
	if question.Resolution == nil {
		patches = append(patches, document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/resolution", Value: value})
	} else {
		patches = append(patches, document.PatchOperation{Kind: document.PatchReplace, Pointer: base + "/resolution", Value: value})
	}
	return QuestionMutation{Ledger: prospective, PriorStatus: question.Status, Patches: patches}, nil
}

func summarizeQuestion(question ledger.Question) QuestionSummary {
	current := question.Revisions[len(question.Revisions)-1]
	result := QuestionSummary{ID: question.ID, Title: current.Title, OutcomeKind: current.OutcomeSpace.Kind, Status: question.Status, CurrentRevisionID: question.CurrentRevisionID, RevisionCount: len(question.Revisions), ExpectedResolutionAt: current.ExpectedResolutionAt, ForecastCount: len(question.Forecasts)}
	for _, forecast := range question.Forecasts {
		switch integrityStatus(forecast.Integrity) {
		case ledger.IntegrityUnanchored:
			result.Integrity.Unanchored++
		case ledger.IntegrityPending:
			result.Integrity.Pending++
		case ledger.IntegrityVerified:
			result.Integrity.Verified++
		case ledger.IntegrityFailed:
			result.Integrity.Failed++
		}
	}
	return result
}

func resolutionSources(input []EvidenceSourceInput) []ledger.ResolutionSource {
	result := make([]ledger.ResolutionSource, len(input))
	for i, source := range input {
		result[i] = ledger.ResolutionSource{Title: source.Title, Publisher: cloneString(source.Publisher), URL: source.URL, PublishedAt: source.PublishedAt, RetrievedAt: source.RetrievedAt, ContentDigest: source.ContentDigest}
	}
	return result
}

func isNonterminalStatus(status ledger.QuestionStatus) bool {
	return status == ledger.QuestionOpen || status == ledger.QuestionClosed || status == ledger.QuestionAwaitingResolution
}

func questionHasTargetMetadata(question ledger.Question) bool {
	for _, forecast := range question.Forecasts {
		if forecast.Integrity.Pending != nil || forecast.Integrity.Verified != nil || forecast.Integrity.Failed != nil && forecast.Integrity.Failed.Target != nil {
			return true
		}
	}
	return false
}

func frozenQuestionConflict(id ledger.Slug) error {
	return app.WithDetails(app.NewError(app.CodeConflict, "question fields covered by an existing forecast target cannot be changed", nil), map[string]any{"question_id": id})
}

func cloneResolution(value *ledger.Resolution) *ledger.Resolution {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func replacePatch(pointer string, value any) document.PatchOperation {
	return document.PatchOperation{Kind: document.PatchReplace, Pointer: pointer, Value: value}
}

func optionalFieldPatch(pointer string, existed bool, value any) document.PatchOperation {
	if value == nil {
		return document.PatchOperation{Kind: document.PatchRemove, Pointer: pointer}
	}
	kind := document.PatchAdd
	if existed {
		kind = document.PatchReplace
	}
	return document.PatchOperation{Kind: kind, Pointer: pointer, Value: value}
}
