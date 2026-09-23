package mcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/buildinfo"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const ProtocolRevision = buildinfo.MCPProtocolVersion

type Config struct {
	LedgerRoots   []string
	OutputRoots   []string
	SecretRoots   []string
	Mode          service.AccessMode
	Timeout       time.Duration
	MaxConcurrent int
	MaxToolBytes  int
	Stderr        io.Writer
	Effects       service.Effects
}

type Server struct {
	config       Config
	roots        *RootSet
	effects      service.Effects
	sdk          *sdk.Server
	sem          chan struct{}
	maxToolBytes int
}

func New(config Config) (*Server, error) {
	if config.Mode.AllowReveal && config.Mode.ReadOnly {
		return nil, app.WithDetails(app.NewError(app.CodeUsage, "--allow-reveal cannot be combined with --read-only", nil), map[string]any{"flags": []string{"--allow-reveal", "--read-only"}})
	}
	if config.Mode.AllowReveal && len(config.SecretRoots) == 0 {
		return nil, app.WithDetails(app.NewError(app.CodeUsage, "--allow-reveal requires at least one secret root", nil), map[string]any{"class": service.RootSecret, "flag": "--secret-root"})
	}
	roots, err := NewRootSet(config.LedgerRoots, config.OutputRoots, config.SecretRoots)
	if err != nil {
		return nil, err
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 16
	}
	if config.MaxToolBytes == 0 {
		config.MaxToolBytes = 8 << 20
	}
	if config.MaxConcurrent < 1 || config.MaxConcurrent > 256 || config.MaxToolBytes < 1024 || config.MaxToolBytes > 64<<20 {
		return nil, app.NewError(app.CodeUsage, "MCP resource limits are outside the supported range", nil)
	}
	if config.Stderr == nil {
		config.Stderr = io.Discard
	}
	effects := config.Effects
	if err := effects.Validate(); err != nil {
		effects = service.ProductionEffects()
	}
	info := buildinfo.Current()
	mode := "online"
	if config.Mode.Offline {
		mode = "offline"
	}
	access := "read-write"
	if config.Mode.ReadOnly {
		access = "read-only"
	}
	instructions := fmt.Sprintf("Forecast Ledger MCP. schema=%s schema_commit=%s schema_sha256=%s mode=%s access=%s timestamps=experimental timestamp_protocol=%s timestamp_hash=%s mcp_protocol=%s", info.Schema.Version, info.Schema.Commit, info.Schema.SHA256, mode, access, info.Timestamp.Protocol, info.Timestamp.HashAlgorithm, info.MCPProtocol)
	logger := slog.New(slog.NewTextHandler(config.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	server := sdk.NewServer(&sdk.Implementation{Name: info.Binary, Version: info.Version}, &sdk.ServerOptions{Instructions: instructions, Logger: logger, PageSize: 100})
	result := &Server{config: config, roots: roots, effects: effects, sdk: server, sem: make(chan struct{}, config.MaxConcurrent), maxToolBytes: config.MaxToolBytes}
	if err := result.registerTools(); err != nil {
		return nil, err
	}
	result.registerResources()
	return result, nil
}

func (s *Server) SDK() *sdk.Server { return s.sdk }

func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	if s == nil || s.sdk == nil {
		return app.NewError(app.CodeInternal, "MCP server is not initialized", nil)
	}
	return s.sdk.Run(ctx, transport)
}

func (s *Server) ServeStdio(ctx context.Context) error {
	return s.Run(ctx, &sdk.StdioTransport{})
}

type toolContract struct {
	Allowed  []string
	Required []string
}

func contracts() map[service.OperationName]toolContract {
	file := []string{"file"}
	return map[service.OperationName]toolContract{
		service.OperationLedgerInit:            {Allowed: []string{"file", "ledger_id", "timezone", "forecaster_id", "forecaster_name", "forecaster_kind", "initial_secret_input_file", "key_file", "dry_run"}, Required: []string{"file", "ledger_id", "timezone", "forecaster_id", "forecaster_name"}},
		service.OperationLedgerUpdate:          {Allowed: append(file, "dry_run"), Required: file},
		service.OperationLedgerValidate:        {Allowed: file, Required: file},
		service.OperationLedgerStatus:          {Allowed: file, Required: file},
		service.OperationPlatformAdd:           {Allowed: append(file, "platform", "dry_run"), Required: []string{"file", "platform"}},
		service.OperationPlatformUpdate:        {Allowed: append(file, "platform", "dry_run"), Required: []string{"file", "platform"}},
		service.OperationPlatformList:          {Allowed: file, Required: file},
		service.OperationPlatformShow:          {Allowed: append(file, "platform"), Required: []string{"file", "platform"}},
		service.OperationPlatformRemove:        {Allowed: append(file, "platform", "confirm", "dry_run"), Required: []string{"file", "platform", "confirm"}},
		service.OperationQuestionAdd:           {Allowed: append(file, "question", "initial_secret_input_file", "key_file", "dry_run"), Required: []string{"file", "question"}},
		service.OperationQuestionUpdate:        {Allowed: append(file, "question", "dry_run"), Required: []string{"file", "question"}},
		service.OperationQuestionRevise:        {Allowed: append(file, "question", "dry_run"), Required: []string{"file", "question"}},
		service.OperationQuestionList:          {Allowed: file, Required: file},
		service.OperationQuestionShow:          {Allowed: append(file, "question"), Required: []string{"file", "question"}},
		service.OperationQuestionResolve:       {Allowed: append(file, "question", "confirm", "dry_run"), Required: []string{"file", "question", "confirm"}},
		service.OperationQuestionAmbiguous:     {Allowed: append(file, "question", "confirm", "dry_run"), Required: []string{"file", "question", "confirm"}},
		service.OperationQuestionVoid:          {Allowed: append(file, "question", "confirm", "dry_run"), Required: []string{"file", "question", "confirm"}},
		service.OperationQuestionDispute:       {Allowed: append(file, "question", "confirm", "dry_run"), Required: []string{"file", "question", "confirm"}},
		service.OperationQuestionNotApplicable: {Allowed: append(file, "question", "confirm", "dry_run"), Required: []string{"file", "question", "confirm"}},
		service.OperationGroupAdd:              {Allowed: append(file, "group", "dry_run"), Required: []string{"file", "group"}},
		service.OperationGroupUpdate:           {Allowed: append(file, "group", "dry_run"), Required: []string{"file", "group"}},
		service.OperationGroupList:             {Allowed: file, Required: file},
		service.OperationGroupShow:             {Allowed: append(file, "group"), Required: []string{"file", "group"}},
		service.OperationGroupRemove:           {Allowed: append(file, "group", "confirm", "dry_run"), Required: []string{"file", "group", "confirm"}},
		service.OperationRelationshipAdd:       {Allowed: append(file, "dry_run"), Required: file},
		service.OperationRelationshipList:      {Allowed: file, Required: file},
		service.OperationRelationshipShow:      {Allowed: append(file, "relationship_id"), Required: []string{"file", "relationship_id"}},
		service.OperationRelationshipRemove:    {Allowed: append(file, "relationship_id", "confirm", "dry_run"), Required: []string{"file", "relationship_id", "confirm"}},
		service.OperationForecastAdd:           {Allowed: append(file, "question", "forecast", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationForecastList:          {Allowed: append(file, "question"), Required: []string{"file", "question"}},
		service.OperationForecastShow:          {Allowed: append(file, "question", "forecast"), Required: []string{"file", "question", "forecast"}},
		service.OperationForecastSeal:          {Allowed: append(file, "question", "forecast", "question_revision_id", "secret_input_file", "forecasted_at", "recorded_at", "public_note", "supersedes_forecast_id", "key_file", "dry_run"), Required: []string{"file", "question", "forecast", "question_revision_id", "secret_input_file", "key_file"}},
		service.OperationForecastReveal:        {Allowed: append(file, "question", "forecast", "key_file", "revealed_at", "confirm", "dry_run"), Required: []string{"file", "question", "forecast", "key_file", "confirm"}},
		service.OperationForecastKeyHintUpdate: {Allowed: append(file, "question", "forecast", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationForecastWithdraw:      {Allowed: append(file, "question", "forecast", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationForecastExpire:        {Allowed: append(file, "question", "forecast", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationForecastReaffirm:      {Allowed: append(file, "question", "forecast", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationTargetBuild:           {Allowed: append(file, "question", "forecast", "scope", "head", "checkpoint", "recorded_at", "all", "dry_run"), Required: file},
		service.OperationTargetCheck:           {Allowed: append(file, "question", "forecast", "scope", "head", "all"), Required: file},
		service.OperationTimestampStamp:        {Allowed: append(file, "question", "forecast", "scope", "head", "tsa_provider", "tsa_url", "ca_bundle", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationTimestampStatus:       {Allowed: append(file, "question", "forecast", "scope", "head"), Required: []string{"file", "question", "forecast"}},
		service.OperationTimestampVerify:       {Allowed: append(file, "question", "forecast", "scope", "head", "dry_run"), Required: []string{"file", "question", "forecast"}},
		service.OperationVerificationRun:       {Allowed: append(file, "question", "forecast", "check_sources"), Required: file},
		service.OperationPublicationBuild:      {Allowed: append(file, "output", "dry_run"), Required: []string{"file", "output"}},
		service.OperationPublicationVerify:     {Allowed: append(file, "manifest"), Required: []string{"file", "manifest"}},
	}
}

func (s *Server) registerTools() error {
	contracts := contracts()
	for _, definition := range service.SortedOperationDefinitions() {
		contract, ok := contracts[definition.Name]
		if !ok {
			return app.NewError(app.CodeInternal, "MCP operation has no closed tool contract", fmt.Errorf("%s", definition.Name))
		}
		contract, err := expandDirectContract(definition, contract)
		if err != nil {
			return err
		}
		if definition.Name == service.OperationForecastReveal && !s.config.Mode.AllowReveal {
			continue
		}
		if s.config.Mode.ReadOnly && definition.Policy.PersistentEffect {
			continue
		}
		if (definition.Name == service.OperationForecastSeal || definition.Name == service.OperationForecastReveal) && !s.roots.Has(service.RootSecret) {
			continue
		}
		if definition.Name == service.OperationPublicationBuild && !s.roots.Has(service.RootOutput) {
			continue
		}
		if definition.Name == service.OperationPublicationVerify && !s.roots.Has(service.RootOutput) {
			continue
		}
		allowed := make(map[string]bool, len(contract.Allowed))
		for _, name := range contract.Allowed {
			allowed[name] = true
		}
		schema, err := toolSchema(definition, contract)
		if err != nil {
			return err
		}
		description := fmt.Sprintf("%s. file paths use root-name:relative/path. effect=%s server_access=%s network=%s", definition.CLI, effectLabel(definition.Policy), accessLabel(s.config.Mode), definition.Policy.Network)
		if definition.ResultNotes != "" {
			description += ". " + definition.ResultNotes
		}
		s.sdk.AddTool(&sdk.Tool{Name: definition.MCPTool, Description: description, InputSchema: schema}, s.toolHandler(definition, allowed, contract.Required))
	}
	return nil
}

func toolSchema(definition service.OperationDefinition, contract toolContract) (map[string]any, error) {
	properties := map[string]any{}
	for _, name := range contract.Allowed {
		properties[name] = scalarProperty(name)
	}
	result := map[string]any{"type": "object", "properties": properties, "required": contract.Required, "additionalProperties": false}
	if definition.RequestSchema != "" && definition.RequestMode != service.RequestSecret {
		requestSchema, err := service.DirectRequestSchema(definition.RequestSchema)
		if err != nil {
			return nil, err
		}
		if nested, ok := requestSchema["properties"].(map[string]any); ok {
			for name, property := range nested {
				properties[name] = property
			}
		}
		if definitions, ok := requestSchema["$defs"].(map[string]any); ok {
			result["$defs"] = definitions
		}
		for _, keyword := range []string{"allOf", "anyOf", "oneOf", "dependentRequired"} {
			if value, ok := requestSchema[keyword]; ok {
				result[keyword] = value
			}
		}
	}
	return result, nil
}

func expandDirectContract(definition service.OperationDefinition, contract toolContract) (toolContract, error) {
	if definition.RequestSchema == "" || definition.RequestMode == service.RequestSecret {
		return contract, nil
	}
	schema, err := service.DirectRequestSchema(definition.RequestSchema)
	if err != nil {
		return contract, err
	}
	properties, _ := schema["properties"].(map[string]any)
	required := stringSlice(schema["required"])
	seen := map[string]bool{}
	for _, name := range contract.Allowed {
		seen[name] = true
	}
	for name := range properties {
		if seen[name] {
			return contract, app.NewError(app.CodeInternal, "direct MCP field collides with a selector or control: "+name, nil)
		}
		contract.Allowed = append(contract.Allowed, name)
		seen[name] = true
	}
	contract.Required = append(contract.Required, required...)
	sort.Strings(contract.Allowed)
	sort.Strings(contract.Required)
	return contract, nil
}

func stringSlice(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func scalarProperty(name string) map[string]any {
	switch name {
	case "dry_run", "confirm", "all", "check_sources":
		return map[string]any{"type": "boolean"}
	case "forecaster_kind":
		return map[string]any{"type": "string", "enum": []string{"individual", "team"}, "default": "individual"}
	case "scope":
		return map[string]any{"type": "string", "enum": []string{"forecast", "lifecycle"}, "default": "forecast"}
	default:
		return map[string]any{"type": "string", "minLength": 1}
	}
}

func accessLabel(mode service.AccessMode) string {
	if mode.ReadOnly {
		return "read-only"
	}
	return "read-write"
}

func effectLabel(policy service.OperationPolicy) string {
	if policy.PersistentEffect {
		return "mutating"
	}
	return "read-only"
}
