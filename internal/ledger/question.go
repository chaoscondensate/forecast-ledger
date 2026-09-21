package ledger

import (
	"encoding/json"
	"fmt"
)

type QuestionStatus string

const (
	QuestionOpen               QuestionStatus = "open"
	QuestionClosed             QuestionStatus = "closed"
	QuestionAwaitingResolution QuestionStatus = "awaiting_resolution"
	QuestionResolved           QuestionStatus = "resolved"
	QuestionAmbiguous          QuestionStatus = "ambiguous"
	QuestionVoid               QuestionStatus = "void"
	QuestionDisputed           QuestionStatus = "disputed"
	QuestionNotApplicable      QuestionStatus = "not_applicable"
)

type Question struct {
	ID                Slug               `json:"id" yaml:"id"`
	Status            QuestionStatus     `json:"status" yaml:"status"`
	CreatedAt         Timestamp          `json:"created_at" yaml:"created_at"`
	CurrentRevisionID Slug               `json:"current_revision_id" yaml:"current_revision_id"`
	Revisions         []QuestionRevision `json:"revisions" yaml:"revisions"`
	Tags              *[]Slug            `json:"tags,omitempty" yaml:"tags,omitempty"`
	Notes             *string            `json:"notes,omitempty" yaml:"notes,omitempty"`
	Forecasts         []Forecast         `json:"forecasts" yaml:"forecasts"`
	Resolution        *Resolution        `json:"resolution,omitempty" yaml:"resolution,omitempty"`
}

type ResolutionStatus string

const (
	ResolutionResolved      ResolutionStatus = "resolved"
	ResolutionAmbiguous     ResolutionStatus = "ambiguous"
	ResolutionVoid          ResolutionStatus = "void"
	ResolutionDisputed      ResolutionStatus = "disputed"
	ResolutionNotApplicable ResolutionStatus = "not_applicable"
)

type ResolutionSource struct {
	Title         string     `json:"title" yaml:"title"`
	Publisher     *string    `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	URL           string     `json:"url" yaml:"url"`
	PublishedAt   *Timestamp `json:"published_at,omitempty" yaml:"published_at,omitempty"`
	RetrievedAt   Timestamp  `json:"retrieved_at" yaml:"retrieved_at"`
	ContentDigest *Digest    `json:"content_digest,omitempty" yaml:"content_digest,omitempty"`
}

type ResolvedResolution struct {
	Status             ResolutionStatus   `json:"status" yaml:"status"`
	QuestionRevisionID Slug               `json:"question_revision_id" yaml:"question_revision_id"`
	Outcome            ScalarValue        `json:"outcome" yaml:"outcome"`
	OutcomeKnownAt     Timestamp          `json:"outcome_known_at" yaml:"outcome_known_at"`
	RecordedAt         Timestamp          `json:"recorded_at" yaml:"recorded_at"`
	Sources            []ResolutionSource `json:"sources" yaml:"sources"`
	Notes              *string            `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type UnresolvedResolution struct {
	Status     ResolutionStatus    `json:"status" yaml:"status"`
	Reason     string              `json:"reason" yaml:"reason"`
	RecordedAt Timestamp           `json:"recorded_at" yaml:"recorded_at"`
	Sources    *[]ResolutionSource `json:"sources,omitempty" yaml:"sources,omitempty"`
}

type NotApplicableResolution struct {
	Status         ResolutionStatus `json:"status" yaml:"status"`
	RelationshipID Slug             `json:"relationship_id" yaml:"relationship_id"`
	Reason         string           `json:"reason" yaml:"reason"`
	RecordedAt     Timestamp        `json:"recorded_at" yaml:"recorded_at"`
}

type Resolution struct {
	Resolved      *ResolvedResolution
	Unresolved    *UnresolvedResolution
	NotApplicable *NotApplicableResolution
}

func (v Resolution) MarshalJSON() ([]byte, error) {
	return marshalOne("resolution", v.Resolved, v.Unresolved, v.NotApplicable)
}

func (v *Resolution) UnmarshalJSON(data []byte) error {
	*v = Resolution{}
	var discriminator struct {
		Status ResolutionStatus `json:"status"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Status {
	case ResolutionResolved:
		v.Resolved = new(ResolvedResolution)
		return decodeClosed(data, v.Resolved)
	case ResolutionAmbiguous, ResolutionVoid, ResolutionDisputed:
		v.Unresolved = new(UnresolvedResolution)
		return decodeClosed(data, v.Unresolved)
	case ResolutionNotApplicable:
		v.NotApplicable = new(NotApplicableResolution)
		return decodeClosed(data, v.NotApplicable)
	default:
		return fmt.Errorf("unknown resolution status %q", discriminator.Status)
	}
}

type Group struct {
	ID          Slug    `json:"id" yaml:"id"`
	Title       string  `json:"title" yaml:"title"`
	Description *string `json:"description,omitempty" yaml:"description,omitempty"`
}

type RelationshipKind string

const (
	RelationshipGroupMembership RelationshipKind = "group_membership"
	RelationshipConditional     RelationshipKind = "conditional"
)

type GroupMembership struct {
	ID         Slug             `json:"id" yaml:"id"`
	Kind       RelationshipKind `json:"kind" yaml:"kind"`
	GroupID    Slug             `json:"group_id" yaml:"group_id"`
	QuestionID Slug             `json:"question_id" yaml:"question_id"`
}

type ConditionalRelationship struct {
	ID                       Slug             `json:"id" yaml:"id"`
	Kind                     RelationshipKind `json:"kind" yaml:"kind"`
	ParentQuestionID         Slug             `json:"parent_question_id" yaml:"parent_question_id"`
	ParentQuestionRevisionID Slug             `json:"parent_question_revision_id" yaml:"parent_question_revision_id"`
	ParentOutcome            ScalarValue      `json:"parent_outcome" yaml:"parent_outcome"`
	ChildQuestionID          Slug             `json:"child_question_id" yaml:"child_question_id"`
}

type Relationship struct {
	GroupMembership *GroupMembership
	Conditional     *ConditionalRelationship
}

func (v Relationship) MarshalJSON() ([]byte, error) {
	return marshalOne("relationship", v.GroupMembership, v.Conditional)
}

func (v *Relationship) UnmarshalJSON(data []byte) error {
	*v = Relationship{}
	var discriminator struct {
		Kind RelationshipKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Kind {
	case RelationshipGroupMembership:
		v.GroupMembership = new(GroupMembership)
		return decodeClosed(data, v.GroupMembership)
	case RelationshipConditional:
		v.Conditional = new(ConditionalRelationship)
		return decodeClosed(data, v.Conditional)
	default:
		return fmt.Errorf("unknown relationship kind %q", discriminator.Kind)
	}
}
