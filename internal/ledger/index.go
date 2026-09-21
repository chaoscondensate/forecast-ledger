package ledger

import (
	"fmt"
	"sort"
)

type IndexErrorCode string

const (
	IndexDuplicateQuestion      IndexErrorCode = "duplicate_question_id"
	IndexDuplicateRevision      IndexErrorCode = "duplicate_question_revision_id"
	IndexUnknownCurrentRevision IndexErrorCode = "unknown_current_question_revision"
	IndexDuplicateForecast      IndexErrorCode = "duplicate_forecast_id"
	IndexUnknownSuperseded      IndexErrorCode = "unknown_superseded_forecast"
	IndexCrossQuestionLink      IndexErrorCode = "cross_question_supersession"
	IndexForwardLink            IndexErrorCode = "forward_supersession"
	IndexDuplicateGroup         IndexErrorCode = "duplicate_group_id"
	IndexDuplicateRelationship  IndexErrorCode = "duplicate_relationship_id"
)

type IndexError struct {
	Code       IndexErrorCode
	QuestionID Slug
	RevisionID Slug
	ForecastID Slug
	Reference  Slug
}

func (e *IndexError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("ledger index %s at question %q revision %q forecast %q reference %q", e.Code, e.QuestionID, e.RevisionID, e.ForecastID, e.Reference)
}

type ForecastLocation struct {
	QuestionIndex int
	ForecastIndex int
	QuestionID    Slug
	ForecastID    Slug
}

type RevisionLocation struct {
	QuestionIndex int
	RevisionIndex int
	QuestionID    Slug
	RevisionID    Slug
}

// Index is an immutable lookup snapshot for one decoded ledger. Positions are
// stored instead of pointers so callers cannot mutate append-only records.
type Index struct {
	PlatformIDs               map[Slug]struct{}
	GroupPositions            map[Slug]int
	RelationshipPositions     map[Slug]int
	QuestionPositions         map[Slug]int
	QuestionRevisionPositions map[Slug]map[Slug]int
	ForecastLocations         map[Slug]ForecastLocation
	QuestionForecastIDs       map[Slug][]Slug
	PlatformQuestionIDs       map[Slug][]Slug
	PlatformForecastIDs       map[Slug][]Slug
	Supersedes                map[Slug]Slug
	SupersededBy              map[Slug][]Slug
}

func BuildIndex(model *Ledger) (*Index, error) {
	if model == nil {
		return nil, fmt.Errorf("ledger is nil")
	}
	index := &Index{
		PlatformIDs:               make(map[Slug]struct{}, len(model.Platforms)),
		GroupPositions:            make(map[Slug]int),
		RelationshipPositions:     make(map[Slug]int),
		QuestionPositions:         make(map[Slug]int, len(model.Questions)),
		QuestionRevisionPositions: make(map[Slug]map[Slug]int, len(model.Questions)),
		ForecastLocations:         make(map[Slug]ForecastLocation),
		QuestionForecastIDs:       make(map[Slug][]Slug, len(model.Questions)),
		PlatformQuestionIDs:       make(map[Slug][]Slug, len(model.Platforms)),
		PlatformForecastIDs:       make(map[Slug][]Slug, len(model.Platforms)),
		Supersedes:                make(map[Slug]Slug),
		SupersededBy:              make(map[Slug][]Slug),
	}
	for id := range model.Platforms {
		index.PlatformIDs[id] = struct{}{}
	}
	if model.Groups != nil {
		for position, group := range *model.Groups {
			if _, exists := index.GroupPositions[group.ID]; exists {
				return nil, &IndexError{Code: IndexDuplicateGroup, Reference: group.ID}
			}
			index.GroupPositions[group.ID] = position
		}
	}
	if model.Relationships != nil {
		for position, relationship := range *model.Relationships {
			id := relationshipID(relationship)
			if _, exists := index.RelationshipPositions[id]; exists {
				return nil, &IndexError{Code: IndexDuplicateRelationship, Reference: id}
			}
			index.RelationshipPositions[id] = position
		}
	}

	for questionPosition := range model.Questions {
		question := &model.Questions[questionPosition]
		if _, exists := index.QuestionPositions[question.ID]; exists {
			return nil, &IndexError{Code: IndexDuplicateQuestion, QuestionID: question.ID}
		}
		index.QuestionPositions[question.ID] = questionPosition
		revisions := make(map[Slug]int, len(question.Revisions))
		for revisionPosition, revision := range question.Revisions {
			if _, exists := revisions[revision.ID]; exists {
				return nil, &IndexError{Code: IndexDuplicateRevision, QuestionID: question.ID, RevisionID: revision.ID}
			}
			revisions[revision.ID] = revisionPosition
			if revision.Provenance != nil {
				appendUnique(index.PlatformQuestionIDs, revision.Provenance.Platform, question.ID)
			}
		}
		index.QuestionRevisionPositions[question.ID] = revisions
		if _, exists := revisions[question.CurrentRevisionID]; !exists {
			return nil, &IndexError{Code: IndexUnknownCurrentRevision, QuestionID: question.ID, Reference: question.CurrentRevisionID}
		}
		for forecastPosition := range question.Forecasts {
			forecast := &question.Forecasts[forecastPosition]
			if _, exists := index.ForecastLocations[forecast.ID]; exists {
				return nil, &IndexError{Code: IndexDuplicateForecast, QuestionID: question.ID, ForecastID: forecast.ID}
			}
			index.ForecastLocations[forecast.ID] = ForecastLocation{
				QuestionIndex: questionPosition,
				ForecastIndex: forecastPosition,
				QuestionID:    question.ID,
				ForecastID:    forecast.ID,
			}
			index.QuestionForecastIDs[question.ID] = append(index.QuestionForecastIDs[question.ID], forecast.ID)
			if forecast.Provenance != nil {
				appendUnique(index.PlatformForecastIDs, forecast.Provenance.Platform, forecast.ID)
			}
		}
	}

	for questionPosition := range model.Questions {
		question := &model.Questions[questionPosition]
		for forecastPosition := range question.Forecasts {
			forecast := &question.Forecasts[forecastPosition]
			if forecast.SupersedesForecastID == nil {
				continue
			}
			reference := *forecast.SupersedesForecastID
			location, exists := index.ForecastLocations[reference]
			if !exists {
				return nil, &IndexError{Code: IndexUnknownSuperseded, QuestionID: question.ID, ForecastID: forecast.ID, Reference: reference}
			}
			if location.QuestionID != question.ID {
				return nil, &IndexError{Code: IndexCrossQuestionLink, QuestionID: question.ID, ForecastID: forecast.ID, Reference: reference}
			}
			if location.ForecastIndex >= forecastPosition {
				return nil, &IndexError{Code: IndexForwardLink, QuestionID: question.ID, ForecastID: forecast.ID, Reference: reference}
			}
			index.Supersedes[forecast.ID] = reference
			index.SupersededBy[reference] = append(index.SupersededBy[reference], forecast.ID)
		}
	}
	for _, values := range []map[Slug][]Slug{index.PlatformQuestionIDs, index.PlatformForecastIDs, index.SupersededBy} {
		for key := range values {
			sort.Slice(values[key], func(i, j int) bool { return values[key][i] < values[key][j] })
		}
	}
	return index, nil
}

func (i *Index) Question(id Slug) (int, bool) {
	if i == nil {
		return 0, false
	}
	position, ok := i.QuestionPositions[id]
	return position, ok
}

func (i *Index) Revision(questionID, revisionID Slug) (RevisionLocation, bool) {
	if i == nil {
		return RevisionLocation{}, false
	}
	questionPosition, ok := i.QuestionPositions[questionID]
	if !ok {
		return RevisionLocation{}, false
	}
	revisionPosition, ok := i.QuestionRevisionPositions[questionID][revisionID]
	if !ok {
		return RevisionLocation{}, false
	}
	return RevisionLocation{QuestionIndex: questionPosition, RevisionIndex: revisionPosition, QuestionID: questionID, RevisionID: revisionID}, true
}

func (i *Index) Forecast(id Slug) (ForecastLocation, bool) {
	if i == nil {
		return ForecastLocation{}, false
	}
	location, ok := i.ForecastLocations[id]
	return location, ok
}

func relationshipID(value Relationship) Slug {
	if value.GroupMembership != nil {
		return value.GroupMembership.ID
	}
	if value.Conditional != nil {
		return value.Conditional.ID
	}
	return ""
}

func appendUnique(index map[Slug][]Slug, key, value Slug) {
	for _, existing := range index[key] {
		if existing == value {
			return
		}
	}
	index[key] = append(index[key], value)
}
