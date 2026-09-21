package service

import (
	"context"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

func TestDryRunRecordsAndSkipsPersistentEffect(t *testing.T) {
	execution, err := PrepareExecution(context.Background(), OperationTargetBuild, RequestOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	defer execution.Close()
	called := false
	err = execution.PerformEffect(SideEffect{Kind: EffectTarget, Action: EffectCreate, Path: "proofs/targets/f.json"}, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run executed persistent effect")
	}
	effects := execution.Effects()
	if len(effects) != 1 || effects[0].Status != EffectDeferred {
		t.Fatalf("effects = %#v", effects)
	}
}

func TestExecutionPolicyRejectsInvalidModesAndApproval(t *testing.T) {
	tests := []struct {
		name      string
		operation OperationName
		options   RequestOptions
		code      app.ErrorCode
	}{
		{name: "dry-run read", operation: OperationVerificationRun, options: RequestOptions{DryRun: true}, code: app.CodeUsage},
		{name: "read-only write", operation: OperationPlatformAdd, options: RequestOptions{Mode: AccessMode{ReadOnly: true}}, code: app.CodeConflict},
		{name: "missing approval", operation: OperationForecastReveal, code: app.CodeConflict},
		{name: "offline network requirement", operation: OperationTimestampStamp, options: RequestOptions{Mode: AccessMode{Offline: true}}, code: app.CodeNetworkDisabled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution, err := PrepareExecution(context.Background(), test.operation, test.options)
			if execution != nil {
				execution.Close()
			}
			if app.ErrorCodeOf(err) != test.code {
				t.Fatalf("error = %#v, want %q", err, test.code)
			}
		})
	}
}

func TestExecutionTimeoutAndPolicyCoverage(t *testing.T) {
	for _, operation := range []OperationName{
		OperationLedgerInit, OperationLedgerUpdate, OperationLedgerValidate, OperationLedgerStatus,
		OperationPlatformAdd, OperationPlatformUpdate, OperationPlatformList, OperationPlatformShow, OperationPlatformRemove,
		OperationQuestionAdd, OperationQuestionUpdate, OperationQuestionRevise, OperationQuestionList, OperationQuestionShow, OperationQuestionResolve, OperationQuestionAmbiguous, OperationQuestionVoid, OperationQuestionDispute, OperationQuestionNotApplicable,
		OperationForecastAdd, OperationForecastList, OperationForecastShow, OperationForecastSeal, OperationForecastReveal, OperationForecastKeyHintUpdate,
		OperationTargetBuild, OperationTargetCheck, OperationTimestampStamp, OperationTimestampStatus, OperationTimestampVerify,
		OperationVerificationRun, OperationPublicationBuild, OperationPublicationVerify,
	} {
		if _, ok := PolicyForOperation(operation); !ok {
			t.Fatalf("operation %q has no policy", operation)
		}
	}
	execution, err := PrepareExecution(context.Background(), OperationLedgerStatus, RequestOptions{Timeout: time.Nanosecond})
	if err != nil && app.ErrorCodeOf(err) != app.CodeInterrupted {
		t.Fatal(err)
	}
	if execution == nil {
		return
	}
	defer execution.Close()
	select {
	case <-execution.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("operation timeout did not cancel context")
	}
}
