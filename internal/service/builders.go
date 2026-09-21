package service

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)

type InitRootRequest struct {
	LedgerID       ledger.Slug
	Timezone       string
	ForecasterID   ledger.Slug
	ForecasterName string
	ForecasterKind ledger.ForecasterKind
	Input          InitInput
}

func BuildLedgerRoot(request InitRootRequest, clock ObservationClock) (*ledger.Ledger, error) {
	observedAt, err := CaptureOperationTime(clock)
	if err != nil {
		return nil, err
	}
	return BuildLedgerRootAt(request, observedAt)
}

// CaptureOperationTime observes the clock exactly once for one operation.
// Callers can then apply the same observation to independent defaulted fields
// without deriving those fields from explicit business timestamps.
func CaptureOperationTime(clock ObservationClock) (ledger.Timestamp, error) {
	if clock == nil {
		return "", app.NewError(app.CodeInternal, "observation clock is not configured", nil)
	}
	return ledger.Timestamp(clock.Now().Format(time.RFC3339)), nil
}

func BuildLedgerRootAt(request InitRootRequest, observedAt ledger.Timestamp) (*ledger.Ledger, error) {
	if _, err := ParseTimestamp(observedAt, "operation_clock"); err != nil {
		return nil, err
	}
	if err := ValidateSlug(request.LedgerID, "ledger_id"); err != nil {
		return nil, err
	}
	if err := ValidateSlug(request.ForecasterID, "forecaster_id"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.ForecasterName) == "" {
		return nil, invalidField("forecaster_name", "forecaster name must not be empty")
	}
	if _, err := time.LoadLocation(request.Timezone); err != nil {
		return nil, invalidField("timezone", "timezone must be a known IANA name")
	}
	if request.ForecasterKind == "" {
		request.ForecasterKind = ledger.ForecasterIndividual
	}
	if request.ForecasterKind != ledger.ForecasterIndividual && request.ForecasterKind != ledger.ForecasterTeam {
		return nil, invalidField("forecaster_kind", "forecaster kind must be individual or team")
	}
	if err := validateContact(request.Input.Contact); err != nil {
		return nil, err
	}
	if err := validateProfiles(request.Input.Profiles, "profiles"); err != nil {
		return nil, err
	}
	members, err := validateForecasterMembers(request.ForecasterKind, request.Input.Members)
	if err != nil {
		return nil, err
	}
	createdAt, err := initialTimestamp(request.Input.CreatedAt, observedAt)
	if err != nil {
		return nil, err
	}
	platforms := make(map[ledger.Slug]ledger.Platform, len(request.Input.Platforms))
	for id, platform := range request.Input.Platforms {
		if err := ValidateSlug(id, "platforms"); err != nil {
			return nil, err
		}
		if err := ValidatePlatform(platform); err != nil {
			return nil, err
		}
		platforms[id] = clonePlatform(platform)
	}
	return &ledger.Ledger{
		SchemaVersion:   ledger.SchemaVersion(ledgerschema.Version),
		LedgerID:        request.LedgerID,
		Title:           cloneString(request.Input.Title),
		Description:     cloneString(request.Input.Description),
		CreatedAt:       createdAt,
		DefaultTimezone: request.Timezone,
		Forecaster: ledger.Forecaster{
			ID: request.ForecasterID, Kind: request.ForecasterKind, Name: request.ForecasterName,
			Contact: cloneContact(request.Input.Contact), Profiles: cloneProfiles(request.Input.Profiles), Members: members,
		},
		Platforms: platforms,
		Questions: []ledger.Question{},
	}, nil
}

func ValidateSlug(value ledger.Slug, field string) error {
	text := string(value)
	if len(text) < 1 || len(text) > 128 || !slugPattern.MatchString(text) {
		return invalidField(field, "ID must use lowercase letters, digits, dots, underscores, or hyphens without empty segments")
	}
	return nil
}

func ParseTimestamp(value ledger.Timestamp, field string) (time.Time, error) {
	text := string(value)
	if !timestampPattern.MatchString(text) {
		return time.Time{}, invalidField(field, "timestamp must be RFC 3339 with seconds and an explicit offset")
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, invalidField(field, "timestamp is not a valid calendar time")
	}
	return parsed, nil
}

func ValidateChronology(earlier ledger.Timestamp, earlierField string, later ledger.Timestamp, laterField string, allowEqual bool) error {
	left, err := ParseTimestamp(earlier, earlierField)
	if err != nil {
		return err
	}
	right, err := ParseTimestamp(later, laterField)
	if err != nil {
		return err
	}
	if left.After(right) || (!allowEqual && left.Equal(right)) {
		if allowEqual {
			return invalidField(laterField, fmt.Sprintf("%s must not be before %s", laterField, earlierField))
		}
		return invalidField(laterField, fmt.Sprintf("%s must be after %s", laterField, earlierField))
	}
	return nil
}

func ValidatePlatform(platform ledger.Platform) error {
	if strings.TrimSpace(platform.Name) == "" {
		return invalidField("platform.name", "platform name must not be empty")
	}
	switch platform.Kind {
	case ledger.PlatformScoringMarket, ledger.PlatformPrediction, ledger.PlatformSelfHosted, ledger.PlatformInternal, ledger.PlatformInformal:
	default:
		return invalidField("platform.kind", "platform kind is not supported")
	}
	if platform.URL != nil {
		if err := validateAbsoluteURI(*platform.URL, "platform.url"); err != nil {
			return err
		}
	}
	if platform.Account != nil {
		account := platform.Account
		if emptyOptional(account.Username) && emptyOptional(account.UserID) && emptyOptional(account.ProfileURL) {
			return invalidField("platform.account", "platform account must contain at least one non-empty field")
		}
		for field, value := range map[string]*string{"username": account.Username, "user_id": account.UserID} {
			if value != nil && strings.TrimSpace(*value) == "" {
				return invalidField("platform.account."+field, "account field must not be empty")
			}
		}
		if account.ProfileURL != nil {
			if err := validateAbsoluteURI(*account.ProfileURL, "platform.account.profile_url"); err != nil {
				return err
			}
		}
	}
	return nil
}

func initialTimestamp(explicit *ledger.Timestamp, observedAt ledger.Timestamp) (ledger.Timestamp, error) {
	if explicit != nil {
		if _, err := ParseTimestamp(*explicit, "created_at"); err != nil {
			return "", err
		}
		return *explicit, nil
	}
	if _, err := ParseTimestamp(observedAt, "operation_clock"); err != nil {
		return "", err
	}
	return observedAt, nil
}

func validateForecasterMembers(kind ledger.ForecasterKind, input *[]ledger.Member) (*[]ledger.Member, error) {
	if kind == ledger.ForecasterIndividual {
		if input != nil {
			return nil, invalidField("members", "individual forecasters must not contain members")
		}
		return nil, nil
	}
	if input == nil || len(*input) < 2 {
		return nil, invalidField("members", "team forecasters require at least two members")
	}
	seen := make(map[ledger.Slug]struct{}, len(*input))
	result := make([]ledger.Member, len(*input))
	for index, member := range *input {
		if err := ValidateSlug(member.ID, fmt.Sprintf("members.%d.id", index)); err != nil {
			return nil, err
		}
		if _, duplicate := seen[member.ID]; duplicate {
			return nil, invalidField("members", "team member IDs must be unique")
		}
		seen[member.ID] = struct{}{}
		if strings.TrimSpace(member.Name) == "" {
			return nil, invalidField(fmt.Sprintf("members.%d.name", index), "team member name must not be empty")
		}
		if member.Role != nil && strings.TrimSpace(*member.Role) == "" {
			return nil, invalidField(fmt.Sprintf("members.%d.role", index), "team member role must not be empty")
		}
		if err := validateProfiles(member.Profiles, fmt.Sprintf("members.%d.profiles", index)); err != nil {
			return nil, err
		}
		result[index] = member
		result[index].Role = cloneString(member.Role)
		result[index].Profiles = cloneProfiles(member.Profiles)
	}
	return &result, nil
}

func validateContact(contact *ledger.Contact) error {
	if contact == nil {
		return nil
	}
	if contact.Email != nil {
		address, err := mail.ParseAddress(*contact.Email)
		if err != nil || address.Address != *contact.Email {
			return invalidField("contact.email", "contact email is not valid")
		}
	}
	if contact.Website != nil {
		return validateAbsoluteURI(*contact.Website, "contact.website")
	}
	return nil
}

func validateProfiles(profiles *[]ledger.Profile, field string) error {
	if profiles == nil {
		return nil
	}
	for index, profile := range *profiles {
		if strings.TrimSpace(profile.Service) == "" {
			return invalidField(fmt.Sprintf("%s.%d.service", field, index), "profile service must not be empty")
		}
		if profile.Username != nil && strings.TrimSpace(*profile.Username) == "" {
			return invalidField(fmt.Sprintf("%s.%d.username", field, index), "profile username must not be empty")
		}
		if err := validateAbsoluteURI(profile.URL, fmt.Sprintf("%s.%d.url", field, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateAbsoluteURI(value, field string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || !parsed.IsAbs() || parsed.Scheme == "" {
		return invalidField(field, "value must be an absolute URI")
	}
	return nil
}

func emptyOptional(value *string) bool { return value == nil || strings.TrimSpace(*value) == "" }

func invalidField(field, message string) error {
	pointer := ""
	for _, token := range strings.Split(field, ".") {
		pointer += "/" + strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	issue := document.Diagnostic{
		Code: "semantic.invalid_field", Message: message,
		Location: document.SourceRef{Pointer: pointer},
	}
	return app.WithDetails(app.NewError(app.CodeInvalidData, message, nil), map[string]any{
		"field": field, "issues": []document.Diagnostic{issue},
	})
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTimestamp(value *ledger.Timestamp) *ledger.Timestamp {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneContact(value *ledger.Contact) *ledger.Contact {
	if value == nil {
		return nil
	}
	return &ledger.Contact{Email: cloneString(value.Email), Website: cloneString(value.Website)}
}

func cloneProfiles(value *[]ledger.Profile) *[]ledger.Profile {
	if value == nil {
		return nil
	}
	result := make([]ledger.Profile, len(*value))
	for index, profile := range *value {
		result[index] = profile
		result[index].Username = cloneString(profile.Username)
	}
	return &result
}

func clonePlatform(value ledger.Platform) ledger.Platform {
	result := value
	result.URL = cloneString(value.URL)
	if value.Account != nil {
		result.Account = &ledger.PlatformAccount{
			Username: cloneString(value.Account.Username), UserID: cloneString(value.Account.UserID), ProfileURL: cloneString(value.Account.ProfileURL),
		}
	}
	return result
}
