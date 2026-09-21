package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

type NetworkRequirement string

const (
	NetworkNone     NetworkRequirement = "none"
	NetworkOptional NetworkRequirement = "optional"
	NetworkRequired NetworkRequirement = "required"
)

type OperationPolicy struct {
	PersistentEffect     bool
	RequiresConfirmation bool
	Network              NetworkRequirement
}

func PolicyForOperation(name OperationName) (OperationPolicy, bool) {
	switch name {
	case OperationLedgerValidate, OperationLedgerStatus,
		OperationPlatformList, OperationPlatformShow,
		OperationQuestionList, OperationQuestionShow,
		OperationGroupList, OperationGroupShow,
		OperationRelationshipList, OperationRelationshipShow,
		OperationForecastList, OperationForecastShow,
		OperationTargetCheck:
		return OperationPolicy{Network: NetworkNone}, true
	case OperationVerificationRun:
		return OperationPolicy{Network: NetworkOptional}, true
	case OperationPublicationVerify, OperationTimestampStatus:
		return OperationPolicy{Network: NetworkNone}, true
	case OperationLedgerInit, OperationLedgerUpdate,
		OperationPlatformAdd, OperationPlatformUpdate,
		OperationQuestionAdd, OperationQuestionUpdate, OperationQuestionRevise,
		OperationGroupAdd, OperationGroupUpdate,
		OperationRelationshipAdd,
		OperationForecastAdd, OperationForecastSeal,
		OperationForecastWithdraw, OperationForecastExpire, OperationForecastReaffirm,
		OperationForecastKeyHintUpdate, OperationTargetBuild,
		OperationPublicationBuild:
		return OperationPolicy{PersistentEffect: true, Network: NetworkNone}, true
	case OperationPlatformRemove, OperationGroupRemove, OperationRelationshipRemove,
		OperationQuestionResolve, OperationQuestionAmbiguous, OperationQuestionVoid,
		OperationQuestionDispute, OperationQuestionNotApplicable,
		OperationForecastReveal:
		return OperationPolicy{PersistentEffect: true, RequiresConfirmation: true, Network: NetworkNone}, true
	case OperationTimestampStamp:
		return OperationPolicy{PersistentEffect: true, Network: NetworkRequired}, true
	case OperationTimestampVerify:
		return OperationPolicy{PersistentEffect: true, Network: NetworkNone}, true
	default:
		return OperationPolicy{}, false
	}
}

type Execution struct {
	Operation OperationName
	Policy    OperationPolicy
	Options   RequestOptions
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	effects   []SideEffect
}

func PrepareExecution(parent context.Context, name OperationName, options RequestOptions) (*Execution, error) {
	policy, ok := PolicyForOperation(name)
	if !ok {
		return nil, app.NewError(app.CodeInternal, "operation policy is not configured", fmt.Errorf("operation %q", name))
	}
	if options.DryRun && !policy.PersistentEffect {
		return nil, app.NewError(app.CodeUsage, "--dry-run is available only for persistent changes or resource creation", nil)
	}
	if options.Mode.ReadOnly && policy.PersistentEffect {
		return nil, app.NewError(app.CodeConflict, "operation is disabled in read-only mode", nil)
	}
	if policy.RequiresConfirmation && !options.Confirmed {
		return nil, app.NewError(app.CodeConflict, "explicit confirmation is required", nil)
	}
	if options.Mode.Offline && policy.Network == NetworkRequired {
		return nil, app.NewError(app.CodeNetworkDisabled, "operation requires network access but offline mode is enabled", nil)
	}
	if parent == nil {
		parent = context.Background()
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if options.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, options.Timeout)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	execution := &Execution{Operation: name, Policy: policy, Options: options, ctx: ctx, cancel: cancel}
	if err := execution.Checkpoint(); err != nil {
		cancel()
		return nil, err
	}
	return execution, nil
}

func (e *Execution) Context() context.Context {
	if e == nil || e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

func (e *Execution) Close() {
	if e != nil && e.cancel != nil {
		e.cancel()
	}
}

func (e *Execution) Checkpoint() error {
	if e == nil || e.ctx == nil || e.ctx.Err() == nil {
		return nil
	}
	return app.NewError(app.CodeInterrupted, "operation was interrupted", e.ctx.Err())
}

// PerformEffect substitutes a side-effect recorder during dry-run. Callers do
// parsing, selection, permission, collision, and prospective validation before
// this boundary; entropy, network, and persistent writes belong inside run.
func (e *Execution) PerformEffect(effect SideEffect, run func(context.Context) error) error {
	if err := e.Checkpoint(); err != nil {
		return err
	}
	if e.Options.DryRun {
		effect.Status = EffectDeferred
		e.record(effect)
		return nil
	}
	if run == nil {
		return app.NewError(app.CodeInternal, "side effect has no implementation", nil)
	}
	if err := run(e.Context()); err != nil {
		return err
	}
	effect.Status = EffectCompleted
	e.record(effect)
	return e.Checkpoint()
}

func (e *Execution) PlanEffect(effect SideEffect) {
	if effect.Status == "" {
		effect.Status = EffectPlanned
	}
	e.record(effect)
}

func (e *Execution) Effects() []SideEffect {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]SideEffect(nil), e.effects...)
}

func (e *Execution) record(effect SideEffect) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.effects = append(e.effects, effect)
}

func WithOperationTimeout(options RequestOptions, timeout time.Duration) RequestOptions {
	options.Timeout = timeout
	return options
}
