package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/presentation"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolInput struct {
	File                   string          `json:"file,omitempty"`
	Platform               string          `json:"platform,omitempty"`
	Question               string          `json:"question,omitempty"`
	QuestionRevision       string          `json:"question_revision_id,omitempty"`
	Forecast               string          `json:"forecast,omitempty"`
	Group                  string          `json:"group,omitempty"`
	Relationship           string          `json:"relationship_id,omitempty"`
	Request                json.RawMessage `json:"-"`
	SecretInputFile        string          `json:"secret_input_file,omitempty"`
	InitialSecretInputFile string          `json:"initial_secret_input_file,omitempty"`
	KeyFile                string          `json:"key_file,omitempty"`
	KeyHint                string          `json:"key_hint,omitempty"`
	LedgerID               string          `json:"ledger_id,omitempty"`
	Timezone               string          `json:"timezone,omitempty"`
	ForecasterID           string          `json:"forecaster_id,omitempty"`
	ForecasterName         string          `json:"forecaster_name,omitempty"`
	ForecasterKind         string          `json:"forecaster_kind,omitempty"`
	ForecastedAt           string          `json:"forecasted_at,omitempty"`
	RecordedAt             string          `json:"recorded_at,omitempty"`
	PublicNote             string          `json:"public_note,omitempty"`
	SupersedesForecastID   string          `json:"supersedes_forecast_id,omitempty"`
	Output                 string          `json:"output,omitempty"`
	Manifest               string          `json:"manifest,omitempty"`
	RevealedAt             string          `json:"revealed_at,omitempty"`
	TSAProvider            string          `json:"tsa_provider,omitempty"`
	TSAURL                 string          `json:"tsa_url,omitempty"`
	CABundle               string          `json:"ca_bundle,omitempty"`
	All                    bool            `json:"all,omitempty"`
	DryRun                 bool            `json:"dry_run,omitempty"`
	Confirm                bool            `json:"confirm,omitempty"`
	CheckSources           bool            `json:"check_sources,omitempty"`
}

type toolEnvelope struct {
	Operation service.OperationName `json:"operation"`
	Code      string                `json:"code"`
	Message   string                `json:"message"`
	Data      any                   `json:"data,omitempty"`
	Error     *app.Error            `json:"error,omitempty"`
}

func (s *Server) toolHandler(def service.OperationDefinition, allowed map[string]bool, required []string) sdk.ToolHandler {
	return func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		default:
			return errorToolResult(def.Name, app.NewError(app.CodeConflict, "MCP concurrent request limit is reached", nil)), nil
		}
		input, err := decodeToolArguments(request.Params.Arguments, s.maxToolBytes, def, allowed, required)
		if err != nil {
			return errorToolResult(def.Name, err), nil
		}
		data, err := s.dispatch(ctx, def, input)
		outcome := def.ClassifyOutcome(service.OutcomeInput{DryRun: input.DryRun, Data: data, Err: err})
		if err != nil && !outcome.HasData {
			return errorToolResult(def.Name, err), nil
		}
		envelope := toolEnvelope{Operation: def.Name, Code: outcome.Code, Message: outcome.Message, Data: data}
		return marshalToolResult(envelope, err != nil || outcome.FailureCode != ""), nil
	}
}

func decodeToolArguments(arguments json.RawMessage, maxBytes int, def service.OperationDefinition, allowed map[string]bool, required []string) (toolInput, error) {
	if len(arguments) > maxBytes {
		return toolInput{}, app.NewError(app.CodeUsage, "tool arguments exceed the size limit", nil)
	}
	var raw map[string]json.RawMessage
	if len(arguments) == 0 {
		raw = map[string]json.RawMessage{}
	} else {
		if _, err := document.ParseJSON(bytes.NewReader(arguments), document.DefaultLimits); err != nil {
			return toolInput{}, app.NewError(app.CodeUsage, "tool arguments are not valid bounded JSON", err)
		}
		if err := json.Unmarshal(arguments, &raw); err != nil {
			return toolInput{}, app.NewError(app.CodeUsage, "tool arguments are not valid JSON", err)
		}
	}
	for name := range raw {
		if !allowed[name] {
			return toolInput{}, app.WithDetails(app.NewError(app.CodeUsage, "tool arguments contain an unknown field", nil), map[string]any{"field": name})
		}
	}
	for _, name := range required {
		value, exists := raw[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return toolInput{}, app.WithDetails(app.NewError(app.CodeUsage, "tool argument is required", nil), map[string]any{"field": name})
		}
	}
	var input toolInput
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	if err := decoder.Decode(&input); err != nil {
		return toolInput{}, app.NewError(app.CodeUsage, "tool arguments have invalid types", err)
	}
	if def.RequestSchema != "" && def.RequestMode != service.RequestSecret {
		fields, err := requestSchemaFields(def.RequestSchema)
		if err != nil {
			return toolInput{}, err
		}
		request := make(map[string]json.RawMessage)
		for name := range fields {
			if value, present := raw[name]; present {
				request[name] = value
			}
		}
		input.Request, err = json.Marshal(request)
		if err != nil {
			return toolInput{}, app.NewError(app.CodeInternal, "direct MCP request cannot be assembled", err)
		}
	}
	return input, nil
}

func requestSchemaFields(name service.InputSchemaName) (map[string]bool, error) {
	schema, err := service.DirectRequestSchema(name)
	if err != nil {
		return nil, err
	}
	properties, _ := schema["properties"].(map[string]any)
	result := make(map[string]bool, len(properties))
	for field := range properties {
		result[field] = true
	}
	return result, nil
}

func (s *Server) dispatch(parent context.Context, def service.OperationDefinition, input toolInput) (any, error) {
	options := service.RequestOptions{DryRun: input.DryRun, Confirmed: input.Confirm || input.DryRun, Timeout: s.config.Timeout, Mode: s.config.Mode, Roots: s.roots.Public()}
	execution, err := service.PrepareExecution(parent, def.Name, options)
	if err != nil {
		return nil, err
	}
	defer execution.Close()
	ctx := execution.Context()
	fileMustExist := def.Name != service.OperationLedgerInit
	file := ""
	timezone := "UTC"
	if input.File != "" {
		fileRoot := service.RootLedger
		if def.Name == service.OperationPublicationVerify {
			fileRoot = service.RootOutput
		}
		label := "ledger file"
		if def.Name == service.OperationPublicationVerify {
			label = "packaged ledger"
		}
		file, err = s.roots.ResolveLabeled(fileRoot, input.File, fileMustExist, label)
		if err != nil {
			return nil, err
		}
	}
	if fileMustExist && file != "" && def.Name != service.OperationPublicationVerify {
		loaded, loadErr := service.LoadAndValidateLedger(ctx, file, nil)
		if loadErr != nil {
			return nil, loadErr
		}
		timezone = loaded.Model.DefaultTimezone
	}
	if def.Name == service.OperationLedgerInit {
		timezone = input.Timezone
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, app.NewError(app.CodeInvalidData, "default timezone is invalid", err)
	}
	now := ledger.Timestamp(s.effects.Clock.Now().In(location).Format(time.RFC3339))

	switch def.Name {
	case service.OperationLedgerInit:
		return s.dispatchInit(ctx, file, input, now)
	case service.OperationLedgerUpdate:
		var value service.RootMetadataPatchInput
		if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaRootMetadata, &value); err != nil {
			return nil, err
		}
		if input.DryRun {
			result, err := service.PlanRootMetadataFileUpdate(ctx, file, value)
			return result, err
		}
		result, err := service.CommitRootMetadataFileUpdate(ctx, file, value)
		return result, err
	case service.OperationLedgerValidate:
		loaded, err := service.LoadAndValidateLedger(ctx, file, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ledger_id": loaded.Model.LedgerID, "schema_version": loaded.Model.SchemaVersion}, nil
	case service.OperationLedgerStatus:
		loaded, err := service.LoadAndValidateLedger(ctx, file, nil)
		if err != nil {
			return nil, err
		}
		result, err := service.StatusForLedger(loaded)
		return result, err
	case service.OperationPlatformAdd, service.OperationPlatformUpdate:
		return dispatchPlatformMutation(ctx, def.Name, file, ledger.Slug(input.Platform), input.Request, input.DryRun)
	case service.OperationPlatformList:
		id, items, err := service.LoadPlatformList(ctx, file, nil)
		return map[string]any{"ledger_id": id, "platforms": items}, err
	case service.OperationPlatformShow:
		id, result, err := service.LoadPlatformShow(ctx, file, nil, ledger.Slug(input.Platform))
		return map[string]any{"ledger_id": id, "platform": result}, err
	case service.OperationPlatformRemove:
		if input.DryRun {
			result, err := service.PlanPlatformRemoveFile(ctx, file, ledger.Slug(input.Platform))
			return result, err
		}
		result, err := service.CommitPlatformRemoveFile(ctx, file, ledger.Slug(input.Platform))
		return result, err
	case service.OperationQuestionAdd:
		return s.dispatchQuestionAdd(ctx, file, input, now)
	case service.OperationQuestionUpdate:
		var value service.QuestionPatchInput
		if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaQuestionPatch, &value); err != nil {
			return nil, err
		}
		if input.DryRun {
			result, err := service.PlanQuestionUpdateFile(ctx, file, ledger.Slug(input.Question), value)
			return result, err
		}
		result, err := service.CommitQuestionUpdateFile(ctx, file, ledger.Slug(input.Question), value)
		return result, err
	case service.OperationQuestionRevise:
		var value service.RevisionInput
		if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaQuestionRevision, &value); err != nil {
			return nil, err
		}
		if input.DryRun {
			result, err := service.PlanQuestionReviseFile(ctx, file, ledger.Slug(input.Question), value, now)
			return result, err
		}
		result, err := service.CommitQuestionReviseFile(ctx, file, ledger.Slug(input.Question), value, now)
		return result, err
	case service.OperationQuestionList:
		id, items, err := service.LoadQuestionList(ctx, file, nil)
		return map[string]any{"ledger_id": id, "questions": items}, err
	case service.OperationQuestionShow:
		id, result, err := service.LoadQuestionShow(ctx, file, nil, ledger.Slug(input.Question))
		return map[string]any{"ledger_id": id, "question": result}, err
	case service.OperationQuestionResolve, service.OperationQuestionAmbiguous, service.OperationQuestionVoid, service.OperationQuestionDispute, service.OperationQuestionNotApplicable:
		return dispatchQuestionTerminal(ctx, def.Name, file, ledger.Slug(input.Question), input.Request, now, input.DryRun)
	case service.OperationGroupAdd, service.OperationGroupUpdate, service.OperationGroupList, service.OperationGroupShow, service.OperationGroupRemove:
		return dispatchGroup(ctx, def.Name, file, ledger.Slug(input.Group), input.Request, input.DryRun)
	case service.OperationRelationshipAdd, service.OperationRelationshipList, service.OperationRelationshipShow, service.OperationRelationshipRemove:
		return dispatchRelationship(ctx, def.Name, file, ledger.Slug(input.Relationship), input.Request, input.DryRun)
	case service.OperationForecastAdd:
		var value service.ForecastCreateInput
		if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaForecastCreate, &value); err != nil {
			return nil, err
		}
		if value.ForecastedAt == "" {
			value.ForecastedAt = now
		}
		if input.DryRun {
			result, err := service.PlanPublicForecastAddFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), value, now)
			return result, err
		}
		result, err := service.CommitPublicForecastAddFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), value, now)
		return result, err
	case service.OperationForecastList:
		id, items, err := service.LoadForecastList(ctx, file, nil, ledger.Slug(input.Question))
		return map[string]any{"ledger_id": id, "question_id": input.Question, "forecasts": items}, err
	case service.OperationForecastShow:
		id, result, err := service.LoadForecastShow(ctx, file, nil, ledger.Slug(input.Question), ledger.Slug(input.Forecast))
		return map[string]any{"ledger_id": id, "question_id": input.Question, "forecast": result}, err
	case service.OperationForecastSeal:
		return s.dispatchForecastSeal(ctx, file, input, now)
	case service.OperationForecastReveal:
		return s.dispatchForecastReveal(ctx, file, input, now)
	case service.OperationForecastWithdraw, service.OperationForecastExpire, service.OperationForecastReaffirm:
		var value service.LifecycleInput
		if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaLifecycle, &value); err != nil {
			return nil, err
		}
		eventType := ledger.LifecycleWithdrawn
		if def.Name == service.OperationForecastExpire {
			eventType = ledger.LifecycleExpired
		} else if def.Name == service.OperationForecastReaffirm {
			eventType = ledger.LifecycleReaffirmed
		}
		if input.DryRun {
			result, err := service.PlanForecastLifecycleFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), eventType, value, now)
			return result, err
		}
		result, err := service.CommitForecastLifecycleFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), eventType, value, now)
		return result, err
	case service.OperationForecastKeyHintUpdate:
		if input.DryRun {
			result, err := service.PlanForecastKeyHintUpdateFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), input.KeyHint)
			return result, err
		}
		result, err := service.CommitForecastKeyHintUpdateFile(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), input.KeyHint)
		return result, err
	case service.OperationTargetBuild:
		if input.DryRun {
			result, err := service.PlanTargetBuild(ctx, file, input.All, ledger.Slug(input.Question), ledger.Slug(input.Forecast))
			return result, err
		}
		result, err := service.CommitTargetBuild(ctx, file, input.All, ledger.Slug(input.Question), ledger.Slug(input.Forecast))
		return result, err
	case service.OperationTargetCheck:
		result, err := service.InspectTargets(ctx, file, input.All, ledger.Slug(input.Question), ledger.Slug(input.Forecast))
		if err != nil {
			return nil, err
		}
		return result, nil
	case service.OperationTimestampStamp:
		result, err := service.CommitTimestampStamp(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), service.TimestampStampOptions{DryRun: input.DryRun, Offline: s.config.Mode.Offline, TSAProvider: input.TSAProvider, TSAURL: input.TSAURL, CABundlePath: input.CABundle, Effects: s.effects})
		if err != nil && result.FailureCode == "" {
			return result, err
		}
		return result, nil
	case service.OperationTimestampStatus:
		result, err := service.TimestampStatusFor(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast))
		return result, err
	case service.OperationTimestampVerify:
		result, err := service.CommitTimestampVerify(ctx, file, ledger.Slug(input.Question), ledger.Slug(input.Forecast), service.TimestampVerifyOptions{DryRun: input.DryRun, Effects: s.effects})
		return result, err
	case service.OperationVerificationRun:
		result, err := service.VerifyLedgerEvidence(ctx, file, service.VerificationOptions{Offline: s.config.Mode.Offline, CheckSources: input.CheckSources, QuestionID: ledger.Slug(input.Question), ForecastID: ledger.Slug(input.Forecast)})
		return result, err
	case service.OperationPublicationBuild:
		output, err := s.roots.Resolve(service.RootOutput, input.Output, false)
		if err != nil {
			return nil, err
		}
		result, err := service.CommitPublicationBuild(ctx, file, output, input.DryRun)
		return result, err
	case service.OperationPublicationVerify:
		manifest, err := s.roots.ResolveLabeled(service.RootOutput, input.Manifest, true, "publication manifest")
		if err != nil {
			return nil, err
		}
		result, err := service.VerifyPublicationPackage(ctx, file, manifest)
		return result, err
	default:
		return nil, app.NewError(app.CodeUnavailable, "operation is not registered", nil)
	}
}

func (s *Server) dispatchInit(ctx context.Context, file string, input toolInput, operationAt ledger.Timestamp) (any, error) {
	if input.InitialSecretInputFile == "" && directVisibility(input.Request, true) == ledger.VisibilitySealed {
		return nil, app.NewError(app.CodeUsage, "sealed initialization must use initial_secret_input_file", nil)
	}
	var value service.InitInput
	if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaInit, &value); err != nil {
		return nil, err
	}
	if input.InitialSecretInputFile != "" {
		if value.Question == nil || value.Question.InitialForecast == nil || value.Question.InitialForecast.Visibility != ledger.VisibilitySealed {
			return nil, app.NewError(app.CodeUsage, "initial_secret_input_file requires direct sealed initial-forecast metadata", nil)
		}
		var private service.SealedForecastPrivateInput
		if err := s.decodeProtected(ctx, input.InitialSecretInputFile, service.InputSchemaForecastSealPrivate, &private); err != nil {
			return nil, err
		}
		mergeInitialForecastPrivate(value.Question.InitialForecast, private)
	}
	if value.Question != nil && value.Question.InitialForecast != nil && value.Question.InitialForecast.ForecastedAt == "" {
		value.Question.InitialForecast.ForecastedAt = operationAt
	}
	root, err := service.BuildLedgerRootAt(service.InitRootRequest{LedgerID: ledger.Slug(input.LedgerID), Timezone: input.Timezone, ForecasterID: ledger.Slug(input.ForecasterID), ForecasterName: input.ForecasterName, ForecasterKind: ledger.ForecasterKind(input.ForecasterKind), Input: value}, operationAt)
	if err != nil {
		return nil, err
	}
	shape, err := service.ClassifyInitInput(value)
	if err != nil {
		return nil, err
	}
	var model *ledger.Ledger
	var sealed service.SealedInitialBuild
	keyPath := ""
	if input.KeyFile != "" {
		keyPath, err = s.roots.Resolve(service.RootSecret, input.KeyFile, false)
		if err != nil {
			return nil, err
		}
	}
	if shape != service.CreationSealedForecast && keyPath != "" {
		return nil, app.NewError(app.CodeUsage, "key_file is only valid for a sealed initial forecast", nil)
	}
	switch shape {
	case service.CreationLedgerOnly:
		model = root
	case service.CreationQuestionOnly:
		model, err = service.BuildInitialQuestionLedgerAt(root, *value.Question, operationAt)
	case service.CreationPublicForecast:
		model, err = service.BuildInitialPublicLedgerAt(root, *value.Question, operationAt)
	case service.CreationSealedForecast:
		if input.InitialSecretInputFile == "" || keyPath == "" {
			return nil, app.NewError(app.CodeUsage, "sealed initialization requires initial_secret_input_file and key_file in a secret root", nil)
		}
		if input.DryRun {
			model, err = service.PlanInitialSealedLedgerAt(root, *value.Question, operationAt)
		} else {
			sealed, err = service.BuildInitialSealedLedgerAt(ctx, root, *value.Question, operationAt, s.effects)
			model = sealed.Ledger
		}
	}
	if err != nil {
		return nil, err
	}
	if input.DryRun {
		if _, err := service.EncodeNewLedger(model, file); err != nil {
			return nil, err
		}
		effects := []service.SideEffect{{Kind: service.EffectLedger, Action: service.EffectCreate, Status: service.EffectDeferred, Path: filepath.Base(file), Owned: true, Rollback: service.RollbackCreatedPublic}}
		if shape == service.CreationSealedForecast {
			effects = append([]service.SideEffect{{Kind: service.EffectKey, Action: service.EffectCreate, Status: service.EffectDeferred, Path: filepath.Base(keyPath), Owned: true, Rollback: service.RollbackRetainSecret}}, effects...)
		}
		return service.NewInitResult(model, effects, service.Recovery{State: service.RecoveryNone}), nil
	}
	recovery := service.Recovery{State: service.RecoveryNone}
	if shape == service.CreationSealedForecast {
		commit, err := service.CommitInitialSealedFiles(ctx, file, keyPath, sealed, service.InitialCommitOptions{})
		recovery = commit.Recovery
		if err != nil {
			return service.NewInitResult(model, nil, recovery), err
		}
	} else {
		if _, err = service.CommitNewLedger(file, model); err != nil {
			return nil, err
		}
	}
	effects := []service.SideEffect{{Kind: service.EffectLedger, Action: service.EffectCreate, Status: service.EffectCompleted, Path: filepath.Base(file), Owned: true, Rollback: service.RollbackCreatedPublic}}
	if shape == service.CreationSealedForecast {
		effects = append([]service.SideEffect{{Kind: service.EffectKey, Action: service.EffectCreate, Status: service.EffectCompleted, Path: filepath.Base(keyPath), Owned: true, Rollback: service.RollbackRetainSecret}}, effects...)
	}
	return service.NewInitResult(model, effects, recovery), nil
}

func dispatchPlatformMutation(ctx context.Context, operation service.OperationName, file string, id ledger.Slug, raw json.RawMessage, dryRun bool) (any, error) {
	if operation == service.OperationPlatformAdd {
		var value service.PlatformCreateInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaPlatformCreate, &value); err != nil {
			return nil, err
		}
		if dryRun {
			result, err := service.PlanPlatformAddFile(ctx, file, id, value)
			return result, err
		}
		result, err := service.CommitPlatformAddFile(ctx, file, id, value)
		return result, err
	}
	var value service.PlatformPatchInput
	if err := decodeDirectRequest(ctx, raw, service.InputSchemaPlatformPatch, &value); err != nil {
		return nil, err
	}
	if dryRun {
		result, err := service.PlanPlatformUpdateFile(ctx, file, id, value)
		return result, err
	}
	result, err := service.CommitPlatformUpdateFile(ctx, file, id, value)
	return result, err
}

func (s *Server) dispatchQuestionAdd(ctx context.Context, file string, input toolInput, now ledger.Timestamp) (any, error) {
	if input.InitialSecretInputFile == "" && directVisibility(input.Request, false) == ledger.VisibilitySealed {
		return nil, app.NewError(app.CodeUsage, "a sealed first forecast must use initial_secret_input_file", nil)
	}
	var value service.QuestionAddInput
	if err := decodeDirectRequest(ctx, input.Request, service.InputSchemaQuestionAdd, &value); err != nil {
		return nil, err
	}
	if input.InitialSecretInputFile != "" {
		if value.InitialForecast == nil || value.InitialForecast.Visibility != ledger.VisibilitySealed {
			return nil, app.NewError(app.CodeUsage, "initial_secret_input_file requires direct sealed initial-forecast metadata", nil)
		}
		var private service.SealedForecastPrivateInput
		if err := s.decodeProtected(ctx, input.InitialSecretInputFile, service.InputSchemaForecastSealPrivate, &private); err != nil {
			return nil, err
		}
		mergeInitialForecastPrivate(value.InitialForecast, private)
	}
	if value.InitialForecast != nil && value.InitialForecast.ForecastedAt == "" {
		value.InitialForecast.ForecastedAt = now
	}
	normalized := service.NormalizedQuestionCreate{ID: ledger.Slug(input.Question), Input: value}
	shape, err := service.ClassifyQuestionAddInput(value)
	if err != nil {
		return nil, err
	}
	if shape != service.CreationSealedForecast && input.KeyFile != "" {
		return nil, app.NewError(app.CodeUsage, "key_file is only valid for a sealed initial forecast", nil)
	}
	if shape == service.CreationQuestionOnly {
		if input.DryRun {
			result, err := service.PlanQuestionAddEmptyFile(ctx, file, normalized, now)
			return result, err
		}
		result, err := service.CommitQuestionAddEmptyFile(ctx, file, normalized, now)
		return result, err
	}
	if shape == service.CreationPublicForecast {
		if input.DryRun {
			result, err := service.PlanQuestionAddPublicFile(ctx, file, normalized, now)
			return result, err
		}
		result, err := service.CommitQuestionAddPublicFile(ctx, file, normalized, now)
		return result, err
	}
	if shape != service.CreationSealedForecast || input.InitialSecretInputFile == "" || input.KeyFile == "" {
		return nil, app.NewError(app.CodeUsage, "a sealed first forecast requires initial_secret_input_file and key_file in a secret root", nil)
	}
	keyPath, err := s.roots.Resolve(service.RootSecret, input.KeyFile, false)
	if err != nil {
		return nil, err
	}
	if input.DryRun {
		result, err := service.PlanQuestionAddSealedFile(ctx, file, keyPath, normalized, now)
		return result, err
	}
	result, err := service.CommitQuestionAddSealedFile(ctx, file, keyPath, normalized, now, s.effects)
	return result, err
}

func dispatchQuestionTerminal(ctx context.Context, operation service.OperationName, file string, id ledger.Slug, raw json.RawMessage, now ledger.Timestamp, dryRun bool) (any, error) {
	switch operation {
	case service.OperationQuestionResolve:
		var value service.ResolutionInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaResolution, &value); err != nil {
			return nil, err
		}
		if dryRun {
			result, err := service.PlanQuestionResolveFile(ctx, file, id, value, now)
			return result, err
		}
		result, err := service.CommitQuestionResolveFile(ctx, file, id, value, now)
		return result, err
	case service.OperationQuestionAmbiguous, service.OperationQuestionVoid, service.OperationQuestionDispute:
		var value service.UnresolvedResolutionInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaUnresolvedResolution, &value); err != nil {
			return nil, err
		}
		status := ledger.ResolutionDisputed
		if operation == service.OperationQuestionAmbiguous {
			status = ledger.ResolutionAmbiguous
		} else if operation == service.OperationQuestionVoid {
			status = ledger.ResolutionVoid
		}
		if dryRun {
			result, err := service.PlanQuestionUnresolvedFile(ctx, file, id, status, value, now)
			return result, err
		}
		result, err := service.CommitQuestionUnresolvedFile(ctx, file, id, status, value, now)
		return result, err
	default:
		var value service.NotApplicableInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaNotApplicable, &value); err != nil {
			return nil, err
		}
		if dryRun {
			result, err := service.PlanQuestionNotApplicableFile(ctx, file, id, value, now)
			return result, err
		}
		result, err := service.CommitQuestionNotApplicableFile(ctx, file, id, value, now)
		return result, err
	}
}

func dispatchGroup(ctx context.Context, operation service.OperationName, file string, id ledger.Slug, raw json.RawMessage, dryRun bool) (any, error) {
	switch operation {
	case service.OperationGroupAdd:
		var value service.GroupCreateInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaGroupCreate, &value); err != nil {
			return nil, err
		}
		if dryRun {
			return service.PlanGroupAddFile(ctx, file, id, value)
		}
		return service.CommitGroupAddFile(ctx, file, id, value)
	case service.OperationGroupUpdate:
		var value service.GroupPatchInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaGroupPatch, &value); err != nil {
			return nil, err
		}
		if dryRun {
			return service.PlanGroupUpdateFile(ctx, file, id, value)
		}
		return service.CommitGroupUpdateFile(ctx, file, id, value)
	case service.OperationGroupList:
		ledgerID, values, err := service.LoadGroupList(ctx, file, nil)
		return map[string]any{"ledger_id": ledgerID, "groups": values}, err
	case service.OperationGroupShow:
		ledgerID, value, err := service.LoadGroupShow(ctx, file, nil, id)
		return map[string]any{"ledger_id": ledgerID, "group": value}, err
	default:
		if dryRun {
			return service.PlanGroupRemoveFile(ctx, file, id)
		}
		return service.CommitGroupRemoveFile(ctx, file, id)
	}
}

func dispatchRelationship(ctx context.Context, operation service.OperationName, file string, id ledger.Slug, raw json.RawMessage, dryRun bool) (any, error) {
	switch operation {
	case service.OperationRelationshipAdd:
		var value service.RelationshipInput
		if err := decodeDirectRequest(ctx, raw, service.InputSchemaRelationship, &value); err != nil {
			return nil, err
		}
		if dryRun {
			return service.PlanRelationshipAddFile(ctx, file, value)
		}
		return service.CommitRelationshipAddFile(ctx, file, value)
	case service.OperationRelationshipList:
		ledgerID, values, err := service.LoadRelationshipList(ctx, file, nil)
		return map[string]any{"ledger_id": ledgerID, "relationships": values}, err
	case service.OperationRelationshipShow:
		ledgerID, value, err := service.LoadRelationshipShow(ctx, file, nil, id)
		return map[string]any{"ledger_id": ledgerID, "relationship": value}, err
	default:
		if dryRun {
			return service.PlanRelationshipRemoveFile(ctx, file, id)
		}
		return service.CommitRelationshipRemoveFile(ctx, file, id)
	}
}

func (s *Server) dispatchForecastSeal(ctx context.Context, file string, input toolInput, now ledger.Timestamp) (any, error) {
	if input.SecretInputFile == "" || input.KeyFile == "" {
		return nil, app.NewError(app.CodeUsage, "forecast_seal requires secret_input_file and key_file references", nil)
	}
	var private service.SealedForecastPrivateInput
	if err := s.decodeProtected(ctx, input.SecretInputFile, service.InputSchemaForecastSealPrivate, &private); err != nil {
		return nil, err
	}
	value := service.SealedForecastInput{QuestionRevisionID: ledger.Slug(input.QuestionRevision), Representations: private.Representations, Rationale: private.Rationale, KeyFactors: private.KeyFactors, Comment: private.Comment}
	if input.ForecastedAt != "" {
		value.ForecastedAt = ledger.Timestamp(input.ForecastedAt)
	} else if value.ForecastedAt == "" {
		value.ForecastedAt = now
	}
	if input.RecordedAt != "" {
		recorded := ledger.Timestamp(input.RecordedAt)
		value.RecordedAt = &recorded
	}
	if input.PublicNote != "" {
		value.PublicNote = &input.PublicNote
	}
	if input.SupersedesForecastID != "" {
		supersedes := ledger.Slug(input.SupersedesForecastID)
		value.SupersedesForecastID = &supersedes
	}
	keyPath, err := s.roots.Resolve(service.RootSecret, input.KeyFile, false)
	if err != nil {
		return nil, err
	}
	if input.DryRun {
		result, err := service.PlanForecastSealFile(ctx, file, keyPath, ledger.Slug(input.Question), ledger.Slug(input.Forecast), value, now)
		return result, err
	}
	result, err := service.CommitForecastSealFile(ctx, file, keyPath, ledger.Slug(input.Question), ledger.Slug(input.Forecast), value, now, s.effects)
	return result, err
}

func (s *Server) dispatchForecastReveal(ctx context.Context, file string, input toolInput, now ledger.Timestamp) (any, error) {
	keyPath, err := s.roots.ResolveLabeled(service.RootSecret, input.KeyFile, true, "key file")
	if err != nil {
		return nil, err
	}
	revealedAt := now
	if input.RevealedAt != "" {
		revealedAt, err = optionalTimestamp(input.RevealedAt, "revealed_at")
		if err != nil {
			return nil, err
		}
	}
	if input.DryRun {
		result, err := service.PlanForecastRevealFile(ctx, file, keyPath, ledger.Slug(input.Question), ledger.Slug(input.Forecast), revealedAt)
		return result, err
	}
	result, err := service.CommitForecastRevealFile(ctx, file, keyPath, ledger.Slug(input.Question), ledger.Slug(input.Forecast), revealedAt)
	return result, err
}

func (s *Server) decodeProtected(ctx context.Context, reference string, schema service.InputSchemaName, destination any) error {
	path, err := s.roots.ResolveLabeled(service.RootSecret, reference, true, "protected input file")
	if err != nil {
		return err
	}
	data, err := storage.ReadProtectedFile(path, 8<<20, "protected input file")
	if err != nil {
		return err
	}
	defer clear(data)
	return service.DecodeOperationInput(ctx, "-", bytes.NewReader(data), schema, destination)
}

func decodeDirectRequest(ctx context.Context, raw json.RawMessage, schema service.InputSchemaName, destination any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return app.NewError(app.CodeUsage, "direct request fields are required", nil)
	}
	return service.DecodeOperationInput(ctx, "-", bytes.NewReader(raw), schema, destination)
}

func optionalTimestamp(value, field string) (ledger.Timestamp, error) {
	if value == "" {
		return "", nil
	}
	result := ledger.Timestamp(value)
	if _, err := service.ParseTimestamp(result, field); err != nil {
		return "", err
	}
	return result, nil
}

func directVisibility(raw json.RawMessage, init bool) ledger.ForecastVisibility {
	var envelope struct {
		Question *struct {
			InitialForecast struct {
				Visibility ledger.ForecastVisibility `json:"visibility"`
			} `json:"initial_forecast"`
		} `json:"question"`
		InitialForecast struct {
			Visibility ledger.ForecastVisibility `json:"visibility"`
		} `json:"initial_forecast"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &envelope) != nil {
		return ""
	}
	if init && envelope.Question != nil {
		return envelope.Question.InitialForecast.Visibility
	}
	return envelope.InitialForecast.Visibility
}

func mergeInitialForecastPrivate(target *service.InitialForecastInput, private service.SealedForecastPrivateInput) {
	if target == nil {
		return
	}
	target.Representations = append([]ledger.ForecastRepresentation(nil), private.Representations...)
	target.Rationale = &private.Rationale
	target.KeyFactors = &private.KeyFactors
	target.Comment = &private.Comment
}

func errorToolResult(operation service.OperationName, err error) *sdk.CallToolResult {
	var applicationErr *app.Error
	if !errors.As(err, &applicationErr) {
		applicationErr = app.NewError(app.CodeInternal, "internal operation error", err)
	}
	safe := *applicationErr
	safe.Cause = nil
	return marshalToolResult(toolEnvelope{Operation: operation, Code: string(safe.Code), Message: safe.Message, Error: &safe}, true)
}

func marshalToolResult(value toolEnvelope, isError bool) *sdk.CallToolResult {
	safe, err := presentation.Redact(value)
	if err != nil {
		safe = map[string]any{"operation": "unknown", "code": "internal", "message": "tool result could not be encoded"}
		isError = true
	}
	data, err := json.Marshal(safe)
	if err != nil {
		data = []byte(`{"operation":"unknown","code":"internal","message":"tool result could not be encoded"}`)
		safe = map[string]any{"operation": "unknown", "code": "internal", "message": "tool result could not be encoded"}
		isError = true
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(data)}}, StructuredContent: safe, IsError: isError}
}
