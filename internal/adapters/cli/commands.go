package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	mcpadapter "github.com/chaoscondensate/forecast-ledger/internal/adapters/mcp"
	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/buildinfo"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/presentation"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	urfavecli "github.com/urfave/cli/v3"
)

func NewCommand(stdin io.Reader, stdout, stderr io.Writer) *urfavecli.Command {
	return newCommandWithEffects(stdin, stdout, stderr, service.ProductionEffects())
}

func newCommandWithEffects(stdin io.Reader, stdout, stderr io.Writer, effects service.Effects) *urfavecli.Command {
	info := buildinfo.Current()
	root := &urfavecli.Command{
		Name:                      "forecast-ledger",
		Usage:                     "Create and verify portable forecast evidence",
		Description:               "Manage Forecast Ledger files without requiring Git or a hosted service.\n\nExamples:\n  forecast-ledger validate --file ledger.yaml\n  forecast-ledger status --file ledger.yaml\n  forecast-ledger completion bash",
		Version:                   info.Version,
		Suggest:                   true,
		EnableShellCompletion:     true,
		DisableSliceFlagSeparator: true,
		ConfigureShellCompletionCommand: func(command *urfavecli.Command) {
			command.Hidden = false
		},
		Reader:    stdin,
		Writer:    stdout,
		ErrWriter: stderr,
		Metadata:  map[string]any{"effects": effects},
		Flags: []urfavecli.Flag{
			&urfavecli.BoolFlag{Name: "json", Usage: "Write stable JSON output"},
			&urfavecli.BoolFlag{Name: "plain", Usage: "Write plain text without decoration"},
			&urfavecli.BoolFlag{Name: "quiet", Aliases: []string{"q"}, Usage: "Suppress successful output"},
			&urfavecli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "Write additional diagnostics to stderr"},
			&urfavecli.BoolFlag{Name: "no-color", Usage: "Disable color and interactive decoration"},
			&urfavecli.BoolFlag{Name: "no-input", Usage: "Never prompt; fail when input is missing"},
			&urfavecli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Approve a change that requires confirmation"},
			&urfavecli.DurationFlag{Name: "timeout", Value: 30 * time.Second, Usage: "Limit operation work; ledger lock conflicts remain immediate"},
		},
		Commands: []*urfavecli.Command{
			initCommand(),
			ledgerCommand(),
			ledgerReadCommand("validate", "Validate a ledger locally", true),
			ledgerReadCommand("status", "Show ledger and evidence status", true),
			platformCommand(), groupCommand(), relationshipCommand(), questionCommand(), forecastCommand(),
			targetCommand(), timestampCommand(), verifyCommand(),
			publishCommand(), mcpCommand(), versionCommand(),
		},
		Action: func(ctx context.Context, command *urfavecli.Command) error {
			if command.NArg() > 0 {
				err := urfavecli.ShowCommandHelp(ctx, command, command.Args().First())
				return app.NewError(app.CodeUsage, err.Error(), err)
			}
			return urfavecli.ShowRootCommandHelp(command)
		},
		Before: func(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
			if err := contextApplicationError(ctx); err != nil {
				return ctx, err
			}
			modes := 0
			for _, name := range []string{"json", "plain", "quiet"} {
				if command.Bool(name) {
					modes++
				}
			}
			if modes > 1 {
				return ctx, app.NewError(app.CodeUsage, "--json, --plain, and --quiet cannot be combined", nil)
			}
			return ctx, nil
		},
		ExitErrHandler: func(context.Context, *urfavecli.Command, error) {},
	}
	_ = root.Walk(func(command *urfavecli.Command) error {
		command.OnUsageError = func(_ context.Context, _ *urfavecli.Command, err error, _ bool) error {
			return app.NewError(app.CodeUsage, "invalid command input: "+err.Error(), err)
		}
		return nil
	})
	return root
}

func commandEffects(command *urfavecli.Command) service.Effects {
	if effects, ok := command.Root().Metadata["effects"].(service.Effects); ok {
		return effects
	}
	return service.ProductionEffects()
}

func presentOperationOutcome(command *urfavecli.Command, operation service.OperationName, dryRun bool, data any, operationErr error, humanMessage string) error {
	outcome := service.ClassifyOperationOutcome(operation, service.OutcomeInput{DryRun: dryRun, Data: data, Err: operationErr})
	if operationErr != nil && !outcome.HasData {
		return operationErr
	}
	presenter := presenterFor(command)
	message := outcome.Message
	if humanMessage != "" && presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		message = humanMessage
	}
	if err := presenter.Success(outcome.Code, message, data); err != nil {
		return err
	}
	if operationErr != nil {
		return presentedApplicationError{operationErr}
	}
	if outcome.FailureCode != "" {
		return presentedApplicationError{app.NewError(outcome.FailureCode, outcome.Message, nil)}
	}
	return nil
}

func ledgerCommand() *urfavecli.Command {
	authorFlags := rootPatchFlags()
	flags := append([]urfavecli.Flag{fileFlag(false)}, authorFlags...)
	command := leaf("update", "Update ledger and current forecaster metadata", "forecast-ledger ledger update --file ledger.yaml --title 'Forecast archive' --timezone Europe/London", false, flags)
	command.Description += "\n\nUse direct flags for authoring."
	command.Action = ledgerUpdateAction
	return group("ledger", "Manage ledger metadata", command)
}

func ledgerUpdateAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	input, err := buildRootPatchInput(command)
	if err != nil {
		return err
	}
	var result service.RootMetadataFileResult
	if runtime.DryRun {
		result, err = service.PlanRootMetadataFileUpdate(operationContext, command.String("file"), input)
	} else {
		result, err = service.CommitRootMetadataFileUpdate(operationContext, command.String("file"), input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationLedgerUpdate, runtime.DryRun, result, nil, "")
}

func initCommand() *urfavecli.Command {
	authorFlags := append(rootAuthoringFlags(), initNestedFlags()...)
	flags := []urfavecli.Flag{fileFlag(false),
		&urfavecli.StringFlag{Name: "ledger-id", Required: true, Usage: "Stable ledger ID"},
		&urfavecli.StringFlag{Name: "timezone", Required: true, Usage: "IANA timezone name"},
		&urfavecli.StringFlag{Name: "forecaster-id", Required: true, Usage: "Stable forecaster ID"},
		&urfavecli.StringFlag{Name: "forecaster-name", Required: true, Usage: "Forecaster display name"},
		&urfavecli.StringFlag{Name: "forecaster-kind", Value: "individual", Usage: "Forecaster kind: individual or team"},
		&urfavecli.StringFlag{Name: "key-file", OnlyOnce: true, TakesFile: true, Usage: "New protected key file; required only for a sealed first forecast"},
	}
	flags = append(flags, authorFlags...)
	command := leaf("init", "Create a new ledger", "forecast-ledger init --file ledger.yaml --ledger-id my-forecasts --timezone Europe/London --forecaster-id me --forecaster-name 'My Name'", false,
		flags)
	command.Action = initAction
	command.Description += "\n\nAll non-secret ledger, platform, question, and public initial-forecast data are supplied through flags. Omitted initial times use one operation-clock observation."
	return command
}

func initAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	effects := commandEffects(command)
	operationAt, err := formatOperationTime(effects.Clock.Now(), command.String("timezone"))
	if err != nil {
		return err
	}
	if command.IsSet("initial-forecast") && !command.IsSet("initial-forecasted-at") {
		if err := command.Set("initial-forecasted-at", string(operationAt)); err != nil {
			return err
		}
	}
	var normalizedTimes []service.TimeNormalization
	for _, item := range []struct {
		name   string
		policy dateOnlyPolicy
	}{{"created-at", dateOnlyRejected}, {"question-created-at", dateOnlyRejected}, {"question-opens-at", dateOnlyStart}, {"question-expected-resolution-at", dateOnlyEnd}, {"initial-forecasted-at", dateOnlyRejected}, {"initial-recorded-at", dateOnlyRejected}} {
		normalized, err := normalizeSetTimeWithMetadata(command, item.name, command.String("timezone"), item.policy)
		if err != nil {
			return err
		}
		normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	}
	input, err := buildInitInput(operationContext, command, command.Root().Reader)
	if err != nil {
		return err
	}
	root, err := service.BuildLedgerRootAt(service.InitRootRequest{
		LedgerID: ledger.Slug(command.String("ledger-id")), Timezone: command.String("timezone"),
		ForecasterID: ledger.Slug(command.String("forecaster-id")), ForecasterName: command.String("forecaster-name"),
		ForecasterKind: ledger.ForecasterKind(command.String("forecaster-kind")), Input: input,
	}, operationAt)
	if err != nil {
		return err
	}
	shape, err := service.ClassifyInitInput(input)
	if err != nil {
		return err
	}
	keyPath := command.String("key-file")
	if shape != service.CreationSealedForecast && keyPath != "" {
		return app.NewError(app.CodeUsage, "--key-file is only valid for a sealed initial forecast", nil)
	}
	var model *ledger.Ledger
	var sealed service.SealedInitialBuild
	switch shape {
	case service.CreationLedgerOnly:
		model = root
	case service.CreationQuestionOnly:
		model, err = service.BuildInitialQuestionLedgerAt(root, *input.Question, operationAt)
	case service.CreationPublicForecast:
		model, err = service.BuildInitialPublicLedgerAt(root, *input.Question, operationAt)
	case service.CreationSealedForecast:
		protectedInputPath := command.String("initial-secret-input")
		protectedArgument := "--initial-secret-input"
		if protectedInputPath != "-" {
			if err := storage.CheckProtectedFile(protectedInputPath); err != nil {
				return protectedArgumentError(err, protectedArgument)
			}
		}
		if strings.TrimSpace(keyPath) == "" {
			return app.NewError(app.CodeUsage, "--key-file is required for a sealed initial forecast", nil)
		}
		if runtime.DryRun {
			model, err = service.PlanInitialSealedLedgerAt(root, *input.Question, operationAt)
		} else {
			sealed, err = service.BuildInitialSealedLedgerAt(operationContext, root, *input.Question, operationAt, effects)
			model = sealed.Ledger
		}
	}
	if err != nil {
		return err
	}
	ledgerPath := command.String("file")
	if runtime.DryRun {
		resolvedLedger, resolveErr := storage.ResolveNewFilePath(ledgerPath, "ledger file")
		if resolveErr != nil {
			return resolveErr
		}
		if _, encodeErr := service.EncodeNewLedger(model, resolvedLedger); encodeErr != nil {
			return encodeErr
		}
		effects := []service.SideEffect{{Kind: service.EffectLedger, Action: service.EffectCreate, Status: service.EffectDeferred, Path: filepath.Base(resolvedLedger), Owned: true, Rollback: service.RollbackCreatedPublic}}
		if shape == service.CreationSealedForecast {
			resolvedKey, keyErr := storage.ResolveNewFilePath(keyPath, "key file")
			if keyErr != nil {
				return keyErr
			}
			if resolvedKey == resolvedLedger {
				return app.NewError(app.CodeConflict, "ledger and key destinations must be different", nil)
			}
			effects = append([]service.SideEffect{{Kind: service.EffectKey, Action: service.EffectCreate, Status: service.EffectDeferred, Path: filepath.Base(resolvedKey), Owned: true, Rollback: service.RollbackRetainSecret}}, effects...)
		}
		result := service.NewInitResult(model, effects, service.Recovery{State: service.RecoveryNone})
		result.NormalizedTimes = normalizedTimes
		return presentOperationOutcome(command, service.OperationLedgerInit, true, result, nil, "")
	}
	recovery := service.Recovery{State: service.RecoveryNone}
	if shape == service.CreationSealedForecast {
		commit, commitErr := service.CommitInitialSealedFiles(operationContext, ledgerPath, keyPath, sealed, service.InitialCommitOptions{})
		recovery = commit.Recovery
		if commitErr != nil {
			return withRecovery(commitErr, recovery)
		}
	} else {
		if _, err := service.CommitNewLedger(ledgerPath, model); err != nil {
			return err
		}
	}
	completedEffects := []service.SideEffect{{Kind: service.EffectLedger, Action: service.EffectCreate, Status: service.EffectCompleted, Path: filepath.Base(ledgerPath), Owned: true, Rollback: service.RollbackCreatedPublic}}
	if shape == service.CreationSealedForecast {
		completedEffects = append([]service.SideEffect{{Kind: service.EffectKey, Action: service.EffectCreate, Status: service.EffectCompleted, Path: filepath.Base(keyPath), Owned: true, Rollback: service.RollbackRetainSecret}}, completedEffects...)
	}
	result := service.NewInitResult(model, completedEffects, recovery)
	result.NormalizedTimes = normalizedTimes
	return presentOperationOutcome(command, service.OperationLedgerInit, false, result, nil, "")
}

func withRecovery(err error, recovery service.Recovery) error {
	if recovery.State == "" || recovery.State == service.RecoveryNone {
		return err
	}
	var applicationErr *app.Error
	if !errors.As(err, &applicationErr) {
		applicationErr = app.NewError(app.CodeInternal, "operation failed and recovery information is available", err)
	}
	details := make(map[string]any, len(applicationErr.Details)+1)
	for key, value := range applicationErr.Details {
		details[key] = value
	}
	details["recovery"] = recovery
	return app.WithDetails(applicationErr, details)
}

func ledgerReadCommand(name, usage string, stdinAllowed bool) *urfavecli.Command {
	command := leaf(name, usage, fmt.Sprintf("forecast-ledger %s --file ledger.yaml", name), true, []urfavecli.Flag{fileFlag(stdinAllowed)})
	command.Action = func(ctx context.Context, command *urfavecli.Command) error {
		runtime := RuntimeFromCommand(command)
		operationContext, cancel := runtime.Context(ctx)
		defer cancel()
		loaded, err := service.LoadAndValidateLedger(operationContext, command.String("file"), command.Root().Reader)
		if err != nil {
			return err
		}
		if name == "validate" {
			return presentOperationOutcome(command, service.OperationLedgerValidate, false, map[string]any{"ledger_id": loaded.Model.LedgerID, "schema_version": loaded.Model.SchemaVersion}, nil, "")
		}
		status, err := service.StatusForLedger(loaded)
		if err != nil {
			return app.NewError(app.CodeInternal, "ledger status could not be built", err)
		}
		message := fmt.Sprintf("%s: %d questions, %d forecasts; integrity: %d unanchored, %d pending, %d verified, %d failed",
			status.LedgerID, status.Questions, status.Forecasts, status.Unanchored, status.Pending, status.Verified, status.Failed)
		return presentOperationOutcome(command, service.OperationLedgerStatus, false, status, nil, message)
	}
	return command
}

func platformCommand() *urfavecli.Command {
	addFlags := append([]urfavecli.Flag{fileFlag(false), platformFlag()}, platformCreateFlags()...)
	add := leaf("add", "Add a platform", "forecast-ledger platform add --file ledger.yaml --platform metaculus --name Metaculus --kind scoring_platform", false, addFlags)
	add.Description += "\n\nUse direct flags for authoring."
	add.Action = platformAddAction
	updateFlags := append([]urfavecli.Flag{fileFlag(false), platformFlag()}, platformPatchFlags()...)
	update := leaf("update", "Update a platform", "forecast-ledger platform update --file ledger.yaml --platform metaculus --url https://www.metaculus.com", false, updateFlags)
	update.Description += "\n\nOmitted fields are unchanged. Use --clear-url or --clear-account for explicit removal."
	update.Action = platformUpdateAction
	list := leaf("list", "List platforms", "forecast-ledger platform list --file ledger.yaml", true, []urfavecli.Flag{fileFlag(true)})
	list.Action = platformListAction
	show := leaf("show", "Show a platform", "forecast-ledger platform show --file ledger.yaml --platform metaculus", true, []urfavecli.Flag{fileFlag(true), platformFlag()})
	show.Action = platformShowAction
	remove := leaf("remove", "Remove an unused platform", "forecast-ledger platform remove --file ledger.yaml --platform old-platform --yes", false, []urfavecli.Flag{fileFlag(false), platformFlag()})
	remove.Action = platformRemoveAction
	return group("platform", "Manage platform records", add, update, list, show, remove)
}

func platformAddAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	input, err := buildPlatformCreateInput(command)
	if err != nil {
		return err
	}
	id := ledger.Slug(command.String("platform"))
	var result service.PlatformFileResult
	if runtime.DryRun {
		result, err = service.PlanPlatformAddFile(operationContext, command.String("file"), id, input)
	} else {
		result, err = service.CommitPlatformAddFile(operationContext, command.String("file"), id, input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationPlatformAdd, runtime.DryRun, result, nil, "")
}

func platformUpdateAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	input, err := buildPlatformPatchInput(command)
	if err != nil {
		return err
	}
	id := ledger.Slug(command.String("platform"))
	var result service.PlatformFileResult
	if runtime.DryRun {
		result, err = service.PlanPlatformUpdateFile(operationContext, command.String("file"), id, input)
	} else {
		result, err = service.CommitPlatformUpdateFile(operationContext, command.String("file"), id, input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationPlatformUpdate, runtime.DryRun, result, nil, "")
}

func platformListAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	ledgerID, items, err := service.LoadPlatformList(operationContext, command.String("file"), command.Root().Reader)
	if err != nil {
		return err
	}
	var lines strings.Builder
	for index, item := range items {
		if index > 0 {
			lines.WriteByte('\n')
		}
		fmt.Fprintf(&lines, "%s\t%s\t%s\t%d", item.ID, item.Kind, item.Name, item.ReferenceCount)
	}
	message := lines.String()
	if message == "" {
		message = "No platforms"
	}
	return presentOperationOutcome(command, service.OperationPlatformList, false, map[string]any{"ledger_id": ledgerID, "platforms": items}, nil, message)
}

func platformShowAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	ledgerID, result, err := service.LoadPlatformShow(operationContext, command.String("file"), command.Root().Reader, ledger.Slug(command.String("platform")))
	if err != nil {
		return err
	}
	message := fmt.Sprintf("%s\t%s\t%s\t%d", result.ID, result.Platform.Kind, result.Platform.Name, len(result.ReferencingQuestionIDs))
	return presentOperationOutcome(command, service.OperationPlatformShow, false, map[string]any{"ledger_id": ledgerID, "platform": result}, nil, message)
}

func platformRemoveAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("platform"))
	if runtime.DryRun {
		result, err := service.PlanPlatformRemoveFile(operationContext, command.String("file"), id)
		if err != nil {
			return err
		}
		return presentOperationOutcome(command, service.OperationPlatformRemove, true, result, nil, "")
	}
	approved, err := runtime.Confirm(operationContext, fmt.Sprintf("Remove platform %s?", id))
	if err != nil {
		return err
	}
	if !approved {
		return app.NewError(app.CodeConflict, "platform removal was not approved", nil)
	}
	result, err := service.CommitPlatformRemoveFile(operationContext, command.String("file"), id)
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationPlatformRemove, false, result, nil, "")
}

func groupCommand() *urfavecli.Command {
	add := leaf("add", "Add a group", "forecast-ledger group add --file ledger.yaml --group macro --title 'Macro questions'", false, []urfavecli.Flag{fileFlag(false), groupFlag(), &urfavecli.StringFlag{Name: "title", Required: true}, &urfavecli.StringFlag{Name: "description"}})
	add.Action = groupAddAction
	update := leaf("update", "Update a group", "forecast-ledger group update --file ledger.yaml --group macro --title '2027 macro'", false, []urfavecli.Flag{fileFlag(false), groupFlag(), &urfavecli.StringFlag{Name: "title"}, &urfavecli.StringFlag{Name: "description"}, &urfavecli.BoolFlag{Name: "clear-description"}})
	update.Action = groupUpdateAction
	list := leaf("list", "List groups", "forecast-ledger group list --file ledger.yaml", true, []urfavecli.Flag{fileFlag(true)})
	list.Action = groupListAction
	show := leaf("show", "Show a group", "forecast-ledger group show --file ledger.yaml --group macro", true, []urfavecli.Flag{fileFlag(true), groupFlag()})
	show.Action = groupShowAction
	remove := leaf("remove", "Remove an unreferenced group", "forecast-ledger group remove --file ledger.yaml --group macro --yes", false, []urfavecli.Flag{fileFlag(false), groupFlag()})
	remove.Action = groupRemoveAction
	return group("group", "Manage question groups", add, update, list, show, remove)
}

func groupAddAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("group"))
	input := service.GroupCreateInput{Title: command.String("title"), Description: optionalStringValue(command, "description")}
	var result service.CollectionFileResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanGroupAddFile(op, command.String("file"), id, input)
	} else {
		result, err = service.CommitGroupAddFile(op, command.String("file"), id, input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationGroupAdd, runtime.DryRun, result, nil, "")
}
func groupUpdateAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	input := service.GroupPatchInput{}
	if command.IsSet("title") {
		input.Title = service.Optional[string]{Set: true, Value: command.String("title")}
	}
	description, err := patchString(command, "description", "clear-description")
	if err != nil {
		return err
	}
	input.Description = description
	id := ledger.Slug(command.String("group"))
	var result service.CollectionFileResult
	if runtime.DryRun {
		result, err = service.PlanGroupUpdateFile(op, command.String("file"), id, input)
	} else {
		result, err = service.CommitGroupUpdateFile(op, command.String("file"), id, input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationGroupUpdate, runtime.DryRun, result, nil, "")
}
func groupListAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id, values, err := service.LoadGroupList(op, command.String("file"), command.Root().Reader)
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationGroupList, false, map[string]any{"ledger_id": id, "groups": values}, nil, "")
}
func groupShowAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	ledgerID, value, err := service.LoadGroupShow(op, command.String("file"), command.Root().Reader, ledger.Slug(command.String("group")))
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationGroupShow, false, map[string]any{"ledger_id": ledgerID, "group": value}, nil, "")
}
func groupRemoveAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("group"))
	if !runtime.DryRun {
		approved, err := runtime.Confirm(op, "Remove group "+string(id)+"?")
		if err != nil {
			return err
		}
		if !approved {
			return app.NewError(app.CodeConflict, "group removal was not approved", nil)
		}
	}
	var result service.CollectionFileResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanGroupRemoveFile(op, command.String("file"), id)
	} else {
		result, err = service.CommitGroupRemoveFile(op, command.String("file"), id)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationGroupRemove, runtime.DryRun, result, nil, "")
}

func relationshipCommand() *urfavecli.Command {
	addFlags := []urfavecli.Flag{fileFlag(false), relationshipFlag(), &urfavecli.StringFlag{Name: "kind", Required: true}, &urfavecli.StringFlag{Name: "group"}, &urfavecli.StringFlag{Name: "question"}, &urfavecli.StringFlag{Name: "parent-question"}, &urfavecli.StringFlag{Name: "parent-revision"}, &urfavecli.StringFlag{Name: "parent-outcome"}, &urfavecli.BoolFlag{Name: "parent-outcome-boolean"}, &urfavecli.StringFlag{Name: "child-question"}}
	add := leaf("add", "Add a typed relationship", "forecast-ledger relationship add --file ledger.yaml --relationship rel-1 --kind group_membership --group macro --question q-rate", false, addFlags)
	add.Action = relationshipAddAction
	list := leaf("list", "List relationships", "forecast-ledger relationship list --file ledger.yaml", true, []urfavecli.Flag{fileFlag(true)})
	list.Action = relationshipListAction
	show := leaf("show", "Show a relationship", "forecast-ledger relationship show --file ledger.yaml --relationship rel-1", true, []urfavecli.Flag{fileFlag(true), relationshipFlag()})
	show.Action = relationshipShowAction
	remove := leaf("remove", "Remove an unreferenced relationship", "forecast-ledger relationship remove --file ledger.yaml --relationship rel-1 --yes", false, []urfavecli.Flag{fileFlag(false), relationshipFlag()})
	remove.Action = relationshipRemoveAction
	return group("relationship", "Manage question relationships", add, list, show, remove)
}

func relationshipAddAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("relationship"))
	kind := ledger.RelationshipKind(command.String("kind"))
	var relationship ledger.Relationship
	switch kind {
	case ledger.RelationshipGroupMembership:
		if err := requireDirectFlags(command, "group", "question"); err != nil {
			return err
		}
		relationship.GroupMembership = &ledger.GroupMembership{ID: id, Kind: kind, GroupID: ledger.Slug(command.String("group")), QuestionID: ledger.Slug(command.String("question"))}
	case ledger.RelationshipConditional:
		if err := requireDirectFlags(command, "parent-question", "parent-revision", "child-question"); err != nil {
			return err
		}
		outcome := ledger.ScalarValue{}
		if command.IsSet("parent-outcome") == command.IsSet("parent-outcome-boolean") {
			return app.NewError(app.CodeUsage, "use exactly one parent outcome flag", nil)
		}
		if command.IsSet("parent-outcome-boolean") {
			outcome.Boolean = pointer(command.Bool("parent-outcome-boolean"))
		} else {
			outcome.String = pointer(command.String("parent-outcome"))
		}
		relationship.Conditional = &ledger.ConditionalRelationship{ID: id, Kind: kind, ParentQuestionID: ledger.Slug(command.String("parent-question")), ParentQuestionRevisionID: ledger.Slug(command.String("parent-revision")), ParentOutcome: outcome, ChildQuestionID: ledger.Slug(command.String("child-question"))}
	default:
		return app.NewError(app.CodeUsage, "--kind must be group_membership or conditional", nil)
	}
	input := service.RelationshipInput{Relationship: relationship}
	var result service.CollectionFileResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanRelationshipAddFile(op, command.String("file"), input)
	} else {
		result, err = service.CommitRelationshipAddFile(op, command.String("file"), input)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationRelationshipAdd, runtime.DryRun, result, nil, "")
}
func relationshipListAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id, values, err := service.LoadRelationshipList(op, command.String("file"), command.Root().Reader)
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationRelationshipList, false, map[string]any{"ledger_id": id, "relationships": values}, nil, "")
}
func relationshipShowAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id, value, err := service.LoadRelationshipShow(op, command.String("file"), command.Root().Reader, ledger.Slug(command.String("relationship")))
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationRelationshipShow, false, map[string]any{"ledger_id": id, "relationship": value}, nil, "")
}
func relationshipRemoveAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("relationship"))
	if !runtime.DryRun {
		approved, err := runtime.Confirm(op, "Remove relationship "+string(id)+"?")
		if err != nil {
			return err
		}
		if !approved {
			return app.NewError(app.CodeConflict, "relationship removal was not approved", nil)
		}
	}
	var result service.CollectionFileResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanRelationshipRemoveFile(op, command.String("file"), id)
	} else {
		result, err = service.CommitRelationshipRemoveFile(op, command.String("file"), id)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationRelationshipRemove, runtime.DryRun, result, nil, "")
}

func questionCommand() *urfavecli.Command {
	addFlags := []urfavecli.Flag{fileFlag(false), questionFlag(), &urfavecli.StringFlag{Name: "key-file", OnlyOnce: true, TakesFile: true, Usage: "New protected key file; required only for a sealed first forecast"}}
	addFlags = append(addFlags, questionCreateFlags(true)...)
	add := leaf("add", "Add a question, optionally with its first forecast", "forecast-ledger question add --file ledger.yaml --question q-launch --revision-id qr-launch-1 --title 'Will it launch?' --resolution-criteria 'Resolves yes on launch' --expected-resolution-at 2027-02-02T23:59:59Z --outcome-kind binary", false, addFlags)
	add.Description += "\n\nUse direct flags for authoring. Repeated structured values use the documented CSV field order; sealed private data remains protected."
	add.Action = questionAddAction
	reviseFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag()}, domainFlags("")...)
	reviseFlags = append(reviseFlags, provenanceFlags("revision")...)
	revise := leaf("revise", "Append a complete immutable question revision", "forecast-ledger question revise --file ledger.yaml --question q-launch --revision-id qr-launch-2 --effective-at 2026-12-02T00:00:00Z --title 'Updated wording' --resolution-criteria '...' --expected-resolution-at 2027-02-02T23:59:59Z --outcome-kind binary", false, reviseFlags)
	revise.Action = questionReviseAction
	updateFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag()}, questionPatchFlags()...)
	update := leaf("update", "Update question metadata", "forecast-ledger question update --file ledger.yaml --question q-launch --tag launch --tag space", false, updateFlags)
	update.Description += "\n\nOmitted fields are unchanged; --clear-* removes optional values."
	update.Action = questionUpdateAction
	list := leaf("list", "List questions", "forecast-ledger question list --file ledger.yaml", true, []urfavecli.Flag{fileFlag(true)})
	list.Action = questionListAction
	show := leaf("show", "Show a question", "forecast-ledger question show --file ledger.yaml --question q-launch", true, []urfavecli.Flag{fileFlag(true), questionFlag()})
	show.Description += "\n\nNormal human and plain output includes public business fields and redacted forecast summaries."
	show.Action = questionShowAction
	resolveFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag()}, lifecycleFlags(true)...)
	resolve := leaf("resolve", "Resolve a question", "forecast-ledger question resolve --file ledger.yaml --question q-launch --outcome-boolean=true --outcome-known-at 2027-01-02T00:00:00Z --source 'Official result,https://example.com/result,2027-01-02T00:10:00Z' --yes", false, resolveFlags)
	resolve.Action = questionResolveAction
	unresolvedFlags := func() []urfavecli.Flag {
		return append([]urfavecli.Flag{fileFlag(false), questionFlag()}, lifecycleFlags(false)...)
	}
	ambiguous := leaf("ambiguous", "Mark a question ambiguous", "forecast-ledger question ambiguous --file ledger.yaml --question q-launch --reason 'Sources do not identify one outcome' --yes", false, unresolvedFlags())
	ambiguous.Action = questionAmbiguousAction
	void := leaf("void", "Mark a question void", "forecast-ledger question void --file ledger.yaml --question q-launch --reason 'The event definition became invalid' --yes", false, unresolvedFlags())
	void.Action = questionVoidAction
	disputeFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag()}, lifecycleFlags(false)...)
	dispute := leaf("dispute", "Dispute a resolution", "forecast-ledger question dispute --file ledger.yaml --question q-launch --reason 'Source conflicts with the recorded outcome' --yes", false, disputeFlags)
	dispute.Action = questionDisputeAction
	notApplicableFlags := []urfavecli.Flag{fileFlag(false), questionFlag(), &urfavecli.StringFlag{Name: "relationship", Required: true, OnlyOnce: true}, &urfavecli.StringFlag{Name: "reason", Required: true, OnlyOnce: true}, &urfavecli.StringFlag{Name: "recorded-at", OnlyOnce: true}}
	notApplicable := leaf("not-applicable", "Mark a conditional child not applicable", "forecast-ledger question not-applicable --file ledger.yaml --question q-child --relationship rel-condition --reason 'Parent condition was not met' --yes", false, notApplicableFlags)
	notApplicable.Action = questionNotApplicableAction
	for _, command := range []*urfavecli.Command{resolve, ambiguous, void, dispute} {
		command.Description += "\n\nUse repeated --source values as title,url,retrieved-at[,publisher[,published-at[,sha256]]]."
	}
	return group("question", "Manage forecast questions", add, revise, update, list, show, resolve, ambiguous, void, dispute, notApplicable)
}

func questionAddAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	if command.IsSet("initial-forecast") && !command.IsSet("initial-forecasted-at") {
		if err := command.Set("initial-forecasted-at", string(observedAt)); err != nil {
			return err
		}
	}
	var normalizedTimes []service.TimeNormalization
	for _, item := range []struct {
		name   string
		policy dateOnlyPolicy
	}{{"created-at", dateOnlyRejected}, {"opens-at", dateOnlyStart}, {"expected-resolution-at", dateOnlyEnd}, {"initial-forecasted-at", dateOnlyRejected}, {"initial-recorded-at", dateOnlyRejected}} {
		normalized, err := normalizeSetTimeWithMetadata(command, item.name, timezone, item.policy)
		if err != nil {
			return err
		}
		normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	}
	input, err := buildQuestionAddInput(operationContext, command, command.Root().Reader)
	if err != nil {
		return err
	}
	normalized := service.NormalizedQuestionCreate{ID: ledger.Slug(command.String("question")), Input: input}
	keyPath := command.String("key-file")
	shape, err := service.ClassifyQuestionAddInput(input)
	if err != nil {
		return err
	}
	var result service.QuestionFileResult
	humanMessage := ""
	if shape != service.CreationSealedForecast && keyPath != "" {
		return app.NewError(app.CodeUsage, "--key-file is only valid for a sealed initial forecast", nil)
	}
	switch shape {
	case service.CreationQuestionOnly:
		if runtime.DryRun {
			result, err = service.PlanQuestionAddEmptyFile(operationContext, command.String("file"), normalized, observedAt)
		} else {
			result, err = service.CommitQuestionAddEmptyFile(operationContext, command.String("file"), normalized, observedAt)
		}
	case service.CreationPublicForecast:
		humanMessage = "Question and initial forecast were added"
		if runtime.DryRun {
			result, err = service.PlanQuestionAddPublicFile(operationContext, command.String("file"), normalized, observedAt)
		} else {
			result, err = service.CommitQuestionAddPublicFile(operationContext, command.String("file"), normalized, observedAt)
		}
	case service.CreationSealedForecast:
		humanMessage = "Question and initial forecast were added"
		protectedInputPath := command.String("initial-secret-input")
		protectedArgument := "--initial-secret-input"
		if protectedInputPath != "-" {
			if err := storage.CheckProtectedFile(protectedInputPath); err != nil {
				return protectedArgumentError(err, protectedArgument)
			}
		}
		if strings.TrimSpace(keyPath) == "" {
			return app.NewError(app.CodeUsage, "--key-file is required for a sealed initial forecast", nil)
		}
		if runtime.DryRun {
			result, err = service.PlanQuestionAddSealedFile(operationContext, command.String("file"), keyPath, normalized, observedAt)
		} else {
			result, err = service.CommitQuestionAddSealedFile(operationContext, command.String("file"), keyPath, normalized, observedAt, commandEffects(command))
		}
	}
	if err != nil {
		return withRecovery(err, result.Recovery)
	}
	result.NormalizedTimes = normalizedTimes
	if runtime.DryRun {
		humanMessage = ""
	}
	return presentOperationOutcome(command, service.OperationQuestionAdd, runtime.DryRun, result, nil, humanMessage)
}

func questionUpdateAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	var normalizedTimes []service.TimeNormalization
	normalized, err := normalizeSetTimeWithMetadata(command, "opens-at", timezone, dateOnlyStart)
	if err != nil {
		return err
	}
	normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	normalized, err = normalizeSetTimeWithMetadata(command, "expected-resolution-at", timezone, dateOnlyEnd)
	if err != nil {
		return err
	}
	normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	input, err := buildQuestionPatchInput(command)
	if err != nil {
		return err
	}
	id := ledger.Slug(command.String("question"))
	var result service.QuestionFileResult
	if runtime.DryRun {
		result, err = service.PlanQuestionUpdateFile(operationContext, command.String("file"), id, input)
	} else {
		result, err = service.CommitQuestionUpdateFile(operationContext, command.String("file"), id, input)
	}
	if err != nil {
		return err
	}
	result.NormalizedTimes = normalizedTimes
	return presentOperationOutcome(command, service.OperationQuestionUpdate, runtime.DryRun, result, nil, "")
}

func questionReviseAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		name   string
		policy dateOnlyPolicy
	}{{"effective-at", dateOnlyRejected}, {"revision-recorded-at", dateOnlyRejected}, {"opens-at", dateOnlyStart}, {"expected-resolution-at", dateOnlyEnd}} {
		if _, err := normalizeSetTimeWithMetadata(command, item.name, timezone, item.policy); err != nil {
			return err
		}
	}
	input, err := buildRevisionInput(command, "", "revision")
	if err != nil {
		return err
	}
	id := ledger.Slug(command.String("question"))
	var result service.QuestionFileResult
	if runtime.DryRun {
		result, err = service.PlanQuestionReviseFile(operationContext, command.String("file"), id, input, observedAt)
	} else {
		result, err = service.CommitQuestionReviseFile(operationContext, command.String("file"), id, input, observedAt)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationQuestionRevise, runtime.DryRun, result, nil, "")
}

func questionListAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	ledgerID, items, err := service.LoadQuestionList(operationContext, command.String("file"), command.Root().Reader)
	if err != nil {
		return err
	}
	var lines strings.Builder
	for index, item := range items {
		if index > 0 {
			lines.WriteByte('\n')
		}
		fmt.Fprintf(&lines, "%s\t%s\t%s\t%s\t%s\t%d\t%s", item.ID, item.CurrentRevisionID, item.Title, item.OutcomeKind, item.Status, item.ForecastCount, item.ExpectedResolutionAt)
	}
	message := lines.String()
	if message == "" {
		message = "No questions"
	}
	return presentOperationOutcome(command, service.OperationQuestionList, false, map[string]any{"ledger_id": ledgerID, "questions": items}, nil, message)
}

func questionShowAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	id := ledger.Slug(command.String("question"))
	ledgerID, result, err := service.LoadQuestionShow(operationContext, command.String("file"), command.Root().Reader, id)
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	message := "Question was found"
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		message = formatQuestionView(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationQuestionShow, false, map[string]any{"ledger_id": ledgerID, "question": result}, nil, message)
}

func questionResolveAction(ctx context.Context, command *urfavecli.Command) error {
	var input service.ResolutionInput
	return questionTerminalAction(ctx, command, func(command *urfavecli.Command) error {
		value, err := buildResolutionInput(command)
		input = value
		return err
	}, "resolve", func(operationContext context.Context, id ledger.Slug, observedAt ledger.Timestamp, dryRun bool) (service.QuestionFileResult, error) {
		if dryRun {
			return service.PlanQuestionResolveFile(operationContext, command.String("file"), id, input, observedAt)
		}
		return service.CommitQuestionResolveFile(operationContext, command.String("file"), id, input, observedAt)
	})
}

func questionAmbiguousAction(ctx context.Context, command *urfavecli.Command) error {
	var input service.UnresolvedResolutionInput
	return questionTerminalAction(ctx, command, func(command *urfavecli.Command) error {
		reason, recordedAt, sources, err := buildReasonInput(command)
		input = service.UnresolvedResolutionInput{Reason: reason, RecordedAt: recordedAt, Sources: sources}
		return err
	}, "ambiguous", func(operationContext context.Context, id ledger.Slug, observedAt ledger.Timestamp, dryRun bool) (service.QuestionFileResult, error) {
		if dryRun {
			return service.PlanQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionAmbiguous, input, observedAt)
		}
		return service.CommitQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionAmbiguous, input, observedAt)
	})
}

func questionVoidAction(ctx context.Context, command *urfavecli.Command) error {
	var input service.UnresolvedResolutionInput
	return questionTerminalAction(ctx, command, func(command *urfavecli.Command) error {
		reason, recordedAt, sources, err := buildReasonInput(command)
		input = service.UnresolvedResolutionInput{Reason: reason, RecordedAt: recordedAt, Sources: sources}
		return err
	}, "void", func(operationContext context.Context, id ledger.Slug, observedAt ledger.Timestamp, dryRun bool) (service.QuestionFileResult, error) {
		if dryRun {
			return service.PlanQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionVoid, input, observedAt)
		}
		return service.CommitQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionVoid, input, observedAt)
	})
}

func questionDisputeAction(ctx context.Context, command *urfavecli.Command) error {
	var input service.UnresolvedResolutionInput
	return questionTerminalAction(ctx, command, func(command *urfavecli.Command) error {
		reason, recordedAt, sources, err := buildReasonInput(command)
		input = service.UnresolvedResolutionInput{Reason: reason, RecordedAt: recordedAt, Sources: sources}
		return err
	}, "dispute", func(operationContext context.Context, id ledger.Slug, observedAt ledger.Timestamp, dryRun bool) (service.QuestionFileResult, error) {
		if dryRun {
			return service.PlanQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionDisputed, input, observedAt)
		}
		return service.CommitQuestionUnresolvedFile(operationContext, command.String("file"), id, ledger.ResolutionDisputed, input, observedAt)
	})
}

func questionNotApplicableAction(ctx context.Context, command *urfavecli.Command) error {
	var input service.NotApplicableInput
	return questionTerminalAction(ctx, command, func(command *urfavecli.Command) error {
		input = service.NotApplicableInput{RelationshipID: ledger.Slug(command.String("relationship")), Reason: command.String("reason"), RecordedAt: optionalTimestampValue(command, "recorded-at")}
		return nil
	}, "not-applicable", func(operationContext context.Context, id ledger.Slug, observedAt ledger.Timestamp, dryRun bool) (service.QuestionFileResult, error) {
		if dryRun {
			return service.PlanQuestionNotApplicableFile(operationContext, command.String("file"), id, input, observedAt)
		}
		return service.CommitQuestionNotApplicableFile(operationContext, command.String("file"), id, input, observedAt)
	})
}

func questionTerminalAction(ctx context.Context, command *urfavecli.Command, buildDirect func(*urfavecli.Command) error, verb string, execute func(context.Context, ledger.Slug, ledger.Timestamp, bool) (service.QuestionFileResult, error)) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	var normalizedTimes []service.TimeNormalization
	for _, name := range []string{"outcome-known-at", "recorded-at"} {
		normalized, err := normalizeSetTimeWithMetadata(command, name, timezone, dateOnlyRejected)
		if err != nil {
			return err
		}
		normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	}
	if err := buildDirect(command); err != nil {
		return err
	}
	id := ledger.Slug(command.String("question"))
	if !runtime.DryRun {
		approved, err := runtime.Confirm(operationContext, fmt.Sprintf("%s question %s?", strings.ToUpper(verb[:1])+verb[1:], id))
		if err != nil {
			return err
		}
		if !approved {
			return app.NewError(app.CodeConflict, "question lifecycle change was not approved", nil)
		}
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	result, err := execute(operationContext, id, observedAt, runtime.DryRun)
	if err != nil {
		return err
	}
	result.NormalizedTimes = normalizedTimes
	operation := service.OperationQuestionDispute
	humanMessage := ""
	if verb == "resolve" {
		operation = service.OperationQuestionResolve
	} else if verb == "ambiguous" {
		operation = service.OperationQuestionAmbiguous
	} else if verb == "void" {
		operation = service.OperationQuestionVoid
	} else if verb == "not-applicable" {
		operation = service.OperationQuestionNotApplicable
	}
	return presentOperationOutcome(command, operation, runtime.DryRun, result, nil, humanMessage)
}

func forecastCommand() *urfavecli.Command {
	addFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag()}, forecastCreateFlags()...)
	add := leaf("add", "Add a public forecast", "forecast-ledger forecast add --file ledger.yaml --question q-launch --forecast f-002 --question-revision qr-launch-1 --forecasted-at 2026-12-01T12:00:00Z --probability 0.65 --probability-outcome", false, addFlags)
	add.Description += "\n\nUse one or more direct v2 representation families: probability, PMF, binned PMF, quantiles, CDF, point, or credible intervals."
	add.Action = forecastAddAction
	list := leaf("list", "List forecasts", "forecast-ledger forecast list --file ledger.yaml --question q-launch", true, []urfavecli.Flag{fileFlag(true), questionFlag()})
	list.Action = forecastListAction
	show := leaf("show", "Show a forecast", "forecast-ledger forecast show --file ledger.yaml --question q-launch --forecast f-001", true, []urfavecli.Flag{fileFlag(true), questionFlag(), forecastFlag()})
	show.Description += "\n\nNormal human and plain output includes type-aware public values and safe stored integrity evidence; sealed private fields stay redacted. No network check is performed."
	show.Action = forecastShowAction
	sealFlags := append([]urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag(), secretOutputFlag()}, forecastSealPublicFlags()...)
	seal := leaf("seal", "Create and append a sealed forecast", "forecast-ledger forecast seal --file ledger.yaml --question q-launch --forecast f-002 --question-revision qr-launch-1 --forecasted-at 2026-12-01T12:00:00Z --secret-input private.yaml --key-file secret.key", false, sealFlags)
	seal.Description += "\n\nValue, rationale, key factors, and comment stay in protected --secret-input while public times, note, and supersedes ID use flags."
	seal.Action = forecastSealAction
	reveal := leaf("reveal", "Verify and reveal a sealed forecast", "forecast-ledger forecast reveal --file ledger.yaml --question q-launch --forecast f-002 --key-file secret.key --yes", false, []urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag(), &urfavecli.StringFlag{Name: "key-file", Required: true, TakesFile: true, Usage: "Protected key file"}, &urfavecli.StringFlag{Name: "revealed-at", OnlyOnce: true, Usage: "Explicit RFC 3339 reveal time; defaults to the current clock"}})
	reveal.Action = forecastRevealAction
	hintUpdate := leaf("update", "Change a non-location key hint", "forecast-ledger forecast key-hint update --file ledger.yaml --question q-launch --forecast f-002 --key-hint forecast-key:f-002", false, []urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag(), &urfavecli.StringFlag{Name: "key-hint", Required: true, OnlyOnce: true, Usage: "Safe scheme:opaque logical hint"}})
	hintUpdate.Action = forecastKeyHintUpdateAction
	lifecycle := func(name string, eventType ledger.LifecycleEventType, operation service.OperationName) *urfavecli.Command {
		flags := []urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag(), &urfavecli.StringFlag{Name: "event", Required: true}, &urfavecli.StringFlag{Name: "effective-at", Required: true}, &urfavecli.StringFlag{Name: "recorded-at"}, &urfavecli.StringFlag{Name: "reason"}}
		flags = append(flags, provenanceFlags("")...)
		command := leaf(name, "Append a "+name+" lifecycle event", "forecast-ledger forecast "+name+" --file ledger.yaml --question q-launch --forecast f-002 --event event-1 --effective-at 2026-12-02T00:00:00Z", false, flags)
		command.Action = func(ctx context.Context, command *urfavecli.Command) error {
			return forecastLifecycleAction(ctx, command, eventType, operation)
		}
		return command
	}
	withdraw := lifecycle("withdraw", ledger.LifecycleWithdrawn, service.OperationForecastWithdraw)
	expire := lifecycle("expire", ledger.LifecycleExpired, service.OperationForecastExpire)
	reaffirm := lifecycle("reaffirm", ledger.LifecycleReaffirmed, service.OperationForecastReaffirm)
	return group("forecast", "Manage append-only forecast records", add, list, show, seal, reveal, withdraw, expire, reaffirm, group("key-hint", "Manage non-authoritative key hints", hintUpdate))
}

func forecastLifecycleAction(ctx context.Context, command *urfavecli.Command, eventType ledger.LifecycleEventType, operation service.OperationName) error {
	runtime := RuntimeFromCommand(command)
	op, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(op, command)
	if err != nil {
		return err
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	for _, name := range []string{"effective-at", "recorded-at"} {
		if _, err := normalizeSetTimeWithMetadata(command, name, timezone, dateOnlyRejected); err != nil {
			return err
		}
	}
	provenance, err := buildProvenance(command, "")
	if err != nil {
		return err
	}
	input := service.LifecycleInput{ID: ledger.Slug(command.String("event")), EffectiveAt: ledger.Timestamp(command.String("effective-at")), RecordedAt: optionalTimestampValue(command, "recorded-at"), Reason: optionalStringValue(command, "reason"), Provenance: provenance}
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	var result service.ForecastFileResult
	if runtime.DryRun {
		result, err = service.PlanForecastLifecycleFile(op, command.String("file"), questionID, forecastID, eventType, input, observedAt)
	} else {
		result, err = service.CommitForecastLifecycleFile(op, command.String("file"), questionID, forecastID, eventType, input, observedAt)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, operation, runtime.DryRun, result, nil, "")
}

func forecastAddAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	if !command.IsSet("forecasted-at") {
		if err := command.Set("forecasted-at", string(observedAt)); err != nil {
			return err
		}
	}
	var normalizedTimes []service.TimeNormalization
	for _, name := range []string{"forecasted-at", "recorded-at"} {
		normalized, err := normalizeSetTimeWithMetadata(command, name, timezone, dateOnlyRejected)
		if err != nil {
			return err
		}
		normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	}
	input, err := buildForecastCreateInput(command)
	if err != nil {
		return err
	}
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	var result service.ForecastFileResult
	if runtime.DryRun {
		result, err = service.PlanPublicForecastAddFile(operationContext, command.String("file"), questionID, forecastID, input, observedAt)
	} else {
		result, err = service.CommitPublicForecastAddFile(operationContext, command.String("file"), questionID, forecastID, input, observedAt)
	}
	if err != nil {
		return err
	}
	result.NormalizedTimes = normalizedTimes
	return presentOperationOutcome(command, service.OperationForecastAdd, runtime.DryRun, result, nil, "")
}

func forecastListAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	questionID := ledger.Slug(command.String("question"))
	ledgerID, items, err := service.LoadForecastList(operationContext, command.String("file"), command.Root().Reader, questionID)
	if err != nil {
		return err
	}
	var lines strings.Builder
	for index, item := range items {
		if index > 0 {
			lines.WriteByte('\n')
		}
		fmt.Fprintf(&lines, "%s\t%s\t%s\t%s\t%s\t%s\t%s", item.ID, item.QuestionRevisionID, item.ForecastedAt, item.Visibility, item.IntegrityStatus, compactPublicJSON(item.Activity), compactPublicJSON(item.RepresentationKinds))
	}
	message := lines.String()
	if message == "" {
		message = "No forecasts"
	}
	return presentOperationOutcome(command, service.OperationForecastList, false, map[string]any{"ledger_id": ledgerID, "question_id": questionID, "forecasts": items}, nil, message)
}

func forecastShowAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	ledgerID, result, err := service.LoadForecastShow(operationContext, command.String("file"), command.Root().Reader, questionID, forecastID)
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	message := "Forecast was found"
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		message = formatForecastView(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationForecastShow, false, map[string]any{"ledger_id": ledgerID, "question_id": questionID, "forecast": result}, nil, message)
}

func forecastSealAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	observedAt, err := formatOperationTime(commandEffects(command).Clock.Now(), timezone)
	if err != nil {
		return err
	}
	if !command.IsSet("forecasted-at") {
		if err := command.Set("forecasted-at", string(observedAt)); err != nil {
			return err
		}
	}
	var normalizedTimes []service.TimeNormalization
	for _, name := range []string{"forecasted-at", "recorded-at"} {
		normalized, err := normalizeSetTimeWithMetadata(command, name, timezone, dateOnlyRejected)
		if err != nil {
			return err
		}
		normalizedTimes = appendTimeNormalization(normalizedTimes, normalized)
	}
	input, err := buildSealedForecastInput(operationContext, command, command.Root().Reader)
	if err != nil {
		return err
	}
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	var result service.ForecastFileResult
	if runtime.DryRun {
		result, err = service.PlanForecastSealFile(operationContext, command.String("file"), command.String("key-file"), questionID, forecastID, input, observedAt)
	} else {
		result, err = service.CommitForecastSealFile(operationContext, command.String("file"), command.String("key-file"), questionID, forecastID, input, observedAt, commandEffects(command))
	}
	if err != nil {
		return withRecovery(err, result.Recovery)
	}
	result.NormalizedTimes = normalizedTimes
	return presentOperationOutcome(command, service.OperationForecastSeal, runtime.DryRun, result, nil, "")
}

func forecastRevealAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	timezone, err := mutationTimezone(operationContext, command)
	if err != nil {
		return err
	}
	if _, err := normalizeSetTimeWithMetadata(command, "revealed-at", timezone, dateOnlyRejected); err != nil {
		return err
	}
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	approved, err := runtime.Confirm(operationContext, fmt.Sprintf("Reveal forecast %s and publish its private fields and key?", forecastID))
	if err != nil {
		return err
	}
	if !approved {
		return app.NewError(app.CodeConflict, "forecast reveal was not approved", nil)
	}
	revealedAt := ledger.Timestamp(command.String("revealed-at"))
	if revealedAt == "" {
		revealedAt, err = formatOperationTime(commandEffects(command).Clock.Now(), timezone)
		if err != nil {
			return err
		}
	}
	var result service.ForecastFileResult
	if runtime.DryRun {
		result, err = service.PlanForecastRevealFile(operationContext, command.String("file"), command.String("key-file"), questionID, forecastID, revealedAt)
	} else {
		result, err = service.CommitForecastRevealFile(operationContext, command.String("file"), command.String("key-file"), questionID, forecastID, revealedAt)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationForecastReveal, runtime.DryRun, result, nil, "")
}

func forecastKeyHintUpdateAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	keyHint := command.String("key-hint")
	var result service.ForecastFileResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanForecastKeyHintUpdateFile(operationContext, command.String("file"), questionID, forecastID, keyHint)
	} else {
		result, err = service.CommitForecastKeyHintUpdateFile(operationContext, command.String("file"), questionID, forecastID, keyHint)
	}
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationForecastKeyHintUpdate, runtime.DryRun, result, nil, "")
}

func decodePrivateOperationInputForArgument(ctx context.Context, path string, stdin io.Reader, schema service.InputSchemaName, destination any, argument string) error {
	if path == "-" {
		return service.DecodeOperationInput(ctx, path, stdin, schema, destination)
	}
	data, err := storage.ReadProtectedFile(path, 8<<20, "protected "+argument+" file")
	if err != nil {
		return protectedArgumentError(err, argument)
	}
	defer clear(data)
	return service.DecodeOperationInput(ctx, "-", bytes.NewReader(data), schema, destination)
}

func protectedArgumentError(err error, argument string) error {
	var applicationErr *app.Error
	if !errors.As(err, &applicationErr) {
		return err
	}
	message := applicationErr.Message
	for _, phrase := range []string{"protected key file", "protected key path", "protected file"} {
		message = strings.ReplaceAll(message, phrase, "protected "+argument+" file")
	}
	details := make(map[string]any, len(applicationErr.Details)+1)
	for key, value := range applicationErr.Details {
		details[key] = value
	}
	details["argument"] = argument
	return app.WithDetails(app.NewError(applicationErr.Code, message, applicationErr.Cause), details)
}

func targetCommand() *urfavecli.Command {
	build := targetLeaf("build", "Build target artifacts", false)
	build.Action = targetBuildAction
	check := targetLeaf("check", "Check target bytes and digests", true)
	check.Description += "\n\nA never-built target is reported as not_applicable with build guidance; --all continues in ledger order."
	check.Action = targetCheckAction
	return group("target", "Build or check canonical forecast targets", build, check)
}

func targetLeaf(name, usage string, readOnly bool) *urfavecli.Command {
	flags := []urfavecli.Flag{fileFlag(false), &urfavecli.StringFlag{Name: "question", Usage: "Question ID"}, &urfavecli.StringFlag{Name: "forecast", Usage: "Forecast ID"}, &urfavecli.BoolFlag{Name: "all", Usage: "Select every forecast"}, &urfavecli.StringFlag{Name: "scope", Value: "forecast", Usage: "Target scope: forecast or lifecycle"}, &urfavecli.StringFlag{Name: "head", Usage: "Lifecycle head event ID"}}
	command := leaf(name, usage, fmt.Sprintf("forecast-ledger target %s --file ledger.yaml --question q-launch --forecast f-001", name), readOnly, flags)
	command.Before = requireTargetSelection
	return command
}

func targetBuildAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	all := command.Bool("all")
	questionID, forecastID := ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast"))
	scope, head := service.TargetScope(command.String("scope")), ledger.Slug(command.String("head"))
	var result service.TargetOperationResult
	var err error
	if runtime.DryRun {
		result, err = service.PlanTargetBuildScoped(operationContext, command.String("file"), scope, all, questionID, forecastID, head)
	} else {
		result, err = service.CommitTargetBuildScoped(operationContext, command.String("file"), scope, all, questionID, forecastID, head)
	}
	if err != nil {
		return withRecovery(err, result.Recovery)
	}
	return presentOperationOutcome(command, service.OperationTargetBuild, runtime.DryRun, result, nil, "")
}

func targetCheckAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.InspectTargetsScoped(operationContext, command.String("file"), service.TargetScope(command.String("scope")), command.Bool("all"), ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast")), ledger.Slug(command.String("head")))
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	humanMessage := ""
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		humanMessage = formatTargetInspection(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationTargetCheck, false, result, nil, humanMessage)
}

func formatTargetInspection(mode presentation.Mode, result service.TargetOperationResult) string {
	var output strings.Builder
	for index, target := range result.Targets {
		if index > 0 {
			output.WriteByte('\n')
		}
		reasons := strings.Join(target.ReasonCodes, ",")
		if mode == presentation.ModePlain {
			fmt.Fprintf(&output, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s", target.QuestionID, target.ForecastID, target.Scope, target.HeadEventID, target.State, reasons, target.Path, target.SHA256, target.ActualSHA256)
			if target.Guidance != "" {
				fmt.Fprintf(&output, "\t%s", target.Guidance)
			}
			continue
		}
		fmt.Fprintf(&output, "Target: %s / %s\n  Scope: %s", target.QuestionID, target.ForecastID, target.Scope)
		if target.HeadEventID != "" {
			fmt.Fprintf(&output, "\n  Head event: %s", target.HeadEventID)
		}
		fmt.Fprintf(&output, "\n  State: %s\n  Path: %s\n  Expected SHA-256: %s", target.State, target.Path, target.SHA256)
		if target.ActualSHA256 != "" {
			fmt.Fprintf(&output, "\n  Actual SHA-256: %s", target.ActualSHA256)
		}
		if reasons != "" {
			fmt.Fprintf(&output, "\n  Reason: %s", reasons)
		}
		if target.Guidance != "" {
			fmt.Fprintf(&output, "\n  Next: %s", target.Guidance)
		}
	}
	return output.String()
}

func timestampCommand() *urfavecli.Command {
	stamp := timestampLeaf("stamp", "Request and retain an RFC 3161 timestamp", false, true)
	stamp.Action = timestampStampAction
	status := timestampLeaf("status", "Show local RFC 3161 evidence status", true, false)
	status.Action = timestampStatusAction
	verify := timestampLeaf("verify", "Verify retained RFC 3161 evidence locally", false, false)
	verify.Action = timestampVerifyAction
	command := group("timestamp", "Manage experimental RFC 3161 request and response evidence", stamp, status, verify)
	command.Description = "Stamp defaults to the built-in FreeTSA profile and retained embedded trust. A named provider or an explicit public HTTPS TSA plus ledger-relative PEM bundle can be selected instead. Status and verification are local and make no timestamp-service network request."
	return command
}

func timestampLeaf(name, usage string, readOnly, stampOptions bool) *urfavecli.Command {
	flags := []urfavecli.Flag{fileFlag(false), questionFlag(), forecastFlag(), &urfavecli.StringFlag{Name: "scope", Value: "forecast", Usage: "Evidence scope: forecast or lifecycle"}, &urfavecli.StringFlag{Name: "head", Usage: "Lifecycle head event ID"}}
	if stampOptions {
		flags = append(flags, &urfavecli.BoolFlag{Name: "offline", Usage: "Open no network connection"})
		flags = append(flags,
			&urfavecli.StringFlag{Name: "tsa-provider", Usage: "Built-in timestamp provider: auto or freetsa (default: auto)"},
			&urfavecli.StringFlag{Name: "tsa-url", Usage: "Custom public HTTPS RFC 3161 timestamp authority URL; requires --ca-bundle"},
			&urfavecli.StringFlag{Name: "ca-bundle", TakesFile: true, Usage: "Custom retained ledger-relative PEM CA bundle; requires --tsa-url"},
		)
	}
	command := leaf(name, usage, fmt.Sprintf("forecast-ledger timestamp %s --file ledger.yaml --question q-launch --forecast f-001", name), readOnly, flags)
	command.Before = requireTimestampScope
	return command
}

func requireTimestampScope(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
	scope, head := command.String("scope"), command.String("head")
	if scope != "forecast" && scope != "lifecycle" {
		return ctx, app.NewError(app.CodeUsage, "--scope must be forecast or lifecycle", nil)
	}
	if scope == "lifecycle" && head == "" {
		return ctx, app.NewError(app.CodeUsage, "--scope lifecycle requires --head", nil)
	}
	if scope == "forecast" && head != "" {
		return ctx, app.NewError(app.CodeUsage, "--head requires --scope lifecycle", nil)
	}
	return ctx, nil
}

func timestampStampAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.CommitTimestampStamp(operationContext, command.String("file"), ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast")), service.TimestampStampOptions{
		DryRun: runtime.DryRun, Offline: command.Bool("offline"), TSAProvider: command.String("tsa-provider"), TSAURL: command.String("tsa-url"), CABundlePath: command.String("ca-bundle"), Effects: commandEffects(command),
		Scope: service.TargetScope(command.String("scope")), HeadEventID: ledger.Slug(command.String("head")),
	})
	if err != nil && result.FailureCode == "" {
		return withRecovery(err, result.Recovery)
	}
	presenter := presenterFor(command)
	humanMessage := ""
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		humanMessage = formatTimestampArtifact(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationTimestampStamp, runtime.DryRun, result, err, humanMessage)
}

func timestampStatusAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.TimestampStatusForScoped(operationContext, command.String("file"), ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast")), service.TargetScope(command.String("scope")), ledger.Slug(command.String("head")))
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	message := "RFC 3161 local status: " + string(result.State)
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		message = formatTimestampArtifact(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationTimestampStatus, false, result, nil, message)
}

func timestampVerifyAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.CommitTimestampVerify(operationContext, command.String("file"), ledger.Slug(command.String("question")), ledger.Slug(command.String("forecast")), service.TimestampVerifyOptions{DryRun: runtime.DryRun, Effects: commandEffects(command), Scope: service.TargetScope(command.String("scope")), HeadEventID: ledger.Slug(command.String("head"))})
	if err != nil && result.FailureCode == "" {
		return err
	}
	presenter := presenterFor(command)
	humanMessage := ""
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		humanMessage = formatTimestampVerification(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationTimestampVerify, runtime.DryRun, result, err, humanMessage)
}

func verifyCommand() *urfavecli.Command {
	command := leaf("verify", "Run layered verification", "forecast-ledger verify --file ledger.yaml --offline", true, []urfavecli.Flag{
		fileFlag(false),
		&urfavecli.StringFlag{Name: "question", Usage: "Optional question ID"},
		&urfavecli.StringFlag{Name: "forecast", Usage: "Optional forecast ID; requires --question"},
		&urfavecli.BoolFlag{Name: "offline", Usage: "Do not retrieve optional outcome sources; timestamp checks remain local"},
		&urfavecli.BoolFlag{Name: "check-sources", Usage: "Check outcome source reachability and stored digests"},
	})
	command.Before = func(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
		if command.String("forecast") != "" && command.String("question") == "" {
			return ctx, app.NewError(app.CodeUsage, "--forecast requires --question", nil)
		}
		return ctx, nil
	}
	command.Action = verificationAction
	command.Description += "\n\nNormal human and plain output includes the complete ordered evidence matrix and safe retained timing values. Offline stored values are not freshly rechecked. Pass requires at least one applicable forecast-evidence layer; an empty or all-not-applicable selection returns no_evidence."
	return command
}

func verificationAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	report, err := service.VerifyLedgerEvidence(operationContext, command.String("file"), service.VerificationOptions{
		Offline: command.Bool("offline"), CheckSources: command.Bool("check-sources"),
		QuestionID: ledger.Slug(command.String("question")), ForecastID: ledger.Slug(command.String("forecast")),
	})
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	humanMessage := ""
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		humanMessage = formatVerificationReport(presenter.Mode(), report)
	}
	return presentOperationOutcome(command, service.OperationVerificationRun, false, report, nil, humanMessage)
}

func publishCommand() *urfavecli.Command {
	build := leaf("build", "Build an evidence package", "forecast-ledger publish build --file ledger.yaml --output evidence", false, []urfavecli.Flag{fileFlag(false), &urfavecli.StringFlag{Name: "output", Required: true, TakesFile: true, Usage: "New package directory"}})
	build.Action = publicationBuildAction
	verify := leaf("verify", "Verify an evidence package", "forecast-ledger publish verify --file package/ledger/ledger.yaml --manifest package/manifest.json", true, []urfavecli.Flag{
		fileFlag(false),
		&urfavecli.StringFlag{Name: "manifest", Required: true, TakesFile: true, Usage: "Package manifest file"},
	})
	verify.Description += "\n\nRFC 3161 target, request, response, and retained CA-bundle checks are always local. Manifest and file integrity remain visible separately."
	verify.Action = publicationVerifyAction
	return group("publish", "Build or verify portable evidence packages", build, verify)
}

func publicationBuildAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.CommitPublicationBuild(operationContext, command.String("file"), command.String("output"), runtime.DryRun)
	if err != nil {
		return err
	}
	return presentOperationOutcome(command, service.OperationPublicationBuild, runtime.DryRun, result, nil, "")
}

func publicationVerifyAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	operationContext, cancel := runtime.Context(ctx)
	defer cancel()
	result, err := service.VerifyPublicationPackage(operationContext, command.String("file"), command.String("manifest"))
	if err != nil {
		return err
	}
	presenter := presenterFor(command)
	humanMessage := ""
	if presenter.Mode() != presentation.ModeJSON && presenter.Mode() != presentation.ModeQuiet {
		humanMessage = formatPublicationVerification(presenter.Mode(), result)
	}
	return presentOperationOutcome(command, service.OperationPublicationVerify, false, result, nil, humanMessage)
}

func mcpCommand() *urfavecli.Command {
	serve := leaf("serve", "Serve MCP over stdio", "forecast-ledger mcp serve --ledger-root main=/data/ledgers", true, []urfavecli.Flag{
		&urfavecli.StringSliceFlag{Name: "ledger-root", Required: true, Usage: "Allowed ledger root as name=path; repeat for more roots"},
		&urfavecli.StringSliceFlag{Name: "output-root", Usage: "Allowed package output root as name=path; repeat for more roots"},
		&urfavecli.StringSliceFlag{Name: "secret-root", Usage: "Allowed protected secret root as name=path; repeat for more roots"},
		&urfavecli.BoolFlag{Name: "read-only", Usage: "Disable every mutation for the whole server"},
		&urfavecli.BoolFlag{Name: "offline", Usage: "Open no network connection for the whole server"},
		&urfavecli.BoolFlag{Name: "allow-reveal", Usage: "Enable the otherwise absent irreversible forecast_reveal tool"},
		&urfavecli.IntFlag{Name: "max-concurrent", Value: 16, Usage: "Maximum concurrent MCP tool calls (1-256)"},
		&urfavecli.IntFlag{Name: "max-tool-bytes", Value: 8 << 20, Usage: "Maximum decoded tool argument bytes"},
	})
	serve.Description += "\n\nRead-only mode omits mutating tools from discovery; direct calls to omitted names return unknown-tool."
	serve.Description += " Ledger writers fail immediately on lock conflict; clients must serialize or use bounded retry with backoff."
	serve.Action = mcpServeAction
	return group("mcp", "Run the MCP adapter", serve)
}

func mcpServeAction(ctx context.Context, command *urfavecli.Command) error {
	runtime := RuntimeFromCommand(command)
	server, err := mcpadapter.New(mcpadapter.Config{
		LedgerRoots: command.StringSlice("ledger-root"), OutputRoots: command.StringSlice("output-root"), SecretRoots: command.StringSlice("secret-root"),
		Mode:          service.AccessMode{ReadOnly: command.Bool("read-only"), Offline: command.Bool("offline"), AllowReveal: command.Bool("allow-reveal")},
		Timeout:       runtime.Timeout,
		MaxConcurrent: command.Int("max-concurrent"), MaxToolBytes: command.Int("max-tool-bytes"), Stderr: command.Root().ErrWriter,
	})
	if err != nil {
		return err
	}
	return server.ServeStdio(ctx)
}

func versionCommand() *urfavecli.Command {
	return &urfavecli.Command{Name: "version", Usage: "Show build and contract versions", Description: "Example:\n  forecast-ledger version\n  forecast-ledger version --json",
		Flags: []urfavecli.Flag{&urfavecli.BoolFlag{Name: "json", Usage: "Write stable JSON metadata", Local: true}},
		Before: func(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
			if command.Bool("json") && (command.Root().Bool("plain") || command.Root().Bool("quiet")) {
				return ctx, app.NewError(app.CodeUsage, "--json, --plain, and --quiet cannot be combined", nil)
			}
			return ctx, nil
		},
		Action: func(_ context.Context, command *urfavecli.Command) error {
			info := buildinfo.Current()
			if command.Bool("json") || command.Root().Bool("json") {
				encoder := json.NewEncoder(command.Root().Writer)
				encoder.SetEscapeHTML(false)
				return encoder.Encode(info)
			}
			presenter := presenterFor(command)
			if presenter.Mode() == presentation.ModeQuiet {
				return nil
			}
			return writeVersionInfo(command.Root().Writer, info, presenter.Mode(), presenter.ColorEnabled())
		}}
}

func writeVersionInfo(writer io.Writer, info buildinfo.Info, mode presentation.Mode, color bool) error {
	placeholder := func(value string) string {
		if strings.TrimSpace(value) == "" || value == "unknown" {
			return "not set"
		}
		return value
	}
	providers := strings.Join(info.Timestamp.Providers, ", ")
	if providers == "" {
		providers = "not set"
	}
	fields := [][2]string{
		{"Binary", placeholder(info.Binary)},
		{"Version", placeholder(info.Version)},
		{"Source revision", placeholder(info.SourceRevision)},
		{"Go", placeholder(info.GoVersion)},
		{"Forecast Ledger schema", placeholder(info.Schema.Version)},
		{"Schema commit", placeholder(info.Schema.Commit)},
		{"Schema SHA-256", placeholder(info.Schema.SHA256)},
		{"MCP protocol", placeholder(info.MCPProtocol)},
		{"Timestamp support", fmt.Sprintf("%s/%s (experimental; %s=%s, retained CA bundle; local verification)", info.Timestamp.Protocol, info.Timestamp.HashAlgorithm, info.Timestamp.DefaultMode, providers)},
		{"Timestamp providers", providers},
	}
	for index, field := range fields {
		if index > 0 {
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
		if mode == presentation.ModeHuman && color {
			if _, err := fmt.Fprintf(writer, "\x1b[36m%s:\x1b[0m %s", field[0], field[1]); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(writer, "%s: %s", field[0], field[1]); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func formatQuestionView(mode presentation.Mode, view service.QuestionView) string {
	fields := [][2]string{
		{"id", string(view.ID)}, {"status", string(view.Status)}, {"created_at", string(view.CreatedAt)},
		{"current_revision_id", string(view.CurrentRevisionID)}, {"revisions", compactPublicJSON(view.Revisions)},
	}
	if view.Tags != nil {
		fields = append(fields, [2]string{"tags", compactPublicJSON(*view.Tags)})
	}
	if view.Notes != nil {
		fields = append(fields, [2]string{"notes", *view.Notes})
	}
	if view.Resolution != nil {
		fields = append(fields, [2]string{"resolution", compactPublicJSON(view.Resolution)})
	}
	var output strings.Builder
	writeDisplayFields(&output, mode, fields)
	for _, forecast := range view.Forecasts {
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		if mode == presentation.ModePlain {
			fmt.Fprintf(&output, "forecast\t%s\t%s\t%s\t%s\t%s", forecast.Summary.ID, forecast.Summary.QuestionRevisionID, forecast.Summary.Visibility, compactPublicJSON(forecast.Summary.Activity), compactPublicJSON(forecast.Summary.RepresentationKinds))
		} else {
			fmt.Fprintf(&output, "Forecast: %s (revision %s, %s, forecast integrity %s, activity %s)", forecast.Summary.ID, forecast.Summary.QuestionRevisionID, forecast.Summary.Visibility, forecast.Summary.IntegrityStatus, forecast.Summary.Activity.Coverage)
		}
	}
	return output.String()
}

func formatForecastView(mode presentation.Mode, view service.ForecastView) string {
	fields := [][2]string{
		{"id", string(view.Summary.ID)}, {"question_revision_id", string(view.Summary.QuestionRevisionID)}, {"forecasted_at", string(view.Summary.ForecastedAt)}, {"recorded_at", string(view.Summary.RecordedAt)},
		{"visibility", string(view.Summary.Visibility)}, {"integrity_status", string(view.Summary.IntegrityStatus)},
		{"activity", compactPublicJSON(view.Summary.Activity)}, {"integrity", compactPublicJSON(view.Integrity)},
	}
	if view.Representations != nil {
		fields = append(fields, [2]string{"representations", compactPublicJSON(view.Representations)})
	}
	if view.Rationale != nil {
		fields = append(fields, [2]string{"rationale", *view.Rationale})
	}
	if view.KeyFactors != nil {
		fields = append(fields, [2]string{"key_factors", compactPublicJSON(*view.KeyFactors)})
	}
	if view.Comment != nil {
		fields = append(fields, [2]string{"comment", *view.Comment})
	}
	if view.PublicNote != nil {
		fields = append(fields, [2]string{"public_note", *view.PublicNote})
	}
	if view.Summary.SupersedesForecastID != nil {
		fields = append(fields, [2]string{"supersedes_forecast_id", string(*view.Summary.SupersedesForecastID)})
	}
	if view.Commitment != nil {
		fields = append(fields, [2]string{"commitment", compactPublicJSON(view.Commitment)})
	}
	if view.LifecycleEvents != nil {
		fields = append(fields, [2]string{"lifecycle_events", compactPublicJSON(*view.LifecycleEvents)})
	}
	if view.ActivityCheckpoints != nil {
		fields = append(fields, [2]string{"activity_checkpoints", compactPublicJSON(*view.ActivityCheckpoints)})
	}
	var output strings.Builder
	writeDisplayFields(&output, mode, fields)
	return output.String()
}

func formatVerificationReport(mode presentation.Mode, report service.VerificationReport) string {
	var output strings.Builder
	if mode == presentation.ModePlain {
		fmt.Fprintf(&output, "overall\t%s\ndocument\t%s", report.Overall, report.Document.State)
		for _, forecast := range report.Forecasts {
			for _, layer := range forecast.Layers {
				fmt.Fprintf(&output, "\n%s\t%s\t%s\t%s\t%s\t%s", forecast.QuestionID, forecast.ForecastID, layer.Name, layer.State, strings.Join(layer.ReasonCodes, ","), compactPublicJSON(layer.Evidence))
			}
		}
		return output.String()
	}
	fmt.Fprintf(&output, "Overall: %s\nDocument: %s", report.Overall, report.Document.State)
	for _, forecast := range report.Forecasts {
		fmt.Fprintf(&output, "\nForecast: %s / %s", forecast.QuestionID, forecast.ForecastID)
		for _, layer := range forecast.Layers {
			fmt.Fprintf(&output, "\n  %s: %s", layer.Name, layer.State)
			if len(layer.ReasonCodes) > 0 {
				fmt.Fprintf(&output, " (%s)", strings.Join(layer.ReasonCodes, ", "))
			}
			if len(layer.Evidence) > 0 {
				fmt.Fprintf(&output, "\n    Evidence: %s", compactPublicJSON(layer.Evidence))
			}
			if len(layer.Limitations) > 0 {
				fmt.Fprintf(&output, "\n    Limits: %s", strings.Join(layer.Limitations, " "))
			}
		}
	}
	return output.String()
}

func formatTimestampVerification(mode presentation.Mode, result service.TimestampVerifyResult) string {
	if mode == presentation.ModePlain {
		return fmt.Sprintf("scope\t%s\nhead_event_id\t%s\nstate\t%s\nverification\t%s\t%s\ntimestamps\t%s",
			result.Scope, result.HeadEventID, result.State, result.Verification.State, strings.Join(result.Verification.ReasonCodes, ","), compactPublicJSON(result.Entries))
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Scope: %s", result.Scope)
	if result.HeadEventID != "" {
		fmt.Fprintf(&output, "\nHead event: %s", result.HeadEventID)
	}
	fmt.Fprintf(&output, "\nState: %s\nVerification: %s", result.State, result.Verification.State)
	if len(result.Verification.ReasonCodes) > 0 {
		fmt.Fprintf(&output, " (%s)", strings.Join(result.Verification.ReasonCodes, ", "))
	}
	fmt.Fprintf(&output, "\nTimestamp entries: %s", compactPublicJSON(result.Entries))
	return output.String()
}

func formatTimestampArtifact(mode presentation.Mode, result service.TimestampArtifactResult) string {
	if mode == presentation.ModePlain {
		return fmt.Sprintf("scope\t%s\nhead_event_id\t%s\nstate\t%s\nselection\t%s\t%s\nrequests\t%d\nattempts\t%s\ntimestamps\t%s",
			result.Scope, result.HeadEventID, result.State, result.SelectionMode, result.SelectedProvider, result.RequestSummary.RequestCount, compactPublicJSON(result.Attempts), compactPublicJSON(result.Entries))
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Scope: %s", result.Scope)
	if result.HeadEventID != "" {
		fmt.Fprintf(&output, "\nHead event: %s", result.HeadEventID)
	}
	fmt.Fprintf(&output, "\nState: %s", result.State)
	if result.SelectionMode != "" {
		fmt.Fprintf(&output, "\nSelection: %s", result.SelectionMode)
	}
	if result.SelectedProvider != "" {
		fmt.Fprintf(&output, "\nSelected provider: %s", result.SelectedProvider)
	}
	fmt.Fprintf(&output, "\nRequests: %d", result.RequestSummary.RequestCount)
	if len(result.Attempts) > 0 {
		fmt.Fprintf(&output, "\nAttempts: %s", compactPublicJSON(result.Attempts))
	}
	fmt.Fprintf(&output, "\nTimestamp entries: %s", compactPublicJSON(result.Entries))
	return output.String()
}

func formatPublicationVerification(mode presentation.Mode, result service.PublicationVerifyResult) string {
	var output strings.Builder
	if mode == presentation.ModePlain {
		fmt.Fprintf(&output, "overall\t%s\nmanifest\t%s\nfiles\t%d\nbytes\t%d", result.Overall, result.ManifestSHA256, result.FileCount, result.TotalBytes)
		for _, file := range result.Files {
			fmt.Fprintf(&output, "\nfile\t%s\t%s\t%d\t%s", file.Role, file.Path, file.Size, file.SHA256)
		}
		for _, forecast := range result.Evidence {
			for _, layer := range forecast.Layers {
				fmt.Fprintf(&output, "\n%s\t%s\t%s\t%s", forecast.QuestionID, forecast.ForecastID, layer.Name, layer.State)
			}
		}
		return output.String()
	}
	fmt.Fprintf(&output, "Overall: %s\nManifest SHA-256: %s\nFiles: %d\nBytes: %d", result.Overall, result.ManifestSHA256, result.FileCount, result.TotalBytes)
	for _, file := range result.Files {
		fmt.Fprintf(&output, "\nFile: %s (%s, %d bytes, %s)", file.Path, file.Role, file.Size, file.SHA256)
	}
	for _, forecast := range result.Evidence {
		fmt.Fprintf(&output, "\nForecast: %s / %s", forecast.QuestionID, forecast.ForecastID)
		for _, layer := range forecast.Layers {
			fmt.Fprintf(&output, "\n  %s: %s", layer.Name, layer.State)
			if len(layer.ReasonCodes) > 0 {
				fmt.Fprintf(&output, " (%s)", strings.Join(layer.ReasonCodes, ", "))
			}
		}
	}
	return output.String()
}

func writeDisplayFields(output *strings.Builder, mode presentation.Mode, fields [][2]string) {
	for index, field := range fields {
		if index > 0 {
			output.WriteByte('\n')
		}
		if mode == presentation.ModePlain {
			fmt.Fprintf(output, "%s\t%s", field[0], field[1])
		} else {
			label := strings.ReplaceAll(field[0], "_", " ")
			fmt.Fprintf(output, "%s: %s", strings.ToUpper(label[:1])+label[1:], field[1])
		}
	}
}

func compactPublicJSON(value any) string {
	redacted, err := presentation.Redact(value)
	if err != nil {
		return ""
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func group(name, usage string, children ...*urfavecli.Command) *urfavecli.Command {
	return &urfavecli.Command{Name: name, Usage: usage, Commands: children, Action: func(ctx context.Context, command *urfavecli.Command) error {
		if command.NArg() > 0 {
			err := urfavecli.ShowCommandHelp(ctx, command, command.Args().First())
			return app.NewError(app.CodeUsage, err.Error(), err)
		}
		return urfavecli.ShowSubcommandHelp(command)
	}}
}

func leaf(name, usage, example string, readOnly bool, flags []urfavecli.Flag) *urfavecli.Command {
	if !readOnly {
		flags = append(flags, &urfavecli.BoolFlag{Name: "dry-run", Usage: "Validate and show the planned change without writing"})
	}
	return &urfavecli.Command{Name: name, Usage: usage, Description: "Example:\n  " + example, Flags: flags, Before: admitSupportedLedger, DisableSliceFlagSeparator: true}
}

func admitSupportedLedger(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
	path := command.String("file")
	if command.Name == "init" || path == "" || path == "-" || strings.HasSuffix(command.FullName(), " publish verify") {
		return ctx, nil
	}
	operationContext, cancel := RuntimeFromCommand(command).Context(ctx)
	defer cancel()
	_, err := service.LoadAndValidateLedger(operationContext, path, nil)
	return ctx, err
}

func fileFlag(allowStdin bool) *urfavecli.StringFlag {
	usage := "Ledger file path"
	if allowStdin {
		usage += "; use - for ledger bytes on stdin (sibling artifacts are unavailable)"
	}
	return &urfavecli.StringFlag{Name: "file", Aliases: []string{"f"}, Required: true, OnlyOnce: true, TakesFile: true, Usage: usage,
		Validator: func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("file path is empty")
			}
			if value == "-" && !allowStdin {
				return errors.New("--file - is only available for eligible read-only commands")
			}
			return nil
		}}
}

func questionFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "question", Required: true, OnlyOnce: true, Usage: "Stable question ID"}
}
func forecastFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "forecast", Required: true, OnlyOnce: true, Usage: "Stable forecast ID"}
}
func platformFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "platform", Required: true, OnlyOnce: true, Usage: "Stable platform ID"}
}

func groupFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "group", Required: true, OnlyOnce: true, Usage: "Stable group ID"}
}

func relationshipFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "relationship", Required: true, OnlyOnce: true, Usage: "Stable relationship ID"}
}
func secretOutputFlag() *urfavecli.StringFlag {
	return &urfavecli.StringFlag{Name: "key-file", Required: true, OnlyOnce: true, TakesFile: true, Usage: "New protected key file"}
}
func requireTargetSelection(ctx context.Context, command *urfavecli.Command) (context.Context, error) {
	all := command.Bool("all")
	question := command.String("question")
	forecast := command.String("forecast")
	scope := command.String("scope")
	head := command.String("head")
	if scope != "forecast" && scope != "lifecycle" {
		return ctx, app.NewError(app.CodeUsage, "--scope must be forecast or lifecycle", nil)
	}
	if scope == "lifecycle" && (all || question == "" || forecast == "" || head == "") {
		return ctx, app.NewError(app.CodeUsage, "--scope lifecycle requires --question, --forecast, and --head and cannot use --all", nil)
	}
	if scope == "forecast" && head != "" {
		return ctx, app.NewError(app.CodeUsage, "--head requires --scope lifecycle", nil)
	}
	if all && (question != "" || forecast != "") {
		return ctx, app.NewError(app.CodeUsage, "--all cannot be combined with --question or --forecast", nil)
	}
	if !all && (question == "" || forecast == "") {
		return ctx, app.NewError(app.CodeUsage, "use --all or both --question and --forecast", nil)
	}
	return ctx, nil
}
