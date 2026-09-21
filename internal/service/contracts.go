package service

import (
	"context"
	"fmt"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
)

// OperationName is the stable public identifier of an application action.
// Adapters may use different command or tool names, but they must call the same
// operation and return its name in structured results.
type OperationName string

const (
	OperationLedgerInit            OperationName = "ledger.init"
	OperationLedgerUpdate          OperationName = "ledger.update"
	OperationLedgerValidate        OperationName = "ledger.validate"
	OperationLedgerStatus          OperationName = "ledger.status"
	OperationPlatformAdd           OperationName = "platform.add"
	OperationPlatformUpdate        OperationName = "platform.update"
	OperationPlatformList          OperationName = "platform.list"
	OperationPlatformShow          OperationName = "platform.show"
	OperationPlatformRemove        OperationName = "platform.remove"
	OperationQuestionAdd           OperationName = "question.add"
	OperationQuestionUpdate        OperationName = "question.update"
	OperationQuestionRevise        OperationName = "question.revise"
	OperationQuestionList          OperationName = "question.list"
	OperationQuestionShow          OperationName = "question.show"
	OperationQuestionResolve       OperationName = "question.resolve"
	OperationQuestionAmbiguous     OperationName = "question.ambiguous"
	OperationQuestionVoid          OperationName = "question.void"
	OperationQuestionDispute       OperationName = "question.dispute"
	OperationQuestionNotApplicable OperationName = "question.not_applicable"
	OperationGroupAdd              OperationName = "group.add"
	OperationGroupUpdate           OperationName = "group.update"
	OperationGroupList             OperationName = "group.list"
	OperationGroupShow             OperationName = "group.show"
	OperationGroupRemove           OperationName = "group.remove"
	OperationRelationshipAdd       OperationName = "relationship.add"
	OperationRelationshipList      OperationName = "relationship.list"
	OperationRelationshipShow      OperationName = "relationship.show"
	OperationRelationshipRemove    OperationName = "relationship.remove"
	OperationForecastAdd           OperationName = "forecast.add"
	OperationForecastList          OperationName = "forecast.list"
	OperationForecastShow          OperationName = "forecast.show"
	OperationForecastSeal          OperationName = "forecast.seal"
	OperationForecastReveal        OperationName = "forecast.reveal"
	OperationForecastWithdraw      OperationName = "forecast.withdraw"
	OperationForecastExpire        OperationName = "forecast.expire"
	OperationForecastReaffirm      OperationName = "forecast.reaffirm"
	OperationForecastKeyHintUpdate OperationName = "forecast.key_hint.update"
	OperationTargetBuild           OperationName = "target.build"
	OperationTargetCheck           OperationName = "target.check"
	OperationTimestampStamp        OperationName = "timestamp.stamp"
	OperationTimestampStatus       OperationName = "timestamp.status"
	OperationTimestampVerify       OperationName = "timestamp.verify"
	OperationVerificationRun       OperationName = "verification.run"
	OperationPublicationBuild      OperationName = "publication.build"
	OperationPublicationVerify     OperationName = "publication.verify"
)

// RootClass identifies a confinement boundary without coupling application
// contracts to the storage implementation.
type RootClass string

const (
	RootLedger RootClass = "ledger"
	RootOutput RootClass = "output"
	RootSecret RootClass = "secret"
)

// Root identifies one caller-authorized filesystem root. Name is safe for
// public results; Path is private operation state and must not be serialized.
type Root struct {
	Name  string    `json:"name"`
	Class RootClass `json:"class"`
	Path  string    `json:"-"`
}

// Roots is the complete explicit root context for an operation. A CLI caller
// normally supplies paths directly and may leave this empty. MCP always fills
// it before resolving a tool path.
type Roots struct {
	Ledger []Root `json:"ledger,omitempty"`
	Output []Root `json:"output,omitempty"`
	Secret []Root `json:"secret,omitempty"`
}

// AccessMode describes server-wide effect restrictions. AllowReveal is kept
// separate because irreversible disclosure requires an explicit MCP grant.
type AccessMode struct {
	ReadOnly    bool `json:"read_only"`
	Offline     bool `json:"offline"`
	AllowReveal bool `json:"allow_reveal"`
}

// NetworkMode identifies the source policy used by one invocation.
type NetworkMode string

const (
	NetworkOffline NetworkMode = "offline"
	NetworkBuiltin NetworkMode = "builtin"
	NetworkCustom  NetworkMode = "custom"
)

// NetworkProfile is public metadata about a bounded network policy. Source IDs
// are safe stable labels, never URLs containing credentials.
type NetworkProfile struct {
	Mode               NetworkMode `json:"mode"`
	ID                 string      `json:"id,omitempty"`
	SourceIDs          []string    `json:"source_ids,omitempty"`
	MinimumSuccess     int         `json:"minimum_success,omitempty"`
	MaxUniqueHeights   int         `json:"max_unique_heights,omitempty"`
	MaxRequests        int         `json:"max_requests,omitempty"`
	MaxConcurrent      int         `json:"max_concurrent,omitempty"`
	TrustLimitations   []string    `json:"trust_limitations,omitempty"`
	PrivacyLimitations []string    `json:"privacy_limitations,omitempty"`
}

// RequestOptions carries behavior shared by CLI and MCP. Confirmed records
// explicit caller intent; adapters remain responsible for interactive prompts.
type RequestOptions struct {
	DryRun    bool           `json:"dry_run,omitempty"`
	Confirmed bool           `json:"confirmed,omitempty"`
	Timeout   time.Duration  `json:"-"`
	Mode      AccessMode     `json:"mode"`
	Roots     Roots          `json:"roots,omitempty"`
	Network   NetworkProfile `json:"network"`
}

// Warning is a stable, non-fatal observation. Details must contain only public
// data because the same value may be presented by CLI and MCP.
type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// TimeNormalization records a non-canonical CLI value and the exact timestamp
// passed to the shared service. MCP requests are already exact and omit it.
type TimeNormalization struct {
	Field      string           `json:"field"`
	Raw        string           `json:"raw"`
	Normalized ledger.Timestamp `json:"normalized"`
	Timezone   string           `json:"timezone"`
	DatePolicy string           `json:"date_policy"`
}

// EffectKind and EffectAction describe planned or completed external effects.
type EffectKind string
type EffectAction string

const (
	EffectLedger            EffectKind = "ledger"
	EffectTarget            EffectKind = "target"
	EffectTimestampRequest  EffectKind = "timestamp_request"
	EffectTimestampResponse EffectKind = "timestamp_response"
	EffectTimestampTrust    EffectKind = "timestamp_trust"
	EffectKey               EffectKind = "key"
	EffectPackage           EffectKind = "package"
	EffectNetwork           EffectKind = "network"
)

const (
	EffectRead    EffectAction = "read"
	EffectCreate  EffectAction = "create"
	EffectReplace EffectAction = "replace"
	EffectRemove  EffectAction = "remove"
	EffectContact EffectAction = "contact"
)

// EffectStatus distinguishes dry-run plans from effects actually completed.
type EffectStatus string

const (
	EffectPlanned   EffectStatus = "planned"
	EffectDeferred  EffectStatus = "deferred"
	EffectCompleted EffectStatus = "completed"
	EffectUnchanged EffectStatus = "unchanged"
)

// SideEffect contains only a safe relative path or source ID. Secret absolute
// paths are never part of a transport result.
type SideEffect struct {
	Kind     EffectKind    `json:"kind"`
	Action   EffectAction  `json:"action"`
	Status   EffectStatus  `json:"status"`
	Root     string        `json:"root,omitempty"`
	Path     string        `json:"path,omitempty"`
	SourceID string        `json:"source_id,omitempty"`
	Owned    bool          `json:"owned,omitempty"`
	Rollback RollbackClass `json:"rollback,omitempty"`
}

// RollbackClass records what recovery is allowed to do with an effect.
type RollbackClass string

const (
	RollbackNone          RollbackClass = "none"
	RollbackCreatedPublic RollbackClass = "created_public"
	RollbackRetainSecret  RollbackClass = "retain_secret"
)

// RecoveryState is the stable state of a multi-resource operation.
type RecoveryState string

const (
	RecoveryNone     RecoveryState = "none"
	RecoveryComplete RecoveryState = "complete"
	RecoveryPending  RecoveryState = "pending"
	RecoveryRetained RecoveryState = "retained"
	RecoveryRequired RecoveryState = "required"
)

// Recovery reports only caller-actionable public state. Paths follow the same
// safe-path rule as SideEffect.
type Recovery struct {
	State   RecoveryState `json:"state"`
	Message string        `json:"message,omitempty"`
	Paths   []string      `json:"paths,omitempty"`
	Actions []string      `json:"actions,omitempty"`
}

// Result is the common successful application envelope below transport-level
// JSON. T is deliberately typed per operation.
type Result[T any] struct {
	Operation OperationName `json:"operation"`
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Data      T             `json:"data"`
	Warnings  []Warning     `json:"warnings,omitempty"`
	Effects   []SideEffect  `json:"effects,omitempty"`
	Recovery  Recovery      `json:"recovery"`
}

// Operation is implemented once in the service layer and called by every
// adapter. Request and result types remain closed concrete structs.
type Operation[Request, Data any] interface {
	Name() OperationName
	Execute(context.Context, Request) (Result[Data], error)
}

// OperationFunc is a small adapter useful for composing and testing service
// operations without introducing a framework or transport dependency.
type OperationFunc[Request, Data any] struct {
	Operation OperationName
	Run       func(context.Context, Request) (Result[Data], error)
}

func (o OperationFunc[Request, Data]) Name() OperationName { return o.Operation }

func (o OperationFunc[Request, Data]) Execute(ctx context.Context, request Request) (Result[Data], error) {
	if o.Operation == "" || o.Run == nil {
		var zero Result[Data]
		return zero, fmt.Errorf("service operation is not configured")
	}
	return o.Run(ctx, request)
}
