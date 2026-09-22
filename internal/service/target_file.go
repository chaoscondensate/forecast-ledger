package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

const maxTargetBytes = 16 << 20

type TargetResult struct {
	QuestionID   ledger.Slug                `json:"question_id"`
	ForecastID   ledger.Slug                `json:"forecast_id"`
	HeadEventID  ledger.Slug                `json:"head_event_id,omitempty"`
	Scope        string                     `json:"scope"`
	Path         ledger.RelativePath        `json:"path"`
	SHA256       string                     `json:"sha256"`
	ActualSHA256 string                     `json:"actual_sha256,omitempty"`
	Size         int                        `json:"size"`
	State        storage.DeterministicState `json:"state,omitempty"`
	Valid        *bool                      `json:"valid,omitempty"`
	ReasonCodes  []string                   `json:"reason_codes,omitempty"`
	Guidance     string                     `json:"guidance,omitempty"`
	ErrorCode    app.ErrorCode              `json:"error_code,omitempty"`
	Message      string                     `json:"message,omitempty"`
}

type TargetScope string

const (
	TargetScopeForecast  TargetScope = "forecast"
	TargetScopeLifecycle TargetScope = "lifecycle"
)

func validateTargetSelection(scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) error {
	if scope == "" {
		scope = TargetScopeForecast
	}
	if scope != TargetScopeForecast && scope != TargetScopeLifecycle {
		return app.NewError(app.CodeUsage, "target scope must be forecast or lifecycle", nil)
	}
	if scope == TargetScopeLifecycle && (all || questionID == "" || forecastID == "" || headEventID == "") {
		return app.NewError(app.CodeUsage, "lifecycle target scope requires question, forecast, and head and cannot be combined with all", nil)
	}
	if scope == TargetScopeForecast && headEventID != "" {
		return app.NewError(app.CodeUsage, "head can be used only with lifecycle target scope", nil)
	}
	return nil
}

type TargetOperationResult struct {
	LedgerID    ledger.Slug    `json:"ledger_id"`
	Targets     []TargetResult `json:"targets"`
	Effects     []SideEffect   `json:"effects,omitempty"`
	Recovery    Recovery       `json:"recovery,omitempty"`
	FailureCode app.ErrorCode  `json:"-"`
}

func PlanTargetBuild(ctx context.Context, path string, all bool, questionID, forecastID ledger.Slug) (TargetOperationResult, error) {
	return PlanTargetBuildScoped(ctx, path, TargetScopeForecast, all, questionID, forecastID, "")
}

func PlanTargetBuildScoped(ctx context.Context, path string, scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) (TargetOperationResult, error) {
	loaded, artifacts, err := loadSelectedTargets(ctx, path, scope, all, questionID, forecastID, headEventID)
	if err != nil {
		return TargetOperationResult{}, err
	}
	if len(artifacts) == 0 {
		return TargetOperationResult{LedgerID: loaded.Model.LedgerID, Targets: []TargetResult{}, Effects: []SideEffect{}, Recovery: Recovery{State: RecoveryNone}}, nil
	}
	root := filepath.Dir(loaded.Path)
	if _, _, err := inspectTargetDirectories(root); err != nil {
		return TargetOperationResult{}, err
	}
	result := TargetOperationResult{LedgerID: loaded.Model.LedgerID, Targets: make([]TargetResult, len(artifacts))}
	for index, artifact := range artifacts {
		state, err := preflightTargetFile(root, artifact)
		if err != nil {
			return TargetOperationResult{}, err
		}
		result.Targets[index] = targetResult(artifact, state, nil)
		status := EffectDeferred
		if state == storage.DeterministicUnchanged {
			status = EffectUnchanged
		}
		result.Effects = append(result.Effects, SideEffect{Kind: EffectTarget, Action: EffectCreate, Status: status, Path: string(artifact.RelativePath), Owned: state != storage.DeterministicUnchanged, Rollback: RollbackCreatedPublic})
	}
	return result, nil
}

func CommitTargetBuild(ctx context.Context, path string, all bool, questionID, forecastID ledger.Slug) (TargetOperationResult, error) {
	return CommitTargetBuildScoped(ctx, path, TargetScopeForecast, all, questionID, forecastID, "")
}

func CommitTargetBuildScoped(ctx context.Context, path string, scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) (TargetOperationResult, error) {
	if err := validateTargetSelection(scope, all, questionID, forecastID, headEventID); err != nil {
		return TargetOperationResult{}, err
	}
	resolvedLedger, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return TargetOperationResult{}, err
	}
	// Admission must happen before the advisory lock is created. In particular,
	// a superseded schema cannot cause lock or artifact side effects.
	if _, err := loadAndValidateLedgerForEvidence(ctx, resolvedLedger); err != nil {
		return TargetOperationResult{}, err
	}
	lock, err := storage.AcquireLedgerLock(ctx, resolvedLedger, 0)
	if err != nil {
		return TargetOperationResult{}, err
	}
	defer lock.Release()
	planned, err := PlanTargetBuildScoped(ctx, resolvedLedger, scope, all, questionID, forecastID, headEventID)
	if err != nil {
		return TargetOperationResult{}, err
	}
	if len(planned.Targets) == 0 {
		return planned, nil
	}
	loaded, artifacts, err := loadSelectedTargets(ctx, resolvedLedger, scope, all, questionID, forecastID, headEventID)
	if err != nil {
		return TargetOperationResult{}, err
	}
	root := filepath.Dir(loaded.Path)
	createProofs, createTargets, err := inspectTargetDirectories(root)
	if err != nil {
		return TargetOperationResult{}, err
	}
	proofsPath := filepath.Join(root, "proofs")
	targetsPath := filepath.Join(proofsPath, "targets")
	entries := make([]storage.ResourceEntry, 0, len(artifacts)+2)
	if createProofs {
		entries = append(entries, storage.ResourceEntry{Kind: storage.ResourceTarget, Type: storage.ResourceDirectory, Path: proofsPath, Owned: true, Rollback: storage.ResourceRollbackRemoveOwned, State: storage.ResourcePlanned})
	}
	if createTargets {
		entries = append(entries, storage.ResourceEntry{Kind: storage.ResourceTarget, Type: storage.ResourceDirectory, Path: targetsPath, Owned: true, Rollback: storage.ResourceRollbackRemoveOwned, State: storage.ResourcePlanned})
	}
	for index, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath)))
		owned := planned.Targets[index].State != storage.DeterministicUnchanged
		rollback := storage.ResourceRollbackNone
		if owned {
			rollback = storage.ResourceRollbackRemoveOwned
		}
		entries = append(entries, storage.ResourceEntry{Kind: storage.ResourceTarget, Type: storage.ResourceFile, Path: path, Owned: owned, Rollback: rollback, State: storage.ResourcePlanned})
	}
	journal := filepath.Join(root, "."+filepath.Base(loaded.Path)+".target-build-resources.json")
	plan, err := storage.NewResourcePlan(journal, string(OperationTargetBuild), entries)
	if err != nil {
		return TargetOperationResult{}, err
	}
	if err := plan.Begin(); err != nil {
		return TargetOperationResult{}, err
	}
	fail := func(cause error) (TargetOperationResult, error) {
		recovery, recoveryErr := storage.RecoverResourcePlan(context.Background(), journal)
		if recoveryErr != nil {
			planned.Recovery = Recovery{State: RecoveryRequired, Message: "Target creation did not finish and automatic cleanup needs attention.", Paths: []string{filepath.Base(journal)}}
			return planned, app.NewError(app.CodeIO, "target creation failed and recovery was incomplete", errors.Join(cause, recoveryErr))
		}
		planned.Recovery = Recovery{State: RecoveryNone}
		_ = recovery
		return planned, cause
	}
	if createProofs {
		if err := os.Mkdir(proofsPath, 0o755); err != nil {
			return fail(app.NewError(app.CodeIO, "proofs directory cannot be created", err))
		}
		if err := plan.MarkCreated(proofsPath, ""); err != nil {
			return fail(err)
		}
	}
	if createTargets {
		if err := os.Mkdir(targetsPath, 0o755); err != nil {
			return fail(app.NewError(app.CodeIO, "target directory cannot be created", err))
		}
		if err := plan.MarkCreated(targetsPath, ""); err != nil {
			return fail(err)
		}
	}
	result := TargetOperationResult{LedgerID: loaded.Model.LedgerID, Targets: make([]TargetResult, len(artifacts))}
	for index, artifact := range artifacts {
		if ctx != nil && ctx.Err() != nil {
			return fail(app.NewError(app.CodeInterrupted, "target build was interrupted", ctx.Err()))
		}
		absolute := filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath)))
		written, err := storage.EnsureDeterministicFile(absolute, artifact.Bytes, 0o644, maxTargetBytes)
		if err != nil {
			return fail(err)
		}
		if written.State == storage.DeterministicCreated {
			if err := plan.MarkCreated(absolute, artifact.SHA256); err != nil {
				return fail(err)
			}
		}
		result.Targets[index] = targetResult(artifact, written.State, nil)
		status := EffectCompleted
		if written.State == storage.DeterministicUnchanged {
			status = EffectUnchanged
		}
		result.Effects = append(result.Effects, SideEffect{Kind: EffectTarget, Action: EffectCreate, Status: status, Path: string(artifact.RelativePath), Owned: written.State == storage.DeterministicCreated, Rollback: RollbackCreatedPublic})
	}
	for _, entry := range entries {
		if err := plan.MarkCommitted(entry.Path); err != nil {
			result.Recovery = Recovery{State: RecoveryRequired, Message: "Targets were created, but target resource journal cleanup needs attention.", Paths: []string{filepath.Base(journal)}}
			return result, err
		}
	}
	if err := plan.Finish(); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "Targets were created, but target resource journal cleanup needs attention.", Paths: []string{filepath.Base(journal)}}
		return result, err
	}
	result.Recovery = Recovery{State: RecoveryNone}
	return result, nil
}

func CheckTargets(ctx context.Context, path string, all bool, questionID, forecastID ledger.Slug) (TargetOperationResult, error) {
	return CheckTargetsScoped(ctx, path, TargetScopeForecast, all, questionID, forecastID, "")
}

func CheckTargetsScoped(ctx context.Context, path string, scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) (TargetOperationResult, error) {
	result, err := InspectTargetsScoped(ctx, path, scope, all, questionID, forecastID, headEventID)
	if err != nil {
		return result, err
	}
	if result.FailureCode != "" {
		return result, targetInspectionError(result)
	}
	for _, target := range result.Targets {
		if target.State == storage.DeterministicState(LayerNotApplicable) {
			return result, app.WithDetails(app.NewError(app.CodeNotFound, "forecast target has not been retained", nil), map[string]any{"targets": result.Targets})
		}
	}
	return result, nil
}

// InspectTargets produces a complete, ordered report. Missing evidence that
// was never retained is a successful not_applicable observation; independently
// inspectable failures are collected so --all never stops at the first row.
func InspectTargets(ctx context.Context, path string, all bool, questionID, forecastID ledger.Slug) (TargetOperationResult, error) {
	return InspectTargetsScoped(ctx, path, TargetScopeForecast, all, questionID, forecastID, "")
}

func InspectTargetsScoped(ctx context.Context, path string, scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) (TargetOperationResult, error) {
	loaded, artifacts, err := loadSelectedTargets(ctx, path, scope, all, questionID, forecastID, headEventID)
	if err != nil {
		return TargetOperationResult{}, err
	}
	root := filepath.Dir(loaded.Path)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return TargetOperationResult{}, err
	}
	result := TargetOperationResult{LedgerID: loaded.Model.LedgerID, Targets: make([]TargetResult, len(artifacts))}
	for index, artifact := range artifacts {
		if ctx != nil && ctx.Err() != nil {
			return result, app.NewError(app.CodeInterrupted, "target inspection was interrupted", ctx.Err())
		}
		row := targetResult(artifact, storage.DeterministicState(LayerPass), nil)
		metadata := recordedTargetMetadata(loaded.Model, artifact)
		candidate := filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath)))
		info, statErr := os.Lstat(candidate)
		if errors.Is(statErr, fs.ErrNotExist) && metadata == nil {
			row.State = storage.DeterministicState(LayerNotApplicable)
			row.ReasonCodes = []string{"content.no_retained_target"}
			row.Guidance = "Run target build for this forecast."
			result.Targets[index] = row
			continue
		}
		if errors.Is(statErr, fs.ErrNotExist) {
			err = app.NewError(app.CodeVerification, "retained forecast target file is missing", statErr)
			result.Targets[index] = failedTargetResult(row, "content.target_missing", err)
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.CodeVerification)
			continue
		}
		if statErr != nil {
			err = app.NewError(app.CodeIO, "target path cannot be inspected", statErr)
			result.Targets[index] = failedTargetResult(row, "content.target_unreadable", err)
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.CodeIO)
			continue
		}
		absolute, err := resolver.ResolveLabeled(string(artifact.RelativePath), true, "target file")
		if err != nil {
			result.Targets[index] = failedTargetResult(row, "content.target_path_unsafe", err)
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.ErrorCodeOf(err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			err = app.NewError(app.CodeVerification, "retained target is not a regular file", nil)
			result.Targets[index] = failedTargetResult(row, "content.target_not_regular", err)
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.CodeVerification)
			continue
		}
		actual, err := readBoundedFile(absolute, maxTargetBytes)
		if err != nil {
			result.Targets[index] = failedTargetResult(row, "content.target_unreadable", err)
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.ErrorCodeOf(err))
			continue
		}
		if !bytes.Equal(actual, artifact.Bytes) {
			row.SHA256 = artifact.SHA256
			row.ActualSHA256 = storage.ResourceDigest(actual)
			row.State = storage.DeterministicState(LayerFail)
			row.Valid = boolPointer(false)
			row.ReasonCodes = []string{"content.target_mismatch"}
			row.ErrorCode = app.CodeVerification
			row.Message = "forecast target bytes do not match the ledger"
			row.Guidance = "Restore the retained target or review the ledger change; target build will not overwrite different bytes."
			result.Targets[index] = row
			result.FailureCode = strongerTargetFailure(result.FailureCode, app.CodeVerification)
			continue
		}
		if target := metadata; target != nil {
			if target.scope != artifact.Scope || target.canonicalization != TargetCanonicalization || target.path != artifact.RelativePath || target.digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
				err = app.NewError(app.CodeVerification, "recorded target metadata does not match the deterministic target", nil)
				result.Targets[index] = failedTargetResult(row, "content.target_metadata_mismatch", err)
				result.FailureCode = strongerTargetFailure(result.FailureCode, app.CodeVerification)
				continue
			}
		}
		valid := true
		row.Valid = &valid
		row.ReasonCodes = []string{"content.target_matches"}
		result.Targets[index] = row
	}
	return result, nil
}

func loadSelectedTargets(ctx context.Context, path string, scope TargetScope, all bool, questionID, forecastID, headEventID ledger.Slug) (*LoadedLedger, []TargetArtifact, error) {
	if err := validateTargetSelection(scope, all, questionID, forecastID, headEventID); err != nil {
		return nil, nil, err
	}
	if scope == "" {
		scope = TargetScopeForecast
	}
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	var artifacts []TargetArtifact
	if scope == TargetScopeLifecycle {
		var artifact TargetArtifact
		artifact, err = BuildLifecycleTarget(loaded.Model, questionID, forecastID, headEventID)
		if err == nil {
			if metadata := recordedTargetMetadata(loaded.Model, artifact); metadata != nil {
				if metadata.scope != artifact.Scope || metadata.canonicalization != TargetCanonicalization || metadata.digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
					return nil, nil, app.NewError(app.CodeVerification, "recorded lifecycle target metadata does not match its canonical prefix", nil)
				}
				artifact.RelativePath = metadata.path
			}
		}
		artifacts = []TargetArtifact{artifact}
	} else if all {
		artifacts, err = BuildAllForecastTargets(loaded.Model)
	} else {
		var artifact TargetArtifact
		artifact, err = BuildForecastTarget(loaded.Model, questionID, forecastID)
		artifacts = []TargetArtifact{artifact}
	}
	if err != nil {
		return nil, nil, err
	}
	return loaded, artifacts, nil
}

func inspectTargetDirectories(root string) (createProofs, createTargets bool, err error) {
	proofs := filepath.Join(root, "proofs")
	targets := filepath.Join(proofs, "targets")
	proofsInfo, err := os.Lstat(proofs)
	if errors.Is(err, fs.ErrNotExist) {
		if collision, collisionErr := caseFoldName(root, "proofs"); collisionErr != nil {
			return false, false, collisionErr
		} else if collision != "" {
			return false, false, app.NewError(app.CodeConflict, "proofs directory has a portable case collision", nil)
		}
		return true, true, nil
	}
	if err != nil {
		return false, false, app.NewError(app.CodeIO, "proofs directory cannot be inspected", err)
	}
	if proofsInfo.Mode()&os.ModeSymlink != 0 || !proofsInfo.IsDir() {
		return false, false, app.NewError(app.CodeConflict, "proofs path must be a real directory", nil)
	}
	targetInfo, err := os.Lstat(targets)
	if errors.Is(err, fs.ErrNotExist) {
		if collision, collisionErr := caseFoldName(proofs, "targets"); collisionErr != nil {
			return false, false, collisionErr
		} else if collision != "" {
			return false, false, app.NewError(app.CodeConflict, "target directory has a portable case collision", nil)
		}
		return false, true, nil
	}
	if err != nil {
		return false, false, app.NewError(app.CodeIO, "target directory cannot be inspected", err)
	}
	if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.IsDir() {
		return false, false, app.NewError(app.CodeConflict, "target path must be a real directory", nil)
	}
	return false, false, nil
}

func preflightTargetFile(root string, artifact TargetArtifact) (storage.DeterministicState, error) {
	absolute := filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath)))
	info, err := os.Lstat(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return storage.DeterministicCreated, nil
	}
	if err != nil {
		return "", app.NewError(app.CodeIO, "target path cannot be inspected", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", app.NewError(app.CodeConflict, "target destination is not a regular file", nil)
	}
	actual, err := readBoundedFile(absolute, maxTargetBytes)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(actual, artifact.Bytes) {
		return "", app.WithDetails(app.NewError(app.CodeConflict, "target destination contains different bytes", nil), map[string]any{"path": artifact.RelativePath, "expected_sha256": artifact.SHA256, "actual_sha256": storage.ResourceDigest(actual)})
	}
	return storage.DeterministicUnchanged, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, app.NewError(app.CodeNotFound, "target file does not exist", err)
		}
		return nil, app.NewError(app.CodeIO, "target file cannot be opened", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, app.NewError(app.CodeIO, "target file cannot be read", err)
	}
	if int64(len(data)) > limit {
		return nil, app.NewError(app.CodeVerification, "target file exceeds its size limit", nil)
	}
	return data, nil
}

func recordedForecastTarget(model *ledger.Ledger, questionID, forecastID ledger.Slug) *ledger.ForecastTarget {
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return nil
	}
	for _, forecast := range question.Forecasts {
		if forecast.ID != forecastID {
			continue
		}
		switch {
		case forecast.Integrity.Pending != nil:
			return &forecast.Integrity.Pending.Target
		case forecast.Integrity.Verified != nil:
			return &forecast.Integrity.Verified.Target
		case forecast.Integrity.Failed != nil:
			return forecast.Integrity.Failed.Target
		}
	}
	return nil
}

type recordedTarget struct {
	scope            string
	canonicalization string
	path             ledger.RelativePath
	digest           ledger.Digest
}

func recordedTargetMetadata(model *ledger.Ledger, artifact TargetArtifact) *recordedTarget {
	if artifact.Scope == ForecastEnvelopeSchema {
		target := recordedForecastTarget(model, artifact.QuestionID, artifact.ForecastID)
		if target == nil {
			return nil
		}
		return &recordedTarget{target.Scope, target.Canonicalization, target.ArtifactPath, target.Digest}
	}
	_, _, _, forecast, err := selectForecast(model, artifact.QuestionID, artifact.ForecastID)
	if err != nil || forecast.ActivityCheckpoints == nil {
		return nil
	}
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		if checkpoint.HeadEventID != artifact.HeadEventID {
			continue
		}
		var target *ledger.LifecycleTarget
		switch {
		case checkpoint.Integrity.Pending != nil:
			target = &checkpoint.Integrity.Pending.Target
		case checkpoint.Integrity.Verified != nil:
			target = &checkpoint.Integrity.Verified.Target
		case checkpoint.Integrity.Failed != nil:
			target = checkpoint.Integrity.Failed.Target
		}
		if target != nil {
			return &recordedTarget{target.Scope, target.Canonicalization, target.ArtifactPath, target.Digest}
		}
	}
	return nil
}

func targetResult(artifact TargetArtifact, state storage.DeterministicState, valid *bool) TargetResult {
	return TargetResult{QuestionID: artifact.QuestionID, ForecastID: artifact.ForecastID, HeadEventID: artifact.HeadEventID, Scope: artifact.Scope, Path: artifact.RelativePath, SHA256: artifact.SHA256, Size: artifact.Size, State: state, Valid: valid}
}

func failedTargetResult(row TargetResult, reason string, err error) TargetResult {
	row.State = storage.DeterministicState(LayerFail)
	row.Valid = boolPointer(false)
	row.ReasonCodes = []string{reason}
	row.ErrorCode = app.ErrorCodeOf(err)
	var applicationErr *app.Error
	if errors.As(err, &applicationErr) {
		row.Message = applicationErr.Message
	}
	return row
}

func boolPointer(value bool) *bool { return &value }

func strongerTargetFailure(current, candidate app.ErrorCode) app.ErrorCode {
	priority := func(code app.ErrorCode) int {
		switch code {
		case "":
			return 0
		case app.CodeInterrupted:
			return 5
		case app.CodeIO:
			return 4
		case app.CodeConflict:
			return 3
		case app.CodeVerification:
			return 2
		default:
			return 1
		}
	}
	if priority(candidate) > priority(current) {
		return candidate
	}
	return current
}

func targetInspectionError(result TargetOperationResult) error {
	return app.WithDetails(app.NewError(result.FailureCode, "one or more forecast targets could not be verified", nil), map[string]any{"ledger_id": result.LedgerID, "targets": result.Targets})
}

func caseFoldName(directory, requested string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", app.NewError(app.CodeIO, "artifact directory cannot be read", err)
	}
	for _, entry := range entries {
		if entry.Name() != requested && strings.EqualFold(entry.Name(), requested) {
			return entry.Name(), nil
		}
	}
	return "", nil
}
