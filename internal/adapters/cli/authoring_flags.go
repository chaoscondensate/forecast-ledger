package cli

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	urfavecli "github.com/urfave/cli/v3"
)

type authoringFieldRoute struct{ Field, Route, Class, Rationale string }
type authoringCommandRoute struct {
	Path      string
	Schema    service.InputSchemaName
	Protected bool
	Fields    []authoringFieldRoute
}

var authoringInventory = []authoringCommandRoute{
	{Path: "init", Schema: service.InputSchemaInit, Fields: routes("title=--title", "description=--description", "created_at=--created-at", "question=--question-*")},
	{Path: "ledger update", Schema: service.InputSchemaRootMetadata, Fields: routes("title=--title/--clear-title", "description=--description/--clear-description", "default_timezone=--timezone", "forecaster=--forecaster-*")},
	{Path: "platform add", Schema: service.InputSchemaPlatformCreate, Fields: routes("name=--name", "kind=--kind", "url=--url", "account=--account-*")},
	{Path: "platform update", Schema: service.InputSchemaPlatformPatch, Fields: routes("name=--name", "kind=--kind", "url=--url/--clear-url", "account=--account-*/--clear-account")},
	{Path: "question add", Schema: service.InputSchemaQuestionAdd, Fields: routes("revision=--revision-* and domain flags", "tags=--tag", "notes=--notes", "initial_forecast=--initial-*")},
	{Path: "question update", Schema: service.InputSchemaQuestionPatch, Fields: routes("tags=--tag/--clear-tags", "notes=--notes/--clear-notes", "status=--status")},
	{Path: "question resolve", Schema: service.InputSchemaResolution, Fields: routes("question_revision_id=--question-revision", "outcome=--outcome/--outcome-boolean", "outcome_known_at=--outcome-known-at", "sources=--source")},
	{Path: "forecast add", Schema: service.InputSchemaForecastCreate, Fields: routes("question_revision_id=--question-revision", "representations=representation flags", "forecasted_at=--forecasted-at", "provenance=--provenance-*")},
	{Path: "forecast seal", Schema: service.InputSchemaForecastSealPrivate, Protected: true, Fields: append(routes("question_revision_id=--question-revision", "forecasted_at=--forecasted-at", "provenance=--provenance-*"), protectedRoutes("representations=protected --secret-input", "rationale=protected --secret-input", "key_factors=protected --secret-input", "comment=protected --secret-input")...)},
	{Path: "forecast key-hint update", Schema: service.InputSchemaKeyHintUpdate, Fields: routes("key_hint=--key-hint")},
}

func routes(values ...string) []authoringFieldRoute {
	result := make([]authoringFieldRoute, 0, len(values))
	for _, value := range values {
		field, route, _ := strings.Cut(value, "=")
		result = append(result, authoringFieldRoute{Field: field, Route: route, Class: "public"})
	}
	return result
}
func protectedRoutes(values ...string) []authoringFieldRoute {
	result := routes(values...)
	for i := range result {
		result[i].Class = "secret"
	}
	return result
}
func serviceOnlyRoutes(values ...string) []authoringFieldRoute {
	result := []authoringFieldRoute{}
	for _, value := range values {
		field, rationale, _ := strings.Cut(value, "=")
		result = append(result, authoringFieldRoute{Field: field, Class: "service-only", Rationale: rationale})
	}
	return result
}

func requireDirectFlags(command *urfavecli.Command, names ...string) error {
	missing := []string{}
	for _, name := range names {
		if !command.IsSet(name) || strings.TrimSpace(command.String(name)) == "" {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) > 0 {
		return app.NewError(app.CodeUsage, strings.Join(missing, " and ")+" required", nil)
	}
	return nil
}

func parseCSVValues(flag string, values []string, minimum, maximum int) ([][]string, error) {
	result := make([][]string, 0, len(values))
	for position, value := range values {
		reader := csv.NewReader(strings.NewReader(value))
		reader.FieldsPerRecord = -1
		record, err := reader.Read()
		if err != nil {
			return nil, app.NewError(app.CodeUsage, fmt.Sprintf("--%s value %d is not valid CSV: %v", flag, position+1, err), err)
		}
		if _, err := reader.Read(); err != io.EOF {
			return nil, app.NewError(app.CodeUsage, fmt.Sprintf("--%s value %d must contain one CSV record", flag, position+1), err)
		}
		if len(record) < minimum || len(record) > maximum {
			return nil, app.NewError(app.CodeUsage, fmt.Sprintf("--%s value %d needs %d to %d CSV fields", flag, position+1, minimum, maximum), nil)
		}
		for i := range record {
			record[i] = strings.TrimSpace(record[i])
		}
		result = append(result, record)
	}
	return result, nil
}

func pointer[T any](value T) *T { return &value }
func optionalStringValue(command *urfavecli.Command, name string) *string {
	if !command.IsSet(name) {
		return nil
	}
	return pointer(command.String(name))
}
func optionalTimestampValue(command *urfavecli.Command, name string) *ledger.Timestamp {
	if !command.IsSet(name) {
		return nil
	}
	return pointer(ledger.Timestamp(command.String(name)))
}
func prefixed(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "-" + name
}

func rootAuthoringFlags() []urfavecli.Flag {
	return []urfavecli.Flag{
		&urfavecli.StringFlag{Name: "title", OnlyOnce: true, Usage: "Ledger title"}, &urfavecli.StringFlag{Name: "description", OnlyOnce: true, Usage: "Ledger description"}, &urfavecli.StringFlag{Name: "created-at", OnlyOnce: true, Usage: "Exact RFC 3339 ledger creation time"},
		&urfavecli.StringFlag{Name: "contact-email", OnlyOnce: true, Usage: "Forecaster contact email"}, &urfavecli.StringFlag{Name: "contact-website", OnlyOnce: true, Usage: "Forecaster website"}, &urfavecli.StringSliceFlag{Name: "profile", Usage: "service,url[,username]"}, &urfavecli.StringSliceFlag{Name: "member", Usage: "id,name[,role]"}, &urfavecli.StringSliceFlag{Name: "member-profile", Usage: "member-id,service,url[,username]"},
	}
}

func rootPatchFlags() []urfavecli.Flag {
	return append([]urfavecli.Flag{
		&urfavecli.StringFlag{Name: "title", OnlyOnce: true}, &urfavecli.BoolFlag{Name: "clear-title"}, &urfavecli.StringFlag{Name: "description", OnlyOnce: true}, &urfavecli.BoolFlag{Name: "clear-description"}, &urfavecli.StringFlag{Name: "timezone", OnlyOnce: true}, &urfavecli.StringFlag{Name: "forecaster-kind", OnlyOnce: true}, &urfavecli.StringFlag{Name: "forecaster-name", OnlyOnce: true}, &urfavecli.BoolFlag{Name: "clear-contact"}, &urfavecli.BoolFlag{Name: "clear-profiles"}, &urfavecli.BoolFlag{Name: "clear-members"},
	}, rootAuthoringFlags()[3:]...)
}

func platformCreateFlags() []urfavecli.Flag {
	return []urfavecli.Flag{&urfavecli.StringFlag{Name: "name", OnlyOnce: true}, &urfavecli.StringFlag{Name: "kind", OnlyOnce: true}, &urfavecli.StringFlag{Name: "url", OnlyOnce: true}, &urfavecli.StringFlag{Name: "account-username", OnlyOnce: true}, &urfavecli.StringFlag{Name: "account-user-id", OnlyOnce: true}, &urfavecli.StringFlag{Name: "account-profile-url", OnlyOnce: true}}
}
func platformPatchFlags() []urfavecli.Flag {
	return append(platformCreateFlags(), &urfavecli.BoolFlag{Name: "clear-url"}, &urfavecli.BoolFlag{Name: "clear-account"}, &urfavecli.BoolFlag{Name: "clear-account-username"}, &urfavecli.BoolFlag{Name: "clear-account-user-id"}, &urfavecli.BoolFlag{Name: "clear-account-profile-url"})
}

func domainFlags(prefix string) []urfavecli.Flag {
	n := func(v string) string { return prefixed(prefix, v) }
	return []urfavecli.Flag{
		&urfavecli.StringFlag{Name: n("revision-id"), OnlyOnce: true, Usage: "Stable question revision ID"}, &urfavecli.StringFlag{Name: n("effective-at"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("revision-recorded-at"), OnlyOnce: true},
		&urfavecli.StringFlag{Name: n("title"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("resolution-criteria"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("opens-at"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("expected-resolution-at"), OnlyOnce: true},
		&urfavecli.StringFlag{Name: n("outcome-kind"), OnlyOnce: true, Usage: "binary, categorical, ordinal, numeric, date, or datetime"},
		&urfavecli.StringFlag{Name: n("option-set-id"), OnlyOnce: true}, &urfavecli.IntFlag{Name: n("option-set-version"), OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: n("option"), Usage: "id,label[,description]"},
		&urfavecli.StringFlag{Name: n("lower-bound"), OnlyOnce: true}, &urfavecli.BoolFlag{Name: n("lower-exclusive")}, &urfavecli.StringFlag{Name: n("upper-bound"), OnlyOnce: true}, &urfavecli.BoolFlag{Name: n("upper-exclusive")},
		&urfavecli.StringFlag{Name: n("values-kind"), OnlyOnce: true, Usage: "continuous, step, or allowed_values"}, &urfavecli.StringFlag{Name: n("step"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("origin"), OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: n("allowed-value")},
		&urfavecli.StringFlag{Name: n("scale"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("unit-name"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("unit-symbol"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("unit-ucum-code"), OnlyOnce: true},
		&urfavecli.StringSliceFlag{Name: n("bin-set"), Usage: "id,version"}, &urfavecli.StringSliceFlag{Name: n("bin"), Usage: "set-id,version,bin-id,label,lower,upper,lower-inclusive,upper-inclusive"},
	}
}

func provenanceFlags(prefix string) []urfavecli.Flag {
	n := func(v string) string { return prefixed(prefix, v) }
	return []urfavecli.Flag{
		&urfavecli.StringFlag{Name: n("provenance-platform"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("remote-object-id"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("remote-object-version"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("source-url"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("source-created-at"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("source-updated-at"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("retrieved-at"), OnlyOnce: true},
		&urfavecli.StringFlag{Name: n("snapshot-path"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("snapshot-media-type"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("snapshot-sha256"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("importer-name"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("importer-version"), OnlyOnce: true},
	}
}

func representationFlags(prefix string) []urfavecli.Flag {
	n := func(v string) string { return prefixed(prefix, v) }
	return []urfavecli.Flag{
		&urfavecli.StringFlag{Name: n("probability"), OnlyOnce: true, Usage: "Exact probability string"}, &urfavecli.BoolFlag{Name: n("probability-outcome"), Usage: "Probability is for outcome true"},
		&urfavecli.StringFlag{Name: n("pmf-set"), OnlyOnce: true, Usage: "option-set-id,version"}, &urfavecli.StringSliceFlag{Name: n("pmf"), Usage: "option-id,probability"},
		&urfavecli.StringFlag{Name: n("binned-pmf-set"), OnlyOnce: true, Usage: "bin-set-id,version"}, &urfavecli.StringSliceFlag{Name: n("bin-probability"), Usage: "bin-id,probability"}, &urfavecli.StringFlag{Name: n("left-tail-probability"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("right-tail-probability"), OnlyOnce: true},
		&urfavecli.StringFlag{Name: n("quantile-interpolation"), OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: n("quantile"), Usage: "level,value"},
		&urfavecli.StringFlag{Name: n("cdf-interpolation"), OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: n("cdf"), Usage: "value,probability"}, &urfavecli.StringFlag{Name: n("cdf-left-tail"), OnlyOnce: true}, &urfavecli.StringFlag{Name: n("cdf-right-tail"), OnlyOnce: true},
		&urfavecli.StringFlag{Name: n("point"), OnlyOnce: true, Usage: "statistic,value"}, &urfavecli.StringSliceFlag{Name: n("credible-interval"), Usage: "coverage,kind,lower,upper"},
	}
}

func initialForecastFlags() []urfavecli.Flag {
	flags := []urfavecli.Flag{&urfavecli.StringFlag{Name: "initial-forecast", OnlyOnce: true}, &urfavecli.StringFlag{Name: "initial-visibility", OnlyOnce: true, Value: "public"}, &urfavecli.StringFlag{Name: "initial-forecasted-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "initial-recorded-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "initial-rationale", OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: "initial-key-factor"}, &urfavecli.StringFlag{Name: "initial-comment", OnlyOnce: true}, &urfavecli.StringFlag{Name: "initial-public-note", OnlyOnce: true}, &urfavecli.StringFlag{Name: "initial-secret-input", OnlyOnce: true, TakesFile: true}}
	flags = append(flags, representationFlags("initial")...)
	return append(flags, provenanceFlags("initial")...)
}

func questionCreateFlags(includeInitial bool) []urfavecli.Flag {
	flags := domainFlags("")
	flags = append(flags, provenanceFlags("revision")...)
	flags = append(flags, &urfavecli.StringFlag{Name: "created-at", OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: "tag"}, &urfavecli.StringFlag{Name: "notes", OnlyOnce: true})
	if includeInitial {
		flags = append(flags, initialForecastFlags()...)
	}
	return flags
}
func questionPatchFlags() []urfavecli.Flag {
	return []urfavecli.Flag{&urfavecli.StringSliceFlag{Name: "tag"}, &urfavecli.BoolFlag{Name: "clear-tags"}, &urfavecli.StringFlag{Name: "notes", OnlyOnce: true}, &urfavecli.BoolFlag{Name: "clear-notes"}, &urfavecli.StringFlag{Name: "status", OnlyOnce: true}}
}
func forecastCreateFlags() []urfavecli.Flag {
	flags := []urfavecli.Flag{&urfavecli.StringFlag{Name: "question-revision", OnlyOnce: true}, &urfavecli.StringFlag{Name: "forecasted-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "recorded-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "rationale", OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: "key-factor"}, &urfavecli.StringFlag{Name: "comment", OnlyOnce: true}, &urfavecli.StringFlag{Name: "public-note", OnlyOnce: true}, &urfavecli.StringFlag{Name: "supersedes-forecast", OnlyOnce: true}}
	flags = append(flags, representationFlags("")...)
	return append(flags, provenanceFlags("")...)
}
func forecastSealPublicFlags() []urfavecli.Flag {
	flags := []urfavecli.Flag{&urfavecli.StringFlag{Name: "question-revision", OnlyOnce: true}, &urfavecli.StringFlag{Name: "secret-input", OnlyOnce: true, TakesFile: true}, &urfavecli.StringFlag{Name: "forecasted-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "recorded-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "public-note", OnlyOnce: true}, &urfavecli.StringFlag{Name: "supersedes-forecast", OnlyOnce: true}}
	return append(flags, provenanceFlags("")...)
}

func initNestedFlags() []urfavecli.Flag {
	flags := []urfavecli.Flag{&urfavecli.StringSliceFlag{Name: "initial-platform"}, &urfavecli.StringFlag{Name: "question", OnlyOnce: true}, &urfavecli.StringFlag{Name: "question-created-at", OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: "question-tag"}, &urfavecli.StringFlag{Name: "question-notes", OnlyOnce: true}}
	flags = append(flags, domainFlags("question")...)
	flags = append(flags, provenanceFlags("question-revision")...)
	return append(flags, initialForecastFlags()...)
}

func lifecycleFlags(resolution bool) []urfavecli.Flag {
	flags := []urfavecli.Flag{&urfavecli.StringFlag{Name: "recorded-at", OnlyOnce: true}, &urfavecli.StringSliceFlag{Name: "source", Usage: "title,url,retrieved-at[,publisher[,published-at[,sha256]]]"}}
	if resolution {
		return append([]urfavecli.Flag{&urfavecli.StringFlag{Name: "question-revision", OnlyOnce: true}, &urfavecli.StringFlag{Name: "outcome", OnlyOnce: true}, &urfavecli.BoolFlag{Name: "outcome-boolean"}, &urfavecli.StringFlag{Name: "outcome-known-at", OnlyOnce: true}, &urfavecli.StringFlag{Name: "notes", OnlyOnce: true}}, flags...)
	}
	return append([]urfavecli.Flag{&urfavecli.StringFlag{Name: "reason", OnlyOnce: true}}, flags...)
}

func buildProfiles(command *urfavecli.Command, profileFlag, memberFlag, memberProfileFlag string) (*[]ledger.Profile, *[]ledger.Member, error) {
	profileRows, err := parseCSVValues(profileFlag, command.StringSlice(profileFlag), 2, 3)
	if err != nil {
		return nil, nil, err
	}
	profiles := []ledger.Profile{}
	for _, row := range profileRows {
		value := ledger.Profile{Service: row[0], URL: row[1]}
		if len(row) > 2 && row[2] != "" {
			value.Username = pointer(row[2])
		}
		profiles = append(profiles, value)
	}
	memberRows, err := parseCSVValues(memberFlag, command.StringSlice(memberFlag), 2, 3)
	if err != nil {
		return nil, nil, err
	}
	members := []ledger.Member{}
	positions := map[ledger.Slug]int{}
	for _, row := range memberRows {
		value := ledger.Member{ID: ledger.Slug(row[0]), Name: row[1]}
		if len(row) > 2 && row[2] != "" {
			value.Role = pointer(row[2])
		}
		positions[value.ID] = len(members)
		members = append(members, value)
	}
	memberProfileRows, err := parseCSVValues(memberProfileFlag, command.StringSlice(memberProfileFlag), 3, 4)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range memberProfileRows {
		position, ok := positions[ledger.Slug(row[0])]
		if !ok {
			return nil, nil, app.NewError(app.CodeUsage, "--member-profile names a missing member", nil)
		}
		value := ledger.Profile{Service: row[1], URL: row[2]}
		if len(row) > 3 && row[3] != "" {
			value.Username = pointer(row[3])
		}
		if members[position].Profiles == nil {
			members[position].Profiles = pointer([]ledger.Profile{})
		}
		*members[position].Profiles = append(*members[position].Profiles, value)
	}
	var pp *[]ledger.Profile
	if command.IsSet(profileFlag) {
		pp = &profiles
	}
	var mp *[]ledger.Member
	if command.IsSet(memberFlag) || command.IsSet(memberProfileFlag) {
		mp = &members
	}
	return pp, mp, nil
}

func buildContact(command *urfavecli.Command) *ledger.Contact {
	if !command.IsSet("contact-email") && !command.IsSet("contact-website") {
		return nil
	}
	return &ledger.Contact{Email: optionalStringValue(command, "contact-email"), Website: optionalStringValue(command, "contact-website")}
}
func buildPlatformAccount(command *urfavecli.Command) *ledger.PlatformAccount {
	if !command.IsSet("account-username") && !command.IsSet("account-user-id") && !command.IsSet("account-profile-url") {
		return nil
	}
	return &ledger.PlatformAccount{Username: optionalStringValue(command, "account-username"), UserID: optionalStringValue(command, "account-user-id"), ProfileURL: optionalStringValue(command, "account-profile-url")}
}

func patchString(command *urfavecli.Command, setter, clearer string) (service.Optional[string], error) {
	if command.IsSet(setter) && command.Bool(clearer) {
		return service.Optional[string]{}, app.NewError(app.CodeUsage, "--"+setter+" cannot be combined with --"+clearer, nil)
	}
	if command.Bool(clearer) {
		return service.Optional[string]{Set: true, Null: true}, nil
	}
	if command.IsSet(setter) {
		return service.Optional[string]{Set: true, Value: command.String(setter)}, nil
	}
	return service.Optional[string]{}, nil
}

func buildRootPatchInput(command *urfavecli.Command) (service.RootMetadataPatchInput, error) {
	input := service.RootMetadataPatchInput{}
	var err error
	input.Title, err = patchString(command, "title", "clear-title")
	if err != nil {
		return input, err
	}
	input.Description, err = patchString(command, "description", "clear-description")
	if err != nil {
		return input, err
	}
	if command.IsSet("timezone") {
		input.DefaultTimezone = service.Optional[string]{Set: true, Value: command.String("timezone")}
	}
	var forecaster service.ForecasterMetadataPatchInput
	if command.IsSet("forecaster-kind") {
		forecaster.Kind = service.Optional[ledger.ForecasterKind]{Set: true, Value: ledger.ForecasterKind(command.String("forecaster-kind"))}
	}
	if command.IsSet("forecaster-name") {
		forecaster.Name = service.Optional[string]{Set: true, Value: command.String("forecaster-name")}
	}
	if command.Bool("clear-contact") && (command.IsSet("contact-email") || command.IsSet("contact-website")) {
		return input, app.NewError(app.CodeUsage, "--clear-contact conflicts with contact flags", nil)
	}
	if command.Bool("clear-contact") {
		forecaster.Contact = service.Optional[ledger.Contact]{Set: true, Null: true}
	} else if value := buildContact(command); value != nil {
		forecaster.Contact = service.Optional[ledger.Contact]{Set: true, Value: *value}
	}
	profiles, members, err := buildProfiles(command, "profile", "member", "member-profile")
	if err != nil {
		return input, err
	}
	if command.Bool("clear-profiles") {
		forecaster.Profiles = service.Optional[[]ledger.Profile]{Set: true, Null: true}
	} else if profiles != nil {
		forecaster.Profiles = service.Optional[[]ledger.Profile]{Set: true, Value: *profiles}
	}
	if command.Bool("clear-members") {
		forecaster.Members = service.Optional[[]ledger.Member]{Set: true, Null: true}
	} else if members != nil {
		forecaster.Members = service.Optional[[]ledger.Member]{Set: true, Value: *members}
	}
	if forecaster.Kind.Set || forecaster.Name.Set || forecaster.Contact.Set || forecaster.Profiles.Set || forecaster.Members.Set {
		input.Forecaster = service.Optional[service.ForecasterMetadataPatchInput]{Set: true, Value: forecaster}
	}
	if !input.Title.Set && !input.Description.Set && !input.DefaultTimezone.Set && !input.Forecaster.Set {
		return input, app.NewError(app.CodeUsage, "at least one ledger authoring flag is required", nil)
	}
	return input, nil
}

func buildInitialPlatforms(command *urfavecli.Command) (map[ledger.Slug]ledger.Platform, error) {
	if !command.IsSet("initial-platform") {
		return nil, nil
	}
	rows, err := parseCSVValues("initial-platform", command.StringSlice("initial-platform"), 3, 7)
	if err != nil {
		return nil, err
	}
	result := map[ledger.Slug]ledger.Platform{}
	for _, row := range rows {
		id := ledger.Slug(row[0])
		if _, ok := result[id]; ok {
			return nil, app.NewError(app.CodeUsage, "duplicate initial platform", nil)
		}
		value := ledger.Platform{Name: row[1], Kind: ledger.PlatformKind(row[2])}
		if len(row) > 3 && row[3] != "" {
			value.URL = pointer(row[3])
		}
		if len(row) > 4 {
			account := &ledger.PlatformAccount{}
			if row[4] != "" {
				account.Username = pointer(row[4])
			}
			if len(row) > 5 && row[5] != "" {
				account.UserID = pointer(row[5])
			}
			if len(row) > 6 && row[6] != "" {
				account.ProfileURL = pointer(row[6])
			}
			if account.Username != nil || account.UserID != nil || account.ProfileURL != nil {
				value.Account = account
			}
		}
		result[id] = value
	}
	return result, nil
}

func buildPlatformCreateInput(command *urfavecli.Command) (service.PlatformCreateInput, error) {
	if err := requireDirectFlags(command, "name", "kind"); err != nil {
		return service.PlatformCreateInput{}, err
	}
	return service.PlatformCreateInput{Name: command.String("name"), Kind: ledger.PlatformKind(command.String("kind")), URL: optionalStringValue(command, "url"), Account: buildPlatformAccount(command)}, nil
}

func buildPlatformPatchInput(command *urfavecli.Command) (service.PlatformPatchInput, error) {
	input := service.PlatformPatchInput{}
	var err error
	input.URL, err = patchString(command, "url", "clear-url")
	if err != nil {
		return input, err
	}
	if command.IsSet("name") {
		input.Name = service.Optional[string]{Set: true, Value: command.String("name")}
	}
	if command.IsSet("kind") {
		input.Kind = service.Optional[ledger.PlatformKind]{Set: true, Value: ledger.PlatformKind(command.String("kind"))}
	}
	if command.Bool("clear-account") {
		input.Account = service.Optional[service.PlatformAccountPatchInput]{Set: true, Null: true}
	} else {
		var account service.PlatformAccountPatchInput
		account.Username, err = patchString(command, "account-username", "clear-account-username")
		if err != nil {
			return input, err
		}
		account.UserID, err = patchString(command, "account-user-id", "clear-account-user-id")
		if err != nil {
			return input, err
		}
		account.ProfileURL, err = patchString(command, "account-profile-url", "clear-account-profile-url")
		if err != nil {
			return input, err
		}
		if account.Username.Set || account.UserID.Set || account.ProfileURL.Set {
			input.Account = service.Optional[service.PlatformAccountPatchInput]{Set: true, Value: account}
		}
	}
	if !input.Name.Set && !input.Kind.Set && !input.URL.Set && !input.Account.Set {
		return input, app.NewError(app.CodeUsage, "at least one platform authoring flag is required", nil)
	}
	return input, nil
}

func parseBool(value, field string) (bool, error) {
	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, app.NewError(app.CodeUsage, field+" must be true or false", err)
	}
	return result, nil
}
func parseVersion(value, field string) (int, error) {
	result, err := strconv.Atoi(value)
	if err != nil || result < 1 {
		return 0, app.NewError(app.CodeUsage, field+" must be a positive integer", err)
	}
	return result, nil
}

func buildProvenance(command *urfavecli.Command, prefix string) (*ledger.Provenance, error) {
	n := func(v string) string { return prefixed(prefix, v) }
	fields := []string{"provenance-platform", "remote-object-id", "remote-object-version", "source-url", "source-created-at", "source-updated-at", "retrieved-at", "snapshot-path", "snapshot-media-type", "snapshot-sha256", "importer-name", "importer-version"}
	set := false
	for _, field := range fields {
		if command.IsSet(n(field)) {
			set = true
		}
	}
	if !set {
		return nil, nil
	}
	if err := requireDirectFlags(command, n("provenance-platform"), n("remote-object-id"), n("retrieved-at")); err != nil {
		return nil, err
	}
	result := &ledger.Provenance{Platform: ledger.Slug(command.String(n("provenance-platform"))), RemoteObjectID: command.String(n("remote-object-id")), RetrievedAt: ledger.Timestamp(command.String(n("retrieved-at"))), RemoteObjectVersion: optionalStringValue(command, n("remote-object-version")), URL: optionalStringValue(command, n("source-url")), SourceCreatedAt: optionalTimestampValue(command, n("source-created-at")), SourceUpdatedAt: optionalTimestampValue(command, n("source-updated-at"))}
	snapshotSet := command.IsSet(n("snapshot-path")) || command.IsSet(n("snapshot-media-type")) || command.IsSet(n("snapshot-sha256"))
	if snapshotSet {
		if err := requireDirectFlags(command, n("snapshot-path"), n("snapshot-media-type"), n("snapshot-sha256")); err != nil {
			return nil, err
		}
		result.Snapshot = &ledger.ArtifactSnapshot{ArtifactPath: ledger.RelativePath(command.String(n("snapshot-path"))), MediaType: command.String(n("snapshot-media-type")), Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(command.String(n("snapshot-sha256")))}}
	}
	importerSet := command.IsSet(n("importer-name")) || command.IsSet(n("importer-version"))
	if importerSet {
		if err := requireDirectFlags(command, n("importer-name"), n("importer-version")); err != nil {
			return nil, err
		}
		result.Importer = &ledger.Importer{Name: command.String(n("importer-name")), Version: command.String(n("importer-version"))}
	}
	return result, nil
}

func parseOptionSet(command *urfavecli.Command, prefix string) (ledger.OptionSet, error) {
	n := func(v string) string { return prefixed(prefix, v) }
	if err := requireDirectFlags(command, n("option-set-id")); err != nil {
		return ledger.OptionSet{}, err
	}
	version := command.Int(n("option-set-version"))
	if version < 1 {
		return ledger.OptionSet{}, app.NewError(app.CodeUsage, "--"+n("option-set-version")+" must be positive", nil)
	}
	rows, err := parseCSVValues(n("option"), command.StringSlice(n("option")), 2, 3)
	if err != nil {
		return ledger.OptionSet{}, err
	}
	if len(rows) < 2 {
		return ledger.OptionSet{}, app.NewError(app.CodeUsage, "at least two --"+n("option")+" values are required", nil)
	}
	options := make([]ledger.Option, 0, len(rows))
	for _, row := range rows {
		value := ledger.Option{ID: ledger.Slug(row[0]), Label: row[1]}
		if len(row) > 2 && row[2] != "" {
			value.Description = pointer(row[2])
		}
		options = append(options, value)
	}
	return ledger.OptionSet{ID: ledger.Slug(command.String(n("option-set-id"))), Version: version, Options: options}, nil
}

func numericBounds(command *urfavecli.Command, prefix string) *ledger.Bounds[ledger.Decimal] {
	n := func(v string) string { return prefixed(prefix, v) }
	if !command.IsSet(n("lower-bound")) && !command.IsSet(n("upper-bound")) {
		return nil
	}
	result := &ledger.Bounds[ledger.Decimal]{}
	if command.IsSet(n("lower-bound")) {
		result.Lower = &ledger.Bound[ledger.Decimal]{Value: ledger.Decimal(command.String(n("lower-bound"))), Inclusive: !command.Bool(n("lower-exclusive"))}
	}
	if command.IsSet(n("upper-bound")) {
		result.Upper = &ledger.Bound[ledger.Decimal]{Value: ledger.Decimal(command.String(n("upper-bound"))), Inclusive: !command.Bool(n("upper-exclusive"))}
	}
	return result
}
func dateBounds(command *urfavecli.Command, prefix string) *ledger.Bounds[ledger.Date] {
	n := func(v string) string { return prefixed(prefix, v) }
	if !command.IsSet(n("lower-bound")) && !command.IsSet(n("upper-bound")) {
		return nil
	}
	result := &ledger.Bounds[ledger.Date]{}
	if command.IsSet(n("lower-bound")) {
		result.Lower = &ledger.Bound[ledger.Date]{Value: ledger.Date(command.String(n("lower-bound"))), Inclusive: !command.Bool(n("lower-exclusive"))}
	}
	if command.IsSet(n("upper-bound")) {
		result.Upper = &ledger.Bound[ledger.Date]{Value: ledger.Date(command.String(n("upper-bound"))), Inclusive: !command.Bool(n("upper-exclusive"))}
	}
	return result
}
func datetimeBounds(command *urfavecli.Command, prefix string) *ledger.Bounds[ledger.Timestamp] {
	n := func(v string) string { return prefixed(prefix, v) }
	if !command.IsSet(n("lower-bound")) && !command.IsSet(n("upper-bound")) {
		return nil
	}
	result := &ledger.Bounds[ledger.Timestamp]{}
	if command.IsSet(n("lower-bound")) {
		result.Lower = &ledger.Bound[ledger.Timestamp]{Value: ledger.Timestamp(command.String(n("lower-bound"))), Inclusive: !command.Bool(n("lower-exclusive"))}
	}
	if command.IsSet(n("upper-bound")) {
		result.Upper = &ledger.Bound[ledger.Timestamp]{Value: ledger.Timestamp(command.String(n("upper-bound"))), Inclusive: !command.Bool(n("upper-exclusive"))}
	}
	return result
}

func decimalValues(command *urfavecli.Command, prefix string) (ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal], error) {
	n := func(v string) string { return prefixed(prefix, v) }
	kind := ledger.ValuesKind(command.String(n("values-kind")))
	switch kind {
	case ledger.ValuesContinuous:
		return ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal]{Continuous: &ledger.ContinuousValues{Kind: kind}}, nil
	case ledger.ValuesStep:
		if err := requireDirectFlags(command, n("step")); err != nil {
			return ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal]{}, err
		}
		value := &ledger.StepValues[ledger.Decimal, ledger.Decimal]{Kind: kind, Step: ledger.Decimal(command.String(n("step")))}
		if command.IsSet(n("origin")) {
			value.Origin = pointer(ledger.Decimal(command.String(n("origin"))))
		}
		return ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal]{Step: value}, nil
	case ledger.ValuesAllowed:
		raw := command.StringSlice(n("allowed-value"))
		values := make([]ledger.Decimal, len(raw))
		for i, v := range raw {
			values[i] = ledger.Decimal(v)
		}
		return ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal]{Allowed: &ledger.AllowedValues[ledger.Decimal]{Kind: kind, Values: values}}, nil
	default:
		return ledger.ValuesPolicy[ledger.Decimal, ledger.Decimal]{}, app.NewError(app.CodeUsage, "--"+n("values-kind")+" is required", nil)
	}
}
func dateValues(command *urfavecli.Command, prefix string) (ledger.ValuesPolicy[ledger.Date, ledger.DayStep], error) {
	n := func(v string) string { return prefixed(prefix, v) }
	kind := ledger.ValuesKind(command.String(n("values-kind")))
	switch kind {
	case ledger.ValuesContinuous:
		return ledger.ValuesPolicy[ledger.Date, ledger.DayStep]{Continuous: &ledger.ContinuousValues{Kind: kind}}, nil
	case ledger.ValuesStep:
		if err := requireDirectFlags(command, n("step")); err != nil {
			return ledger.ValuesPolicy[ledger.Date, ledger.DayStep]{}, err
		}
		value := &ledger.StepValues[ledger.Date, ledger.DayStep]{Kind: kind, Step: ledger.DayStep(command.String(n("step")))}
		if command.IsSet(n("origin")) {
			value.Origin = pointer(ledger.Date(command.String(n("origin"))))
		}
		return ledger.ValuesPolicy[ledger.Date, ledger.DayStep]{Step: value}, nil
	case ledger.ValuesAllowed:
		raw := command.StringSlice(n("allowed-value"))
		values := make([]ledger.Date, len(raw))
		for i, v := range raw {
			values[i] = ledger.Date(v)
		}
		return ledger.ValuesPolicy[ledger.Date, ledger.DayStep]{Allowed: &ledger.AllowedValues[ledger.Date]{Kind: kind, Values: values}}, nil
	default:
		return ledger.ValuesPolicy[ledger.Date, ledger.DayStep]{}, app.NewError(app.CodeUsage, "--"+n("values-kind")+" is required", nil)
	}
}
func datetimeValues(command *urfavecli.Command, prefix string) (ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep], error) {
	n := func(v string) string { return prefixed(prefix, v) }
	kind := ledger.ValuesKind(command.String(n("values-kind")))
	switch kind {
	case ledger.ValuesContinuous:
		return ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep]{Continuous: &ledger.ContinuousValues{Kind: kind}}, nil
	case ledger.ValuesStep:
		if err := requireDirectFlags(command, n("step")); err != nil {
			return ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep]{}, err
		}
		value := &ledger.StepValues[ledger.Timestamp, ledger.SecondStep]{Kind: kind, Step: ledger.SecondStep(command.String(n("step")))}
		if command.IsSet(n("origin")) {
			value.Origin = pointer(ledger.Timestamp(command.String(n("origin"))))
		}
		return ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep]{Step: value}, nil
	case ledger.ValuesAllowed:
		raw := command.StringSlice(n("allowed-value"))
		values := make([]ledger.Timestamp, len(raw))
		for i, v := range raw {
			values[i] = ledger.Timestamp(v)
		}
		return ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep]{Allowed: &ledger.AllowedValues[ledger.Timestamp]{Kind: kind, Values: values}}, nil
	default:
		return ledger.ValuesPolicy[ledger.Timestamp, ledger.SecondStep]{}, app.NewError(app.CodeUsage, "--"+n("values-kind")+" is required", nil)
	}
}

func buildDecimalBinSets(command *urfavecli.Command, prefix string) (*[]ledger.BinSet[ledger.Decimal], error) {
	n := func(v string) string { return prefixed(prefix, v) }
	sets, err := parseCSVValues(n("bin-set"), command.StringSlice(n("bin-set")), 2, 2)
	if err != nil {
		return nil, err
	}
	bins, err := parseCSVValues(n("bin"), command.StringSlice(n("bin")), 8, 8)
	if err != nil {
		return nil, err
	}
	if len(sets) == 0 && len(bins) == 0 {
		return nil, nil
	}
	result := make([]ledger.BinSet[ledger.Decimal], 0, len(sets))
	positions := map[string]int{}
	for _, row := range sets {
		version, err := parseVersion(row[1], "bin-set version")
		if err != nil {
			return nil, err
		}
		key := row[0] + ":" + row[1]
		positions[key] = len(result)
		result = append(result, ledger.BinSet[ledger.Decimal]{ID: ledger.Slug(row[0]), Version: version, Bins: []ledger.Bin[ledger.Decimal]{}})
	}
	for _, row := range bins {
		position, ok := positions[row[0]+":"+row[1]]
		if !ok {
			return nil, app.NewError(app.CodeUsage, "--"+n("bin")+" references an undeclared bin set", nil)
		}
		lowerInclusive, err := parseBool(row[6], "bin lower-inclusive")
		if err != nil {
			return nil, err
		}
		upperInclusive, err := parseBool(row[7], "bin upper-inclusive")
		if err != nil {
			return nil, err
		}
		value := ledger.Bin[ledger.Decimal]{ID: ledger.Slug(row[2]), Lower: ledger.Decimal(row[4]), Upper: ledger.Decimal(row[5]), LowerInclusive: lowerInclusive, UpperInclusive: upperInclusive}
		if row[3] != "" {
			value.Label = pointer(row[3])
		}
		result[position].Bins = append(result[position].Bins, value)
	}
	return &result, nil
}

func buildDateBinSets(command *urfavecli.Command, prefix string) (*[]ledger.BinSet[ledger.Date], error) {
	decimalSets, err := buildDecimalBinSets(command, prefix)
	if err != nil || decimalSets == nil {
		return nil, err
	}
	result := make([]ledger.BinSet[ledger.Date], len(*decimalSets))
	for i, set := range *decimalSets {
		result[i] = ledger.BinSet[ledger.Date]{ID: set.ID, Version: set.Version, Bins: make([]ledger.Bin[ledger.Date], len(set.Bins))}
		for j, bin := range set.Bins {
			result[i].Bins[j] = ledger.Bin[ledger.Date]{ID: bin.ID, Label: bin.Label, Lower: ledger.Date(bin.Lower), Upper: ledger.Date(bin.Upper), LowerInclusive: bin.LowerInclusive, UpperInclusive: bin.UpperInclusive}
		}
	}
	return &result, nil
}
func buildDatetimeBinSets(command *urfavecli.Command, prefix string) (*[]ledger.BinSet[ledger.Timestamp], error) {
	decimalSets, err := buildDecimalBinSets(command, prefix)
	if err != nil || decimalSets == nil {
		return nil, err
	}
	result := make([]ledger.BinSet[ledger.Timestamp], len(*decimalSets))
	for i, set := range *decimalSets {
		result[i] = ledger.BinSet[ledger.Timestamp]{ID: set.ID, Version: set.Version, Bins: make([]ledger.Bin[ledger.Timestamp], len(set.Bins))}
		for j, bin := range set.Bins {
			result[i].Bins[j] = ledger.Bin[ledger.Timestamp]{ID: bin.ID, Label: bin.Label, Lower: ledger.Timestamp(bin.Lower), Upper: ledger.Timestamp(bin.Upper), LowerInclusive: bin.LowerInclusive, UpperInclusive: bin.UpperInclusive}
		}
	}
	return &result, nil
}

func buildDomain(command *urfavecli.Command, prefix string) (ledger.OutcomeSpace, ledger.Domain, error) {
	n := func(v string) string { return prefixed(prefix, v) }
	kind := ledger.OutcomeKind(command.String(n("outcome-kind")))
	space := ledger.OutcomeSpace{Kind: kind}
	switch kind {
	case ledger.OutcomeBinary:
		return space, ledger.Domain{Binary: &ledger.BinaryDomain{Kind: kind}}, nil
	case ledger.OutcomeCategorical, ledger.OutcomeOrdinal:
		set, err := parseOptionSet(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		if kind == ledger.OutcomeCategorical {
			return space, ledger.Domain{Categorical: &ledger.CategoricalDomain{Kind: kind, OptionSet: set}}, nil
		}
		return space, ledger.Domain{Ordinal: &ledger.OrdinalDomain{Kind: kind, OptionSet: set}}, nil
	case ledger.OutcomeNumeric:
		values, err := decimalValues(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		bins, err := buildDecimalBinSets(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		value := &ledger.NumericDomain{Kind: kind, Bounds: numericBounds(command, prefix), Values: values, BinSets: bins}
		if command.IsSet(n("scale")) {
			scale := ledger.ElicitationScale(command.String(n("scale")))
			value.ElicitationScale = &scale
		}
		if command.IsSet(n("unit-name")) {
			value.Unit = &ledger.Unit{Name: command.String(n("unit-name")), Symbol: optionalStringValue(command, n("unit-symbol")), UCUMCode: optionalStringValue(command, n("unit-ucum-code"))}
		}
		return space, ledger.Domain{Numeric: value}, nil
	case ledger.OutcomeDate:
		values, err := dateValues(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		bins, err := buildDateBinSets(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		return space, ledger.Domain{Date: &ledger.DateDomain{Kind: kind, Bounds: dateBounds(command, prefix), Values: values, BinSets: bins}}, nil
	case ledger.OutcomeDatetime:
		values, err := datetimeValues(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		bins, err := buildDatetimeBinSets(command, prefix)
		if err != nil {
			return space, ledger.Domain{}, err
		}
		return space, ledger.Domain{Datetime: &ledger.DatetimeDomain{Kind: kind, Bounds: datetimeBounds(command, prefix), Values: values, BinSets: bins}}, nil
	default:
		return space, ledger.Domain{}, app.NewError(app.CodeUsage, "--"+n("outcome-kind")+" must be one of the six v2 kinds", nil)
	}
}

func buildRevisionInput(command *urfavecli.Command, prefix, provenancePrefix string) (service.RevisionInput, error) {
	n := func(v string) string { return prefixed(prefix, v) }
	if err := requireDirectFlags(command, n("revision-id"), n("title"), n("resolution-criteria"), n("expected-resolution-at"), n("outcome-kind")); err != nil {
		return service.RevisionInput{}, err
	}
	space, domain, err := buildDomain(command, prefix)
	if err != nil {
		return service.RevisionInput{}, err
	}
	provenance, err := buildProvenance(command, provenancePrefix)
	if err != nil {
		return service.RevisionInput{}, err
	}
	return service.RevisionInput{ID: ledger.Slug(command.String(n("revision-id"))), EffectiveAt: ledger.Timestamp(command.String(n("effective-at"))), RecordedAt: optionalTimestampValue(command, n("revision-recorded-at")), Title: command.String(n("title")), ResolutionCriteria: command.String(n("resolution-criteria")), ForecastingOpensAt: optionalTimestampValue(command, n("opens-at")), ExpectedResolutionAt: ledger.Timestamp(command.String(n("expected-resolution-at"))), OutcomeSpace: space, Domain: domain, Provenance: provenance}, nil
}

func parseSetRef(command *urfavecli.Command, name string) (ledger.Slug, int, error) {
	rows, err := parseCSVValues(name, []string{command.String(name)}, 2, 2)
	if err != nil {
		return "", 0, err
	}
	version, err := parseVersion(rows[0][1], name+" version")
	return ledger.Slug(rows[0][0]), version, err
}

func scalar(value string) ledger.ScalarValue { return ledger.ScalarValue{String: pointer(value)} }

func buildRepresentations(command *urfavecli.Command, prefix string) ([]ledger.ForecastRepresentation, error) {
	n := func(v string) string { return prefixed(prefix, v) }
	result := []ledger.ForecastRepresentation{}
	if command.IsSet(n("probability")) {
		outcome := true
		if command.IsSet(n("probability-outcome")) {
			outcome = command.Bool(n("probability-outcome"))
		}
		result = append(result, ledger.ForecastRepresentation{Probability: &ledger.ProbabilityRepresentation{Kind: ledger.RepresentationProbability, Outcome: outcome, Probability: ledger.Probability(command.String(n("probability")))}})
	}
	if command.IsSet(n("pmf-set")) || command.IsSet(n("pmf")) {
		if !command.IsSet(n("pmf-set")) {
			return nil, app.NewError(app.CodeUsage, "--"+n("pmf-set")+" required", nil)
		}
		id, version, err := parseSetRef(command, n("pmf-set"))
		if err != nil {
			return nil, err
		}
		rows, err := parseCSVValues(n("pmf"), command.StringSlice(n("pmf")), 2, 2)
		if err != nil {
			return nil, err
		}
		entries := make([]ledger.PMFEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, ledger.PMFEntry{OptionID: ledger.Slug(row[0]), Probability: ledger.Probability(row[1])})
		}
		result = append(result, ledger.ForecastRepresentation{PMF: &ledger.PMFRepresentation{Kind: ledger.RepresentationPMF, OptionSetRef: ledger.OptionSetRef{ID: id, Version: version}, Entries: entries}})
	}
	if command.IsSet(n("binned-pmf-set")) || command.IsSet(n("bin-probability")) {
		if !command.IsSet(n("binned-pmf-set")) {
			return nil, app.NewError(app.CodeUsage, "--"+n("binned-pmf-set")+" required", nil)
		}
		id, version, err := parseSetRef(command, n("binned-pmf-set"))
		if err != nil {
			return nil, err
		}
		rows, err := parseCSVValues(n("bin-probability"), command.StringSlice(n("bin-probability")), 2, 2)
		if err != nil {
			return nil, err
		}
		entries := make([]ledger.BinnedPMFEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, ledger.BinnedPMFEntry{BinID: ledger.Slug(row[0]), Probability: ledger.Probability(row[1])})
		}
		result = append(result, ledger.ForecastRepresentation{BinnedPMF: &ledger.BinnedPMFRepresentation{Kind: ledger.RepresentationBinnedPMF, BinSetRef: ledger.BinSetRef{ID: id, Version: version}, Entries: entries, LeftTailProbability: ledger.Probability(command.String(n("left-tail-probability"))), RightTailProbability: ledger.Probability(command.String(n("right-tail-probability")))}})
	}
	if command.IsSet(n("quantile")) {
		rows, err := parseCSVValues(n("quantile"), command.StringSlice(n("quantile")), 2, 2)
		if err != nil {
			return nil, err
		}
		points := make([]ledger.QuantilePoint, 0, len(rows))
		for _, row := range rows {
			points = append(points, ledger.QuantilePoint{Level: ledger.OpenProbability(row[0]), Value: scalar(row[1])})
		}
		result = append(result, ledger.ForecastRepresentation{Quantiles: &ledger.QuantilesRepresentation{Kind: ledger.RepresentationQuantiles, Interpolation: ledger.QuantileInterpolation(command.String(n("quantile-interpolation"))), Points: points}})
	}
	if command.IsSet(n("cdf")) {
		rows, err := parseCSVValues(n("cdf"), command.StringSlice(n("cdf")), 2, 2)
		if err != nil {
			return nil, err
		}
		points := make([]ledger.CDFPoint, 0, len(rows))
		for _, row := range rows {
			points = append(points, ledger.CDFPoint{Value: scalar(row[0]), Probability: ledger.Probability(row[1])})
		}
		result = append(result, ledger.ForecastRepresentation{CDF: &ledger.CDFRepresentation{Kind: ledger.RepresentationCDF, Interpolation: ledger.CDFInterpolation(command.String(n("cdf-interpolation"))), Points: points, LeftTailProbability: ledger.Probability(command.String(n("cdf-left-tail"))), RightTailProbability: ledger.Probability(command.String(n("cdf-right-tail")))}})
	}
	if command.IsSet(n("point")) {
		rows, err := parseCSVValues(n("point"), []string{command.String(n("point"))}, 2, 2)
		if err != nil {
			return nil, err
		}
		result = append(result, ledger.ForecastRepresentation{Point: &ledger.PointRepresentation{Kind: ledger.RepresentationPoint, Statistic: ledger.PointStatistic(rows[0][0]), Value: scalar(rows[0][1])}})
	}
	if command.IsSet(n("credible-interval")) {
		rows, err := parseCSVValues(n("credible-interval"), command.StringSlice(n("credible-interval")), 4, 4)
		if err != nil {
			return nil, err
		}
		intervals := make([]ledger.CredibleInterval, 0, len(rows))
		for _, row := range rows {
			intervals = append(intervals, ledger.CredibleInterval{Coverage: ledger.OpenProbability(row[0]), IntervalKind: ledger.IntervalKind(row[1]), Lower: scalar(row[2]), Upper: scalar(row[3])})
		}
		result = append(result, ledger.ForecastRepresentation{CredibleIntervals: &ledger.CredibleIntervalsRepresentation{Kind: ledger.RepresentationCredibleIntervals, Intervals: intervals}})
	}
	if len(result) == 0 {
		return nil, app.NewError(app.CodeUsage, "at least one v2 representation flag is required", nil)
	}
	return result, nil
}

func buildInitialForecast(ctx context.Context, command *urfavecli.Command, stdin io.Reader) (*service.InitialForecastInput, error) {
	if !command.IsSet("initial-forecast") {
		return nil, nil
	}
	visibility := ledger.ForecastVisibility(command.String("initial-visibility"))
	result := &service.InitialForecastInput{Visibility: visibility, ID: ledger.Slug(command.String("initial-forecast")), ForecastedAt: ledger.Timestamp(command.String("initial-forecasted-at")), RecordedAt: optionalTimestampValue(command, "initial-recorded-at"), PublicNote: optionalStringValue(command, "initial-public-note")}
	provenance, err := buildProvenance(command, "initial")
	if err != nil {
		return nil, err
	}
	result.Provenance = provenance
	if visibility == ledger.VisibilitySealed {
		if err := requireDirectFlags(command, "initial-secret-input"); err != nil {
			return nil, err
		}
		var private service.SealedForecastPrivateInput
		if err := decodePrivateOperationInputForArgument(ctx, command.String("initial-secret-input"), stdin, service.InputSchemaForecastSealPrivate, &private, "--initial-secret-input"); err != nil {
			return nil, err
		}
		result.Representations = private.Representations
		result.Rationale = private.Rationale
		result.KeyFactors = private.KeyFactors
		result.Comment = private.Comment
		return result, nil
	}
	if visibility != ledger.VisibilityPublic {
		return nil, app.NewError(app.CodeUsage, "--initial-visibility must be public or sealed", nil)
	}
	representations, err := buildRepresentations(command, "initial")
	if err != nil {
		return nil, err
	}
	result.Representations = representations
	result.Rationale = optionalStringValue(command, "initial-rationale")
	result.Comment = optionalStringValue(command, "initial-comment")
	if command.IsSet("initial-key-factor") {
		values := command.StringSlice("initial-key-factor")
		result.KeyFactors = &values
	}
	return result, nil
}

func buildInitInput(ctx context.Context, command *urfavecli.Command, stdin io.Reader) (service.InitInput, error) {
	profiles, members, err := buildProfiles(command, "profile", "member", "member-profile")
	if err != nil {
		return service.InitInput{}, err
	}
	platforms, err := buildInitialPlatforms(command)
	if err != nil {
		return service.InitInput{}, err
	}
	result := service.InitInput{Title: optionalStringValue(command, "title"), Description: optionalStringValue(command, "description"), CreatedAt: optionalTimestampValue(command, "created-at"), Contact: buildContact(command), Profiles: profiles, Members: members, Platforms: platforms}
	if !command.IsSet("question") {
		return result, nil
	}
	revision, err := buildRevisionInput(command, "question", "question-revision")
	if err != nil {
		return service.InitInput{}, err
	}
	question := &service.InitialQuestionInput{ID: ledger.Slug(command.String("question")), CreatedAt: optionalTimestampValue(command, "question-created-at"), Revision: revision, Notes: optionalStringValue(command, "question-notes")}
	if command.IsSet("question-tag") {
		raw := command.StringSlice("question-tag")
		tags := make([]ledger.Slug, len(raw))
		for i, v := range raw {
			tags[i] = ledger.Slug(v)
		}
		question.Tags = &tags
	}
	question.InitialForecast, err = buildInitialForecast(ctx, command, stdin)
	if err != nil {
		return service.InitInput{}, err
	}
	result.Question = question
	return result, nil
}

func buildQuestionAddInput(ctx context.Context, command *urfavecli.Command, stdin io.Reader) (service.QuestionAddInput, error) {
	revision, err := buildRevisionInput(command, "", "revision")
	if err != nil {
		return service.QuestionAddInput{}, err
	}
	result := service.QuestionAddInput{CreatedAt: optionalTimestampValue(command, "created-at"), Revision: revision, Notes: optionalStringValue(command, "notes")}
	if command.IsSet("tag") {
		raw := command.StringSlice("tag")
		tags := make([]ledger.Slug, len(raw))
		for i, v := range raw {
			tags[i] = ledger.Slug(v)
		}
		result.Tags = &tags
	}
	result.InitialForecast, err = buildInitialForecast(ctx, command, stdin)
	return result, err
}

func buildQuestionPatchInput(command *urfavecli.Command) (service.QuestionPatchInput, error) {
	result := service.QuestionPatchInput{}
	if command.IsSet("tag") && command.Bool("clear-tags") {
		return result, app.NewError(app.CodeUsage, "--tag conflicts with --clear-tags", nil)
	}
	if command.Bool("clear-tags") {
		result.Tags = service.Optional[[]ledger.Slug]{Set: true, Null: true}
	} else if command.IsSet("tag") {
		raw := command.StringSlice("tag")
		tags := make([]ledger.Slug, len(raw))
		for i, v := range raw {
			tags[i] = ledger.Slug(v)
		}
		result.Tags = service.Optional[[]ledger.Slug]{Set: true, Value: tags}
	}
	var err error
	result.Notes, err = patchString(command, "notes", "clear-notes")
	if err != nil {
		return result, err
	}
	if command.IsSet("status") {
		result.Status = service.Optional[ledger.QuestionStatus]{Set: true, Value: ledger.QuestionStatus(command.String("status"))}
	}
	if !result.Tags.Set && !result.Notes.Set && !result.Status.Set {
		return result, app.NewError(app.CodeUsage, "at least one question metadata flag is required", nil)
	}
	return result, nil
}

func buildForecastCreateInput(command *urfavecli.Command) (service.ForecastCreateInput, error) {
	if err := requireDirectFlags(command, "question-revision"); err != nil {
		return service.ForecastCreateInput{}, err
	}
	representations, err := buildRepresentations(command, "")
	if err != nil {
		return service.ForecastCreateInput{}, err
	}
	provenance, err := buildProvenance(command, "")
	if err != nil {
		return service.ForecastCreateInput{}, err
	}
	result := service.ForecastCreateInput{QuestionRevisionID: ledger.Slug(command.String("question-revision")), ForecastedAt: ledger.Timestamp(command.String("forecasted-at")), RecordedAt: optionalTimestampValue(command, "recorded-at"), Representations: representations, Rationale: optionalStringValue(command, "rationale"), Comment: optionalStringValue(command, "comment"), PublicNote: optionalStringValue(command, "public-note"), Provenance: provenance}
	if command.IsSet("key-factor") {
		values := command.StringSlice("key-factor")
		result.KeyFactors = &values
	}
	if command.IsSet("supersedes-forecast") {
		result.SupersedesForecastID = pointer(ledger.Slug(command.String("supersedes-forecast")))
	}
	return result, nil
}

func buildSealedForecastInput(ctx context.Context, command *urfavecli.Command, stdin io.Reader) (service.SealedForecastInput, error) {
	if err := requireDirectFlags(command, "question-revision", "secret-input"); err != nil {
		return service.SealedForecastInput{}, err
	}
	var private service.SealedForecastPrivateInput
	if err := decodePrivateOperationInputForArgument(ctx, command.String("secret-input"), stdin, service.InputSchemaForecastSealPrivate, &private, "--secret-input"); err != nil {
		return service.SealedForecastInput{}, err
	}
	provenance, err := buildProvenance(command, "")
	if err != nil {
		return service.SealedForecastInput{}, err
	}
	result := service.SealedForecastInput{QuestionRevisionID: ledger.Slug(command.String("question-revision")), ForecastedAt: ledger.Timestamp(command.String("forecasted-at")), RecordedAt: optionalTimestampValue(command, "recorded-at"), Representations: private.Representations, Rationale: private.Rationale, KeyFactors: private.KeyFactors, Comment: private.Comment, PublicNote: optionalStringValue(command, "public-note"), Provenance: provenance}
	if command.IsSet("supersedes-forecast") {
		result.SupersedesForecastID = pointer(ledger.Slug(command.String("supersedes-forecast")))
	}
	return result, nil
}

func parseSources(command *urfavecli.Command) ([]service.EvidenceSourceInput, error) {
	rows, err := parseCSVValues("source", command.StringSlice("source"), 3, 6)
	if err != nil {
		return nil, err
	}
	result := make([]service.EvidenceSourceInput, 0, len(rows))
	for _, row := range rows {
		value := service.EvidenceSourceInput{Title: row[0], URL: row[1], RetrievedAt: ledger.Timestamp(row[2])}
		if len(row) > 3 && row[3] != "" {
			value.Publisher = pointer(row[3])
		}
		if len(row) > 4 && row[4] != "" {
			value.PublishedAt = pointer(ledger.Timestamp(row[4]))
		}
		if len(row) > 5 && row[5] != "" {
			value.ContentDigest = &ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(row[5])}
		}
		result = append(result, value)
	}
	return result, nil
}

func buildResolutionInput(command *urfavecli.Command) (service.ResolutionInput, error) {
	if command.IsSet("outcome") == command.IsSet("outcome-boolean") {
		return service.ResolutionInput{}, app.NewError(app.CodeUsage, "use exactly one of --outcome or --outcome-boolean", nil)
	}
	if err := requireDirectFlags(command, "question-revision", "outcome-known-at"); err != nil {
		return service.ResolutionInput{}, err
	}
	sources, err := parseSources(command)
	if err != nil {
		return service.ResolutionInput{}, err
	}
	if len(sources) == 0 {
		return service.ResolutionInput{}, app.NewError(app.CodeUsage, "at least one --source is required", nil)
	}
	result := service.ResolutionInput{QuestionRevisionID: ledger.Slug(command.String("question-revision")), OutcomeKnownAt: ledger.Timestamp(command.String("outcome-known-at")), RecordedAt: optionalTimestampValue(command, "recorded-at"), Sources: sources, Notes: optionalStringValue(command, "notes")}
	if command.IsSet("outcome-boolean") {
		result.Outcome.Boolean = pointer(command.Bool("outcome-boolean"))
	} else {
		result.Outcome.String = pointer(command.String("outcome"))
	}
	return result, nil
}

func buildReasonInput(command *urfavecli.Command) (string, *ledger.Timestamp, []service.EvidenceSourceInput, error) {
	if err := requireDirectFlags(command, "reason"); err != nil {
		return "", nil, nil, err
	}
	sources, err := parseSources(command)
	if err != nil {
		return "", nil, nil, err
	}
	return command.String("reason"), optionalTimestampValue(command, "recorded-at"), sources, nil
}
