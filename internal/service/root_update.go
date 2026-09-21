package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

type RootMetadataUpdate struct {
	Ledger   *ledger.Ledger
	Patches  []document.PatchOperation
	Warnings []Warning
}

func BuildRootMetadataUpdate(model *ledger.Ledger, input RootMetadataPatchInput) (RootMetadataUpdate, error) {
	var result RootMetadataUpdate
	if model == nil {
		return result, app.NewError(app.CodeInternal, "ledger is nil", nil)
	}
	prospective, err := cloneLedger(model)
	if err != nil {
		return result, err
	}
	var patches []document.PatchOperation
	if input.Title.Set {
		if input.Title.Null {
			if model.Title != nil {
				prospective.Title = nil
				patches = append(patches, document.PatchOperation{Kind: document.PatchRemove, Pointer: "/title"})
			}
		} else {
			prospective.Title = cloneString(&input.Title.Value)
			patches = append(patches, optionalStringPatch("/title", model.Title, input.Title.Value))
		}
	}
	if input.Description.Set {
		if input.Description.Null {
			if model.Description != nil {
				prospective.Description = nil
				patches = append(patches, document.PatchOperation{Kind: document.PatchRemove, Pointer: "/description"})
			}
		} else {
			prospective.Description = cloneString(&input.Description.Value)
			patches = append(patches, optionalStringPatch("/description", model.Description, input.Description.Value))
		}
	}
	if input.DefaultTimezone.Set {
		if input.DefaultTimezone.Null {
			return result, invalidField("default_timezone", "default timezone cannot be removed")
		}
		if _, err := time.LoadLocation(input.DefaultTimezone.Value); err != nil {
			return result, invalidField("default_timezone", "default timezone must be a known IANA name")
		}
		prospective.DefaultTimezone = input.DefaultTimezone.Value
		patches = append(patches, document.PatchOperation{Kind: document.PatchReplace, Pointer: "/default_timezone", Value: input.DefaultTimezone.Value})
	}
	if input.Forecaster.Set {
		if input.Forecaster.Null {
			return result, invalidField("forecaster", "forecaster metadata cannot be removed")
		}
		forecasterPatches, err := applyForecasterPatch(&prospective.Forecaster, model.Forecaster, input.Forecaster.Value)
		if err != nil {
			return result, err
		}
		patches = append(patches, forecasterPatches...)
	}
	patches = removeNoopPatches(model, prospective, patches)
	if err := ValidateProspectiveLedgerModel(prospective); err != nil {
		return result, err
	}
	result.Ledger = prospective
	result.Patches = patches
	result.Warnings = []Warning{
		{Code: "forecaster.current_metadata", Message: "Forecast Ledger v2 stores current forecaster metadata and no internal identity history."},
		{Code: "authorship.not_proven", Message: "Changing forecaster metadata does not prove authorship."},
	}
	return result, nil
}

func applyForecasterPatch(target *ledger.Forecaster, original ledger.Forecaster, input ForecasterMetadataPatchInput) ([]document.PatchOperation, error) {
	if target == nil {
		return nil, app.NewError(app.CodeInternal, "forecaster target is nil", nil)
	}
	var patches []document.PatchOperation
	if input.Kind.Set {
		if input.Kind.Null || (input.Kind.Value != ledger.ForecasterIndividual && input.Kind.Value != ledger.ForecasterTeam) {
			return nil, invalidField("forecaster.kind", "forecaster kind must be individual or team")
		}
		target.Kind = input.Kind.Value
		patches = append(patches, document.PatchOperation{Kind: document.PatchReplace, Pointer: "/forecaster/kind", Value: string(input.Kind.Value)})
	}
	if input.Name.Set {
		if input.Name.Null || strings.TrimSpace(input.Name.Value) == "" {
			return nil, invalidField("forecaster.name", "forecaster name cannot be removed or empty")
		}
		target.Name = input.Name.Value
		patches = append(patches, document.PatchOperation{Kind: document.PatchReplace, Pointer: "/forecaster/name", Value: input.Name.Value})
	}
	if input.Contact.Set {
		if input.Contact.Null {
			if original.Contact != nil {
				target.Contact = nil
				patches = append(patches, document.PatchOperation{Kind: document.PatchRemove, Pointer: "/forecaster/contact"})
			}
		} else {
			if err := validateContact(&input.Contact.Value); err != nil {
				return nil, err
			}
			target.Contact = cloneContact(&input.Contact.Value)
			value, err := jsonPatchValue(input.Contact.Value)
			if err != nil {
				return nil, err
			}
			patches = append(patches, optionalValuePatch("/forecaster/contact", original.Contact != nil, value))
		}
	}
	if input.Profiles.Set {
		if input.Profiles.Null {
			if original.Profiles != nil {
				target.Profiles = nil
				patches = append(patches, document.PatchOperation{Kind: document.PatchRemove, Pointer: "/forecaster/profiles"})
			}
		} else {
			profiles := input.Profiles.Value
			if err := validateProfiles(&profiles, "forecaster.profiles"); err != nil {
				return nil, err
			}
			target.Profiles = cloneProfiles(&profiles)
			value, err := jsonPatchValue(input.Profiles.Value)
			if err != nil {
				return nil, err
			}
			patches = append(patches, optionalValuePatch("/forecaster/profiles", original.Profiles != nil, value))
		}
	}
	if input.Members.Set {
		if input.Members.Null {
			target.Members = nil
			if original.Members != nil {
				patches = append(patches, document.PatchOperation{Kind: document.PatchRemove, Pointer: "/forecaster/members"})
			}
		} else {
			members := input.Members.Value
			validated, err := validateForecasterMembers(ledger.ForecasterTeam, &members)
			if err != nil {
				return nil, err
			}
			target.Members = validated
			value, err := jsonPatchValue(input.Members.Value)
			if err != nil {
				return nil, err
			}
			patches = append(patches, optionalValuePatch("/forecaster/members", original.Members != nil, value))
		}
	}

	switch target.Kind {
	case ledger.ForecasterTeam:
		if target.Members == nil || len(*target.Members) < 2 {
			return nil, invalidField("forecaster.members", "team forecasters require at least two members in the same update")
		}
		if original.Kind == ledger.ForecasterIndividual && (!input.Kind.Set || !input.Members.Set || input.Members.Null) {
			return nil, invalidField("forecaster", "changing an individual to a team requires kind and at least two members in the same update")
		}
	case ledger.ForecasterIndividual:
		if target.Members != nil {
			return nil, invalidField("forecaster.members", "individual forecasters must not contain members")
		}
		if original.Kind == ledger.ForecasterTeam && (!input.Kind.Set || !input.Members.Set || !input.Members.Null) {
			return nil, invalidField("forecaster", "changing a team to an individual requires kind and members: null in the same update")
		}
	default:
		return nil, invalidField("forecaster.kind", "forecaster kind must be individual or team")
	}
	return patches, nil
}

func optionalStringPatch(pointer string, existing *string, value string) document.PatchOperation {
	return optionalValuePatch(pointer, existing != nil, value)
}

func optionalValuePatch(pointer string, exists bool, value any) document.PatchOperation {
	kind := document.PatchAdd
	if exists {
		kind = document.PatchReplace
	}
	return document.PatchOperation{Kind: kind, Pointer: pointer, Value: value}
}

func jsonPatchValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "metadata patch value cannot be encoded", err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(encoded), document.DefaultLimits)
	if err != nil {
		return nil, app.NewError(app.CodeInternal, "metadata patch value cannot be normalized", err)
	}
	return document.Ordered(parsed.Root), nil
}

// Full prospective validation handles equality; this filter keeps identical
// scalar replacements from reporting a mutation or rewriting source bytes.
func removeNoopPatches(before, after *ledger.Ledger, patches []document.PatchOperation) []document.PatchOperation {
	if before == nil || after == nil {
		return patches
	}
	result := patches[:0]
	for _, patch := range patches {
		skip := false
		switch patch.Pointer {
		case "/title":
			skip = equalStringPointers(before.Title, after.Title)
		case "/description":
			skip = equalStringPointers(before.Description, after.Description)
		case "/default_timezone":
			skip = before.DefaultTimezone == after.DefaultTimezone
		case "/forecaster/kind":
			skip = before.Forecaster.Kind == after.Forecaster.Kind
		case "/forecaster/name":
			skip = before.Forecaster.Name == after.Forecaster.Name
		}
		if !skip {
			result = append(result, patch)
		}
	}
	return result
}

func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func ChangedPointers(patches []document.PatchOperation) []string {
	result := make([]string, len(patches))
	for index, patch := range patches {
		result[index] = patch.Pointer
	}
	return result
}

func (update RootMetadataUpdate) String() string {
	return fmt.Sprintf("RootMetadataUpdate(%d patches)", len(update.Patches))
}
