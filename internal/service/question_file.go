package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

type QuestionFileResult struct {
	LedgerID        ledger.Slug           `json:"ledger_id"`
	QuestionID      ledger.Slug           `json:"question_id"`
	Status          ledger.QuestionStatus `json:"status"`
	PriorStatus     ledger.QuestionStatus `json:"prior_status,omitempty"`
	RecordedAt      *ledger.Timestamp     `json:"recorded_at,omitempty"`
	NormalizedTimes []TimeNormalization   `json:"normalized_times,omitempty"`
	Changed         bool                  `json:"changed"`
	ChangedPointers []string              `json:"changed_pointers"`
	BeforeSHA256    string                `json:"before_sha256"`
	AfterSHA256     string                `json:"after_sha256"`
	Warnings        []Warning             `json:"warnings,omitempty"`
	Effects         []SideEffect          `json:"effects,omitempty"`
	Recovery        Recovery              `json:"recovery,omitempty"`
}

type questionMutationBuilder func(*ledger.Ledger) (QuestionMutation, error)

func PlanQuestionAddEmptyFile(ctx context.Context, path string, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, input.ID, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionAddEmpty(model, input, observedAt)
	})
}

func CommitQuestionAddEmptyFile(ctx context.Context, path string, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, input.ID, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionAddEmpty(model, input, observedAt)
	})
}

func PlanQuestionAddPublicFile(ctx context.Context, path string, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, input.ID, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionAddPublic(model, input, observedAt)
	})
}

func CommitQuestionAddPublicFile(ctx context.Context, path string, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, input.ID, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionAddPublic(model, input, observedAt)
	})
}

func PlanQuestionAddSealedFile(ctx context.Context, path, keyPath string, input NormalizedQuestionCreate, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return QuestionFileResult{}, err
	}
	resolvedKey, err := storage.ResolveNewFilePath(keyPath, "key file")
	if err != nil {
		return QuestionFileResult{}, err
	}
	if resolvedKey == loaded.Path {
		return QuestionFileResult{}, app.NewError(app.CodeConflict, "ledger and key destinations must be different", nil)
	}
	mutation, err := PlanQuestionAddSealed(loaded.Model, input, observedAt)
	if err != nil {
		return QuestionFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return QuestionFileResult{}, err
	}
	result, err := buildQuestionFileResult(loaded.Document, loaded.Model, input.ID, mutation)
	if err != nil {
		return QuestionFileResult{}, err
	}
	result.Effects = []SideEffect{
		{Kind: EffectKey, Action: EffectCreate, Status: EffectDeferred, Path: filepath.Base(resolvedKey), Owned: true, Rollback: RollbackRetainSecret},
		{Kind: EffectLedger, Action: EffectReplace, Status: EffectDeferred, Path: filepath.Base(loaded.Path), Owned: false, Rollback: RollbackNone},
	}
	return result, nil
}

func CommitQuestionAddSealedFile(ctx context.Context, path, keyPath string, input NormalizedQuestionCreate, observedAt ledger.Timestamp, effects Effects) (QuestionFileResult, error) {
	planned, err := PlanQuestionAddSealedFile(ctx, path, keyPath, input, observedAt)
	if err != nil {
		return QuestionFileResult{}, err
	}
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return QuestionFileResult{}, err
	}
	resolvedKey, err := storage.ResolveNewFilePath(keyPath, "key file")
	if err != nil {
		return QuestionFileResult{}, err
	}
	resourceJournal := filepath.Join(filepath.Dir(resolved), "."+filepath.Base(resolved)+".question-add-resources.json")
	plan, err := storage.NewResourcePlan(resourceJournal, string(OperationQuestionAdd), []storage.ResourceEntry{
		{Kind: storage.ResourceKey, Type: storage.ResourceFile, Path: resolvedKey, Owned: true, Rollback: storage.ResourceRollbackRetainSecret, State: storage.ResourcePlanned},
		{Kind: storage.ResourceLedger, Type: storage.ResourceFile, Path: resolved, Owned: false, Rollback: storage.ResourceRollbackNone, State: storage.ResourcePlanned},
	})
	if err != nil {
		return QuestionFileResult{}, err
	}
	if err := plan.Begin(); err != nil {
		return QuestionFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	result := planned
	keyCreated := false
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for sealed question mutation", decodeErr)
			}
			build, buildErr := BuildQuestionAddSealed(ctx, model, input, observedAt, effects)
			if buildErr != nil {
				return nil, buildErr
			}
			patched, patchErr := document.ApplyPatch(parsed, build.Mutation.Patches)
			if patchErr != nil {
				return nil, app.NewError(app.CodeInternal, "sealed question mutation cannot be applied", patchErr)
			}
			if err := storage.CreateProtectedFile(resolvedKey, build.KeyFile); err != nil {
				return nil, err
			}
			keyCreated = true
			if err := plan.MarkCreated(resolvedKey, storage.ResourceDigest(build.KeyFile)); err != nil {
				return nil, err
			}
			result, buildErr = buildQuestionFileResult(parsed, model, input.ID, build.Mutation)
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
			result.Effects = []SideEffect{
				{Kind: EffectKey, Action: EffectCreate, Status: EffectCompleted, Path: filepath.Base(resolvedKey), Owned: true, Rollback: RollbackRetainSecret},
				{Kind: EffectLedger, Action: EffectReplace, Status: EffectDeferred, Path: filepath.Base(resolved), Owned: false, Rollback: RollbackNone},
			}
		}
		return result, err
	}
	if err := plan.MarkReplaced(resolved, result.AfterSHA256); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "The protected key and ledger update succeeded, but resource journal cleanup requires attention.", Paths: []string{filepath.Base(resolvedKey), filepath.Base(resolved), filepath.Base(resourceJournal)}}
		return result, err
	}
	if err := plan.MarkCommitted(resolvedKey); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "The protected key and ledger update succeeded, but resource journal cleanup requires attention.", Paths: []string{filepath.Base(resolvedKey), filepath.Base(resolved), filepath.Base(resourceJournal)}}
		return result, err
	}
	if err := plan.MarkCommitted(resolved); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "The protected key and ledger update succeeded, but resource journal cleanup requires attention.", Paths: []string{filepath.Base(resolvedKey), filepath.Base(resolved), filepath.Base(resourceJournal)}}
		return result, err
	}
	if err := plan.Finish(); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "The protected key and ledger update succeeded, but resource journal cleanup requires attention.", Paths: []string{filepath.Base(resolvedKey), filepath.Base(resolved), filepath.Base(resourceJournal)}}
		return result, err
	}
	result.Recovery = Recovery{State: RecoveryNone}
	result.Effects = []SideEffect{
		{Kind: EffectKey, Action: EffectCreate, Status: EffectCompleted, Path: filepath.Base(resolvedKey), Owned: true, Rollback: RollbackRetainSecret},
		{Kind: EffectLedger, Action: EffectReplace, Status: EffectCompleted, Path: filepath.Base(resolved), Owned: false, Rollback: RollbackNone},
	}
	return result, nil
}

func PlanQuestionUpdateFile(ctx context.Context, path string, id ledger.Slug, input QuestionPatchInput) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) { return BuildQuestionUpdate(model, id, input) })
}

func CommitQuestionUpdateFile(ctx context.Context, path string, id ledger.Slug, input QuestionPatchInput) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) { return BuildQuestionUpdate(model, id, input) })
}

func PlanQuestionReviseFile(ctx context.Context, path string, id ledger.Slug, input RevisionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionRevise(model, id, input, observedAt)
	})
}

func CommitQuestionReviseFile(ctx context.Context, path string, id ledger.Slug, input RevisionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionRevise(model, id, input, observedAt)
	})
}

func PlanQuestionResolveFile(ctx context.Context, path string, id ledger.Slug, input ResolutionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionResolve(model, id, input, observedAt)
	})
}

func CommitQuestionResolveFile(ctx context.Context, path string, id ledger.Slug, input ResolutionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionResolve(model, id, input, observedAt)
	})
}

func PlanQuestionUnresolvedFile(ctx context.Context, path string, id ledger.Slug, status ledger.ResolutionStatus, input UnresolvedResolutionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionUnresolved(model, id, status, input, observedAt)
	})
}

func CommitQuestionUnresolvedFile(ctx context.Context, path string, id ledger.Slug, status ledger.ResolutionStatus, input UnresolvedResolutionInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionUnresolved(model, id, status, input, observedAt)
	})
}

func PlanQuestionNotApplicableFile(ctx context.Context, path string, id ledger.Slug, input NotApplicableInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return planQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionNotApplicable(model, id, input, observedAt)
	})
}

func CommitQuestionNotApplicableFile(ctx context.Context, path string, id ledger.Slug, input NotApplicableInput, observedAt ledger.Timestamp) (QuestionFileResult, error) {
	return commitQuestionMutation(ctx, path, id, func(model *ledger.Ledger) (QuestionMutation, error) {
		return BuildQuestionNotApplicable(model, id, input, observedAt)
	})
}

func planQuestionMutation(ctx context.Context, path string, id ledger.Slug, builder questionMutationBuilder) (QuestionFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return QuestionFileResult{}, err
	}
	mutation, err := builder(loaded.Model)
	if err != nil {
		return QuestionFileResult{}, err
	}
	if err := rejectExistingQuestionTargets(loaded.Path, id, mutation); err != nil {
		return QuestionFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return QuestionFileResult{}, err
	}
	return buildQuestionFileResult(loaded.Document, loaded.Model, id, mutation)
}

func commitQuestionMutation(ctx context.Context, path string, id ledger.Slug, builder questionMutationBuilder) (QuestionFileResult, error) {
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return QuestionFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	var result QuestionFileResult
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{
		Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) },
		Mutate: func(parsed *document.Document) ([]byte, error) {
			model, decodeErr := validation.DecodeLedger(parsed)
			if decodeErr != nil {
				return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded for question mutation", decodeErr)
			}
			mutation, buildErr := builder(model)
			if buildErr != nil {
				return nil, buildErr
			}
			if buildErr := rejectExistingQuestionTargets(resolved, id, mutation); buildErr != nil {
				return nil, buildErr
			}
			result, buildErr = buildQuestionFileResult(parsed, model, id, mutation)
			if buildErr != nil {
				return nil, buildErr
			}
			return document.ApplyPatch(parsed, mutation.Patches)
		},
	})
	if err != nil {
		return QuestionFileResult{}, err
	}
	return result, nil
}

func buildQuestionFileResult(parsed *document.Document, model *ledger.Ledger, id ledger.Slug, mutation QuestionMutation) (QuestionFileResult, error) {
	patched, err := document.ApplyPatch(parsed, mutation.Patches)
	if err != nil {
		return QuestionFileResult{}, app.NewError(app.CodeInternal, "question mutation cannot be applied to source document", err)
	}
	_, question, err := selectQuestion(mutation.Ledger, id)
	if err != nil {
		return QuestionFileResult{}, app.NewError(app.CodeInternal, "mutated question cannot be selected", err)
	}
	before := sha256.Sum256(parsed.Raw)
	after := sha256.Sum256(patched)
	result := QuestionFileResult{
		LedgerID: model.LedgerID, QuestionID: id, Status: question.Status, PriorStatus: mutation.PriorStatus,
		Changed: len(mutation.Patches) > 0, ChangedPointers: ChangedPointers(mutation.Patches),
		BeforeSHA256: hex.EncodeToString(before[:]), AfterSHA256: hex.EncodeToString(after[:]),
	}
	if question.Resolution != nil {
		if question.Resolution.Resolved != nil {
			value := question.Resolution.Resolved.RecordedAt
			result.RecordedAt = &value
		} else if question.Resolution.Unresolved != nil {
			value := question.Resolution.Unresolved.RecordedAt
			result.RecordedAt = &value
		} else if question.Resolution.NotApplicable != nil {
			value := question.Resolution.NotApplicable.RecordedAt
			result.RecordedAt = &value
		}
	}
	if mutation.PriorStatus == ledger.QuestionDisputed || mutation.PriorStatus == ledger.QuestionResolved || mutation.PriorStatus == ledger.QuestionAmbiguous || mutation.PriorStatus == ledger.QuestionVoid || mutation.PriorStatus == ledger.QuestionNotApplicable {
		result.Warnings = append(result.Warnings, Warning{Code: "resolution_history_external", Message: "Forecast Ledger v2 stores only the current resolution object; use external file history if prior resolution records must be retained."})
	}
	return result, nil
}

func rejectExistingQuestionTargets(ledgerPath string, questionID ledger.Slug, mutation QuestionMutation) error {
	if !mutation.TargetCoveredChanged {
		return nil
	}
	root := filepath.Dir(ledgerPath)
	for _, id := range mutation.AffectedForecastIDs {
		path := filepath.Join(root, "proofs", "targets", string(id)+".json")
		_, err := os.Lstat(path)
		if err == nil {
			return frozenQuestionConflict(questionID)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return app.NewError(app.CodeIO, "forecast target path cannot be inspected", err)
		}
	}
	return nil
}

func LoadQuestionList(ctx context.Context, path string, stdin io.Reader) (ledger.Slug, []QuestionSummary, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", nil, err
	}
	items, err := ListQuestions(loaded.Model)
	return loaded.Model.LedgerID, items, err
}

func LoadQuestionShow(ctx context.Context, path string, stdin io.Reader, id ledger.Slug) (ledger.Slug, QuestionView, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", QuestionView{}, err
	}
	result, err := ShowQuestion(loaded.Model, id)
	return loaded.Model.LedgerID, result, err
}
