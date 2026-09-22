package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

func PlanForecastSealFile(ctx context.Context, path, keyPath string, questionID, forecastID ledger.Slug, input SealedForecastInput, observedAt ledger.Timestamp) (ForecastFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return ForecastFileResult{}, err
	}
	resolvedKey, err := storage.ResolveNewFilePath(keyPath, "key file")
	if err != nil {
		return ForecastFileResult{}, err
	}
	if resolvedKey == loaded.Path {
		return ForecastFileResult{}, app.NewError(app.CodeConflict, "ledger and key destinations must be different", nil)
	}
	mutation, err := PlanSealedForecastAppend(loaded.Model, questionID, forecastID, input, observedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return ForecastFileResult{}, err
	}
	result, err := buildForecastMutationFileResult(loaded.Document, loaded.Model, questionID, forecastID, mutation)
	if err != nil {
		return ForecastFileResult{}, err
	}
	result.Effects = []SideEffect{
		{Kind: EffectKey, Action: EffectCreate, Status: EffectDeferred, Path: filepath.Base(resolvedKey), Owned: true, Rollback: RollbackRetainSecret},
		{Kind: EffectLedger, Action: EffectReplace, Status: EffectDeferred, Path: filepath.Base(loaded.Path), Owned: false, Rollback: RollbackNone},
	}
	return result, nil
}

func CommitForecastSealFile(ctx context.Context, path, keyPath string, questionID, forecastID ledger.Slug, input SealedForecastInput, observedAt ledger.Timestamp, effects Effects) (ForecastFileResult, error) {
	planned, err := PlanForecastSealFile(ctx, path, keyPath, questionID, forecastID, input, observedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return ForecastFileResult{}, err
	}
	resolvedKey, err := storage.ResolveNewFilePath(keyPath, "key file")
	if err != nil {
		return ForecastFileResult{}, err
	}
	resourceJournal := filepath.Join(filepath.Dir(resolved), "."+filepath.Base(resolved)+".forecast-seal-resources.json")
	plan, err := storage.NewResourcePlan(resourceJournal, string(OperationForecastSeal), []storage.ResourceEntry{
		{Kind: storage.ResourceKey, Type: storage.ResourceFile, Path: resolvedKey, Owned: true, Rollback: storage.ResourceRollbackRetainSecret, State: storage.ResourcePlanned},
		{Kind: storage.ResourceLedger, Type: storage.ResourceFile, Path: resolved, Owned: false, Rollback: storage.ResourceRollbackNone, State: storage.ResourcePlanned},
	})
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := plan.Begin(); err != nil {
		return ForecastFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	result, keyCreated := planned, false
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for forecast seal", decodeErr)
			}
			build, buildErr := BuildSealedForecastAppend(ctx, model, questionID, forecastID, input, observedAt, effects)
			if buildErr != nil {
				return nil, buildErr
			}
			patched, patchErr := document.ApplyPatch(parsed, build.Mutation.Patches)
			if patchErr != nil {
				return nil, app.NewError(app.CodeInternal, "sealed forecast mutation cannot be applied", patchErr)
			}
			if err := storage.CreateProtectedFile(resolvedKey, build.KeyFile); err != nil {
				return nil, err
			}
			keyCreated = true
			if err := plan.MarkCreated(resolvedKey, storage.ResourceDigest(build.KeyFile)); err != nil {
				return nil, err
			}
			result, buildErr = buildForecastMutationFileResult(parsed, model, questionID, forecastID, build.Mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return patched, nil
		},
	})
	if err != nil {
		_ = plan.Finish()
		if keyCreated {
			result.Recovery = retainedKeyRecovery(resolvedKey)
		}
		return result, err
	}
	if err := plan.MarkReplaced(resolved, result.AfterSHA256); err != nil {
		return sealedForecastCleanupFailure(result, resourceJournal, resolvedKey, resolved, err)
	}
	if err := plan.MarkCommitted(resolvedKey); err != nil {
		return sealedForecastCleanupFailure(result, resourceJournal, resolvedKey, resolved, err)
	}
	if err := plan.MarkCommitted(resolved); err != nil {
		return sealedForecastCleanupFailure(result, resourceJournal, resolvedKey, resolved, err)
	}
	if err := plan.Finish(); err != nil {
		return sealedForecastCleanupFailure(result, resourceJournal, resolvedKey, resolved, err)
	}
	result.Recovery = Recovery{State: RecoveryNone}
	result.Effects = []SideEffect{
		{Kind: EffectKey, Action: EffectCreate, Status: EffectCompleted, Path: filepath.Base(resolvedKey), Owned: true, Rollback: RollbackRetainSecret},
		{Kind: EffectLedger, Action: EffectReplace, Status: EffectCompleted, Path: filepath.Base(resolved), Owned: false, Rollback: RollbackNone},
	}
	return result, nil
}

func sealedForecastCleanupFailure(result ForecastFileResult, journal, key, ledgerPath string, err error) (ForecastFileResult, error) {
	result.Recovery = Recovery{State: RecoveryRequired, Message: "The protected key and ledger update succeeded, but resource journal cleanup needs attention.", Paths: []string{filepath.Base(key), filepath.Base(ledgerPath), filepath.Base(journal)}}
	return result, err
}

func PlanForecastRevealFile(ctx context.Context, path, keyPath string, questionID, forecastID ledger.Slug, revealedAt ledger.Timestamp) (ForecastFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return ForecastFileResult{}, err
	}
	keyBytes, err := storage.ReadProtectedFile(keyPath, 4096, "key file")
	if err != nil {
		return ForecastFileResult{}, err
	}
	defer clear(keyBytes)
	if err := ensureOriginalTargetContinuity(ctx, loaded, questionID, forecastID); err != nil {
		return ForecastFileResult{}, err
	}
	mutation, err := BuildForecastReveal(loaded.Model, questionID, forecastID, keyBytes, revealedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return ForecastFileResult{}, err
	}
	return buildForecastMutationFileResult(loaded.Document, loaded.Model, questionID, forecastID, mutation)
}

func CommitForecastRevealFile(ctx context.Context, path, keyPath string, questionID, forecastID ledger.Slug, revealedAt ledger.Timestamp) (ForecastFileResult, error) {
	planned, err := PlanForecastRevealFile(ctx, path, keyPath, questionID, forecastID, revealedAt)
	if err != nil {
		return ForecastFileResult{}, err
	}
	keyBytes, err := storage.ReadProtectedFile(keyPath, 4096, "key file")
	if err != nil {
		return ForecastFileResult{}, err
	}
	defer clear(keyBytes)
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return ForecastFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	result := planned
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for forecast reveal", decodeErr)
			}
			loaded := &LoadedLedger{Path: resolved, Document: parsed, Model: model}
			if continuityErr := ensureOriginalTargetContinuity(ctx, loaded, questionID, forecastID); continuityErr != nil {
				return nil, continuityErr
			}
			mutation, buildErr := BuildForecastReveal(model, questionID, forecastID, keyBytes, revealedAt)
			if buildErr != nil {
				return nil, buildErr
			}
			result, buildErr = buildForecastMutationFileResult(parsed, model, questionID, forecastID, mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return document.ApplyPatch(parsed, mutation.Patches)
		},
	})
	if err != nil {
		return ForecastFileResult{}, err
	}
	return result, nil
}

func PlanForecastKeyHintUpdateFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, keyHint string) (ForecastFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return ForecastFileResult{}, err
	}
	mutation, err := BuildForecastKeyHintUpdate(loaded.Model, questionID, forecastID, keyHint)
	if err != nil {
		return ForecastFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return ForecastFileResult{}, err
	}
	return buildForecastMutationFileResult(loaded.Document, loaded.Model, questionID, forecastID, mutation)
}

func CommitForecastKeyHintUpdateFile(ctx context.Context, path string, questionID, forecastID ledger.Slug, keyHint string) (ForecastFileResult, error) {
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return ForecastFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	var result ForecastFileResult
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for key hint update", decodeErr)
			}
			mutation, buildErr := BuildForecastKeyHintUpdate(model, questionID, forecastID, keyHint)
			if buildErr != nil {
				return nil, buildErr
			}
			result, buildErr = buildForecastMutationFileResult(parsed, model, questionID, forecastID, mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return document.ApplyPatch(parsed, mutation.Patches)
		},
	})
	if err != nil {
		return ForecastFileResult{}, err
	}
	return result, nil
}

func ensureOriginalTargetContinuity(ctx context.Context, loaded *LoadedLedger, questionID, forecastID ledger.Slug) error {
	if loaded == nil || loaded.Model == nil || loaded.Path == "" {
		return app.NewError(app.CodeInternal, "loaded ledger path is required for reveal target continuity", nil)
	}
	target := recordedForecastTarget(loaded.Model, questionID, forecastID)
	path := filepath.Join(filepath.Dir(loaded.Path), "proofs", "targets", string(forecastID)+".json")
	_, statErr := os.Lstat(path)
	if target == nil && errors.Is(statErr, fs.ErrNotExist) {
		return nil
	}
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return app.NewError(app.CodeIO, "original sealed target cannot be inspected", statErr)
	}
	_, err := CheckTargets(ctx, loaded.Path, false, questionID, forecastID)
	if err != nil {
		return app.NewError(app.CodeVerification, "original sealed target continuity check failed", err)
	}
	return nil
}
