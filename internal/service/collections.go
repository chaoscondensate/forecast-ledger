package service

import (
	"strconv"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

type CollectionMutation struct {
	Ledger  *ledger.Ledger
	Patches []document.PatchOperation
}

func ListGroups(model *ledger.Ledger) ([]ledger.Group, error) {
	if model == nil {
		return nil, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	if model.Groups == nil {
		return []ledger.Group{}, nil
	}
	return append([]ledger.Group(nil), (*model.Groups)...), nil
}
func ShowGroup(model *ledger.Ledger, id ledger.Slug) (ledger.Group, error) {
	groups, err := ListGroups(model)
	if err != nil {
		return ledger.Group{}, err
	}
	for _, group := range groups {
		if group.ID == id {
			return group, nil
		}
	}
	return ledger.Group{}, app.NewError(app.CodeNotFound, "group was not found", nil)
}

func BuildGroupAdd(model *ledger.Ledger, id ledger.Slug, input GroupCreateInput) (CollectionMutation, error) {
	if err := ValidateSlug(id, "group"); err != nil {
		return CollectionMutation{}, err
	}
	if strings.TrimSpace(input.Title) == "" {
		return CollectionMutation{}, invalidField("title", "group title must not be empty")
	}
	if _, err := ShowGroup(model, id); app.ErrorCodeOf(err) != app.CodeNotFound {
		return CollectionMutation{}, app.NewError(app.CodeConflict, "group ID already exists", nil)
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	group := ledger.Group{ID: id, Title: input.Title, Description: cloneString(input.Description)}
	var patch document.PatchOperation
	if prospective.Groups == nil {
		values := []ledger.Group{group}
		prospective.Groups = &values
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: "/groups", Value: values}
	} else {
		*prospective.Groups = append(*prospective.Groups, group)
		value, _ := jsonPatchValue(group)
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: "/groups/-", Value: value}
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return CollectionMutation{}, err
	}
	return CollectionMutation{Ledger: prospective, Patches: []document.PatchOperation{patch}}, nil
}

func BuildGroupUpdate(model *ledger.Ledger, id ledger.Slug, input GroupPatchInput) (CollectionMutation, error) {
	groups, err := ListGroups(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	position := -1
	for i, group := range groups {
		if group.ID == id {
			position = i
			break
		}
	}
	if position < 0 {
		return CollectionMutation{}, app.NewError(app.CodeNotFound, "group was not found", nil)
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	updated := &(*prospective.Groups)[position]
	base := "/groups/" + strconv.Itoa(position)
	patches := []document.PatchOperation{}
	if input.Title.Set {
		if input.Title.Null || strings.TrimSpace(input.Title.Value) == "" {
			return CollectionMutation{}, invalidField("title", "group title must not be empty")
		}
		if updated.Title != input.Title.Value {
			updated.Title = input.Title.Value
			patches = append(patches, replacePatch(base+"/title", updated.Title))
		}
	}
	if input.Description.Set {
		before := updated.Description != nil
		if input.Description.Null {
			updated.Description = nil
		} else {
			value := input.Description.Value
			updated.Description = &value
		}
		patches = append(patches, optionalFieldPatch(base+"/description", before, updated.Description))
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return CollectionMutation{}, err
	}
	return CollectionMutation{Ledger: prospective, Patches: patches}, nil
}

func BuildGroupRemove(model *ledger.Ledger, id ledger.Slug) (CollectionMutation, error) {
	groups, err := ListGroups(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	position := -1
	for i, group := range groups {
		if group.ID == id {
			position = i
			break
		}
	}
	if position < 0 {
		return CollectionMutation{}, app.NewError(app.CodeNotFound, "group was not found", nil)
	}
	if model.Relationships != nil {
		for _, relationship := range *model.Relationships {
			if relationship.GroupMembership != nil && relationship.GroupMembership.GroupID == id {
				return CollectionMutation{}, app.NewError(app.CodeConflict, "group is referenced by a relationship", nil)
			}
		}
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	values := append((*prospective.Groups)[:position], (*prospective.Groups)[position+1:]...)
	patch := document.PatchOperation{Kind: document.PatchRemove, Pointer: "/groups/" + strconv.Itoa(position)}
	if len(values) == 0 {
		prospective.Groups = nil
		patch = document.PatchOperation{Kind: document.PatchRemove, Pointer: "/groups"}
	} else {
		prospective.Groups = &values
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return CollectionMutation{}, err
	}
	return CollectionMutation{Ledger: prospective, Patches: []document.PatchOperation{patch}}, nil
}

func relationshipID(value ledger.Relationship) ledger.Slug {
	if value.GroupMembership != nil {
		return value.GroupMembership.ID
	}
	if value.Conditional != nil {
		return value.Conditional.ID
	}
	return ""
}
func ListRelationships(model *ledger.Ledger) ([]ledger.Relationship, error) {
	if model == nil {
		return nil, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	if model.Relationships == nil {
		return []ledger.Relationship{}, nil
	}
	return append([]ledger.Relationship(nil), (*model.Relationships)...), nil
}
func ShowRelationship(model *ledger.Ledger, id ledger.Slug) (ledger.Relationship, error) {
	values, err := ListRelationships(model)
	if err != nil {
		return ledger.Relationship{}, err
	}
	for _, value := range values {
		if relationshipID(value) == id {
			return value, nil
		}
	}
	return ledger.Relationship{}, app.NewError(app.CodeNotFound, "relationship was not found", nil)
}
func BuildRelationshipAdd(model *ledger.Ledger, input RelationshipInput) (CollectionMutation, error) {
	id := relationshipID(input.Relationship)
	if err := ValidateSlug(id, "relationship"); err != nil {
		return CollectionMutation{}, err
	}
	if _, err := ShowRelationship(model, id); app.ErrorCodeOf(err) != app.CodeNotFound {
		return CollectionMutation{}, app.NewError(app.CodeConflict, "relationship ID already exists", nil)
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	var patch document.PatchOperation
	if prospective.Relationships == nil {
		values := []ledger.Relationship{input.Relationship}
		prospective.Relationships = &values
		value, err := jsonPatchValue(values)
		if err != nil {
			return CollectionMutation{}, err
		}
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: "/relationships", Value: value}
	} else {
		*prospective.Relationships = append(*prospective.Relationships, input.Relationship)
		value, err := jsonPatchValue(input.Relationship)
		if err != nil {
			return CollectionMutation{}, err
		}
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: "/relationships/-", Value: value}
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return CollectionMutation{}, err
	}
	return CollectionMutation{Ledger: prospective, Patches: []document.PatchOperation{patch}}, nil
}
func BuildRelationshipRemove(model *ledger.Ledger, id ledger.Slug) (CollectionMutation, error) {
	values, err := ListRelationships(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	position := -1
	for i, value := range values {
		if relationshipID(value) == id {
			position = i
			break
		}
	}
	if position < 0 {
		return CollectionMutation{}, app.NewError(app.CodeNotFound, "relationship was not found", nil)
	}
	for _, question := range model.Questions {
		if question.Resolution != nil && question.Resolution.NotApplicable != nil && question.Resolution.NotApplicable.RelationshipID == id {
			return CollectionMutation{}, app.NewError(app.CodeConflict, "relationship is referenced by a not-applicable resolution", nil)
		}
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return CollectionMutation{}, err
	}
	remaining := append((*prospective.Relationships)[:position], (*prospective.Relationships)[position+1:]...)
	patch := document.PatchOperation{Kind: document.PatchRemove, Pointer: "/relationships/" + strconv.Itoa(position)}
	if len(remaining) == 0 {
		prospective.Relationships = nil
		patch = document.PatchOperation{Kind: document.PatchRemove, Pointer: "/relationships"}
	} else {
		prospective.Relationships = &remaining
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return CollectionMutation{}, err
	}
	return CollectionMutation{Ledger: prospective, Patches: []document.PatchOperation{patch}}, nil
}

func BuildForecastLifecycle(model *ledger.Ledger, questionID, forecastID ledger.Slug, eventType ledger.LifecycleEventType, input LifecycleInput, observedAt ledger.Timestamp) (ForecastMutation, error) {
	questionPosition, _, forecastPosition, forecast, err := selectForecast(model, questionID, forecastID)
	if err != nil {
		return ForecastMutation{}, err
	}
	if err := ValidateSlug(input.ID, "event.id"); err != nil {
		return ForecastMutation{}, err
	}
	effective := input.EffectiveAt
	if effective == "" {
		effective = observedAt
	}
	recorded := observedAt
	if input.RecordedAt != nil {
		recorded = *input.RecordedAt
	}
	event := ledger.LifecycleEvent{ID: input.ID, Type: eventType, EffectiveAt: effective, RecordedAt: recorded, Reason: cloneString(input.Reason), Provenance: input.Provenance}
	prospective, err := cloneLedger(model)
	if err != nil {
		return ForecastMutation{}, err
	}
	updated := &prospective.Questions[questionPosition].Forecasts[forecastPosition]
	base := "/questions/" + strconv.Itoa(questionPosition) + "/forecasts/" + strconv.Itoa(forecastPosition) + "/lifecycle_events"
	var patch document.PatchOperation
	if updated.LifecycleEvents == nil {
		values := []ledger.LifecycleEvent{event}
		updated.LifecycleEvents = &values
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: base, Value: values}
	} else {
		*updated.LifecycleEvents = append(*updated.LifecycleEvents, event)
		value, _ := jsonPatchValue(event)
		patch = document.PatchOperation{Kind: document.PatchAdd, Pointer: base + "/-", Value: value}
	}
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return ForecastMutation{}, err
	}
	_ = forecast
	return ForecastMutation{Ledger: prospective, Patches: []document.PatchOperation{patch}}, nil
}
