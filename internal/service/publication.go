package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/publication"
	ledgerschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/timestamp/rfc3161"
)

type PublicationFile struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	bytes  []byte
}

type PublicationBuildResult struct {
	LedgerID       ledger.Slug       `json:"ledger_id"`
	Output         string            `json:"output"`
	LedgerPath     string            `json:"ledger_path"`
	ManifestPath   string            `json:"manifest_path"`
	ManifestSHA256 string            `json:"manifest_sha256"`
	FileCount      int               `json:"file_count"`
	TotalBytes     int64             `json:"total_bytes"`
	Files          []PublicationFile `json:"files"`
	EvidenceState  string            `json:"evidence_state"`
	Effects        []SideEffect      `json:"effects,omitempty"`
	Recovery       Recovery          `json:"recovery,omitempty"`
	manifestBytes  []byte
}

type PublicationVerifyResult struct {
	LedgerID       ledger.Slug            `json:"ledger_id"`
	ManifestPath   string                 `json:"manifest_path"`
	ManifestSHA256 string                 `json:"manifest_sha256"`
	FileCount      int                    `json:"file_count"`
	TotalBytes     int64                  `json:"total_bytes"`
	Files          []PublicationFile      `json:"files"`
	Evidence       []ForecastVerification `json:"evidence"`
	Overall        VerificationOverall    `json:"overall"`
	Limitations    []string               `json:"limitations"`
	FailureCode    app.ErrorCode          `json:"-"`
}

func PlanPublicationBuild(ctx context.Context, ledgerPath, output string) (PublicationBuildResult, error) {
	resolvedOutput, err := storage.ResolveNewFilePath(output, "package output")
	if err != nil {
		return PublicationBuildResult{}, err
	}
	loaded, err := LoadAndValidateLedger(ctx, ledgerPath, nil)
	if err != nil {
		return PublicationBuildResult{}, err
	}
	result, err := collectPublication(loaded, resolvedOutput)
	if err != nil {
		return PublicationBuildResult{}, err
	}
	for _, file := range result.Files {
		result.Effects = append(result.Effects, SideEffect{Kind: EffectPackage, Action: EffectCreate, Status: EffectDeferred, Path: file.Path, Owned: true, Rollback: RollbackCreatedPublic})
	}
	result.Effects = append(result.Effects, SideEffect{Kind: EffectPackage, Action: EffectCreate, Status: EffectDeferred, Path: "manifest.json", Owned: true, Rollback: RollbackCreatedPublic})
	return result, nil
}

func CommitPublicationBuild(ctx context.Context, ledgerPath, output string, dryRun bool) (PublicationBuildResult, error) {
	result, err := PlanPublicationBuild(ctx, ledgerPath, output)
	if err != nil || dryRun {
		return result, err
	}
	resolvedOutput, err := storage.ResolveNewFilePath(output, "package output")
	if err != nil {
		return result, err
	}
	if err := os.Mkdir(resolvedOutput, 0o755); err != nil {
		return result, app.NewError(app.CodeIO, "package output directory cannot be created", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(resolvedOutput)
		}
	}()
	for _, file := range result.Files {
		if ctx != nil && ctx.Err() != nil {
			return result, app.NewError(app.CodeInterrupted, "package build was interrupted", ctx.Err())
		}
		destination := filepath.Join(resolvedOutput, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return result, app.NewError(app.CodeIO, "package directory cannot be created", err)
		}
		if err := storage.CreateExclusive(destination, file.bytes, 0o644); err != nil {
			return result, err
		}
	}
	if err := storage.CreateExclusive(filepath.Join(resolvedOutput, "manifest.json"), result.manifestBytes, 0o644); err != nil {
		return result, err
	}
	complete = true
	result.Effects = nil
	for _, file := range result.Files {
		result.Effects = append(result.Effects, SideEffect{Kind: EffectPackage, Action: EffectCreate, Status: EffectCompleted, Path: file.Path, Owned: true, Rollback: RollbackCreatedPublic})
	}
	result.Effects = append(result.Effects, SideEffect{Kind: EffectPackage, Action: EffectCreate, Status: EffectCompleted, Path: "manifest.json", Owned: true, Rollback: RollbackCreatedPublic})
	result.Recovery = Recovery{State: RecoveryNone}
	return result, nil
}

func collectPublication(loaded *LoadedLedger, output string) (PublicationBuildResult, error) {
	root := filepath.Dir(loaded.Path)
	ledgerRelative := filepath.ToSlash(filepath.Join("ledger", filepath.Base(loaded.Path)))
	ledgerBytes := append([]byte(nil), loaded.Document.Raw...)
	files := map[string]PublicationFile{ledgerRelative: publicationFile(publication.RoleLedger, ledgerRelative, ledgerBytes)}
	evidenceState := "complete"
	for _, question := range loaded.Model.Questions {
		for _, forecast := range question.Forecasts {
			if forecast.Commitment != nil {
				hint := ""
				if forecast.Commitment.Sealed != nil {
					hint = forecast.Commitment.Sealed.KeyHint
				} else if forecast.Commitment.Revealed != nil {
					hint = forecast.Commitment.Revealed.KeyHint
				}
				if err := ValidateKeyHint(forecast.ID, hint); err != nil {
					return PublicationBuildResult{}, app.WithDetails(app.NewError(app.CodeConflict, "forecast key hint is not package-safe; run forecast key-hint update", err), map[string]any{"forecast_id": forecast.ID})
				}
			}
			var target *ledger.ForecastTarget
			var timestamps []ledger.RFC3161Timestamp
			switch {
			case forecast.Integrity.Pending != nil:
				target, timestamps, evidenceState = &forecast.Integrity.Pending.Target, forecast.Integrity.Pending.Timestamps, "pending"
			case forecast.Integrity.Verified != nil:
				target, timestamps = &forecast.Integrity.Verified.Target, forecast.Integrity.Verified.Timestamps
			case forecast.Integrity.Failed != nil:
				target = forecast.Integrity.Failed.Target
				if forecast.Integrity.Failed.Timestamps != nil {
					timestamps = *forecast.Integrity.Failed.Timestamps
				}
			}
			lifecyclePending, lifecycleErr := collectLifecyclePublicationFiles(files, root, loaded.Model, question, forecast)
			if lifecycleErr != nil {
				return PublicationBuildResult{}, lifecycleErr
			}
			if lifecyclePending {
				evidenceState = "pending"
			}
			if target == nil {
				continue
			}
			artifact, err := BuildForecastTarget(loaded.Model, question.ID, forecast.ID)
			if err != nil || *target != TargetMetadataFor(artifact) {
				return PublicationBuildResult{}, app.NewError(app.CodeVerification, "recorded target metadata does not match the selected forecast", err)
			}
			actual, err := readConfinedArtifact(root, string(target.ArtifactPath), maxTargetBytes)
			if err != nil || !bytes.Equal(actual, artifact.Bytes) {
				return PublicationBuildResult{}, app.NewError(app.CodeVerification, "forecast target cannot be packaged because its bytes do not match", err)
			}
			if err := addPublicationFile(files, publicationFile(publication.RoleTarget, string(target.ArtifactPath), actual)); err != nil {
				return PublicationBuildResult{}, err
			}
			for _, timestamp := range timestamps {
				requestBytes, err := readConfinedArtifact(root, string(timestamp.RequestPath), maxTimestampRequestBytes)
				if err != nil {
					return PublicationBuildResult{}, err
				}
				if _, err := rfc3161.ParseRequest(requestBytes, artifact.Bytes, rfc3161.DefaultLimits()); err != nil {
					return PublicationBuildResult{}, app.NewError(app.CodeVerification, "RFC 3161 request cannot be packaged because its target binding is invalid", nil)
				}
				responseBytes, err := readConfinedArtifact(root, string(timestamp.ResponsePath), maxTimestampResponseBytes)
				if err != nil {
					return PublicationBuildResult{}, err
				}
				if err := rfc3161.ParseResponse(responseBytes, rfc3161.DefaultLimits()); err != nil {
					return PublicationBuildResult{}, app.NewError(app.CodeVerification, "RFC 3161 response cannot be packaged because it is malformed", nil)
				}
				if err := addPublicationFile(files, publicationFile(publication.RoleRequest, string(timestamp.RequestPath), requestBytes)); err != nil {
					return PublicationBuildResult{}, err
				}
				if err := addPublicationFile(files, publicationFile(publication.RoleResponse, string(timestamp.ResponsePath), responseBytes)); err != nil {
					return PublicationBuildResult{}, err
				}
				if timestamp.CABundlePath != nil {
					caBytes, err := readConfinedArtifact(root, string(*timestamp.CABundlePath), maxTimestampCABundleBytes)
					if err != nil {
						return PublicationBuildResult{}, err
					}
					if err := rfc3161.ValidateCABundle(caBytes, rfc3161.DefaultLimits()); err != nil {
						return PublicationBuildResult{}, app.NewError(app.CodeVerification, "RFC 3161 CA bundle cannot be packaged because it is invalid", nil)
					}
					if err := addPublicationFile(files, publicationFile(publication.RoleCABundle, string(*timestamp.CABundlePath), caBytes)); err != nil {
						return PublicationBuildResult{}, err
					}
				}
			}
			if forecast.Visibility == ledger.VisibilityRevealed {
				layer := verifyRevealLayer(question, forecast, VerificationLayer{Name: "content_binding", State: LayerPass})
				if layer.State != LayerPass {
					return PublicationBuildResult{}, app.NewError(app.CodeVerification, "revealed forecast authentication failed before package build", nil)
				}
			}
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	if err := storage.DetectPortablePathCollisions(paths); err != nil {
		return PublicationBuildResult{}, err
	}
	sort.Strings(paths)
	result := PublicationBuildResult{LedgerID: loaded.Model.LedgerID, Output: filepath.Base(output), LedgerPath: ledgerRelative, ManifestPath: "manifest.json", EvidenceState: evidenceState}
	manifest := publication.Manifest{Profile: publication.ManifestProfile, LedgerSchema: publication.SchemaPin{Version: ledgerschema.Version, Commit: ledgerschema.Commit, SHA256: ledgerschema.SchemaSHA256}, LedgerPath: ledgerRelative}
	for _, path := range paths {
		file := files[path]
		result.Files = append(result.Files, file)
		result.TotalBytes += file.Size
		manifest.Entries = append(manifest.Entries, publication.Entry{Role: file.Role, Path: file.Path, Size: file.Size, Digest: publication.Digest{Algorithm: "sha-256", Value: file.SHA256}})
	}
	publication.SortEntries(manifest.Entries)
	manifestBytes, err := publication.Encode(manifest)
	if err != nil {
		return PublicationBuildResult{}, app.NewError(app.CodeInternal, "publication manifest cannot be encoded", err)
	}
	result.manifestBytes = manifestBytes
	result.ManifestSHA256 = storage.ResourceDigest(manifestBytes)
	result.FileCount = len(result.Files) + 1
	result.TotalBytes += int64(len(manifestBytes))
	return result, nil
}

func collectLifecyclePublicationFiles(files map[string]PublicationFile, root string, model *ledger.Ledger, question ledger.Question, forecast ledger.Forecast) (bool, error) {
	if forecast.ActivityCheckpoints == nil {
		return false, nil
	}
	pending := false
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		var target *ledger.LifecycleTarget
		var timestamps []ledger.RFC3161Timestamp
		switch {
		case checkpoint.Integrity.Pending != nil:
			target, timestamps, pending = &checkpoint.Integrity.Pending.Target, checkpoint.Integrity.Pending.Timestamps, true
		case checkpoint.Integrity.Verified != nil:
			target, timestamps = &checkpoint.Integrity.Verified.Target, checkpoint.Integrity.Verified.Timestamps
		case checkpoint.Integrity.Failed != nil:
			target = checkpoint.Integrity.Failed.Target
			if checkpoint.Integrity.Failed.Timestamps != nil {
				timestamps = *checkpoint.Integrity.Failed.Timestamps
			}
		}
		if target == nil {
			continue
		}
		artifact, err := BuildLifecycleTarget(model, question.ID, forecast.ID, checkpoint.HeadEventID)
		if err != nil || target.Scope != artifact.Scope || target.Canonicalization != TargetCanonicalization || target.Digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
			return pending, app.NewError(app.CodeVerification, "recorded lifecycle target metadata does not match its checkpoint", err)
		}
		actual, err := readConfinedArtifact(root, string(target.ArtifactPath), maxTargetBytes)
		if err != nil || !bytes.Equal(actual, artifact.Bytes) {
			return pending, app.NewError(app.CodeVerification, "lifecycle target cannot be packaged because its bytes do not match", err)
		}
		if err := addPublicationFile(files, publicationFile(publication.RoleTarget, string(target.ArtifactPath), actual)); err != nil {
			return pending, err
		}
		for _, timestamp := range timestamps {
			requestBytes, err := readConfinedArtifact(root, string(timestamp.RequestPath), maxTimestampRequestBytes)
			if err != nil {
				return pending, err
			}
			if _, err := rfc3161.ParseRequest(requestBytes, artifact.Bytes, rfc3161.DefaultLimits()); err != nil {
				return pending, app.NewError(app.CodeVerification, "lifecycle RFC 3161 request has an invalid target binding", nil)
			}
			responseBytes, err := readConfinedArtifact(root, string(timestamp.ResponsePath), maxTimestampResponseBytes)
			if err != nil {
				return pending, err
			}
			if err := rfc3161.ParseResponse(responseBytes, rfc3161.DefaultLimits()); err != nil {
				return pending, app.NewError(app.CodeVerification, "lifecycle RFC 3161 response is malformed", nil)
			}
			if err := addPublicationFile(files, publicationFile(publication.RoleRequest, string(timestamp.RequestPath), requestBytes)); err != nil {
				return pending, err
			}
			if err := addPublicationFile(files, publicationFile(publication.RoleResponse, string(timestamp.ResponsePath), responseBytes)); err != nil {
				return pending, err
			}
			if timestamp.CABundlePath != nil {
				caBytes, err := readConfinedArtifact(root, string(*timestamp.CABundlePath), maxTimestampCABundleBytes)
				if err != nil {
					return pending, err
				}
				if err := rfc3161.ValidateCABundle(caBytes, rfc3161.DefaultLimits()); err != nil {
					return pending, app.NewError(app.CodeVerification, "lifecycle RFC 3161 CA bundle is invalid", nil)
				}
				if err := addPublicationFile(files, publicationFile(publication.RoleCABundle, string(*timestamp.CABundlePath), caBytes)); err != nil {
					return pending, err
				}
			}
		}
	}
	return pending, nil
}

func VerifyPublicationPackage(ctx context.Context, ledgerPath, manifestPath string) (PublicationVerifyResult, error) {
	resolvedLedger, err := storage.ResolveLedgerPath(ledgerPath, true)
	if err != nil {
		return PublicationVerifyResult{}, err
	}
	resolvedManifest, err := storage.ResolveLedgerPath(manifestPath, true)
	if err != nil {
		return PublicationVerifyResult{}, err
	}
	root := filepath.Dir(resolvedManifest)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return PublicationVerifyResult{}, err
	}
	manifestBytes, err := readBoundedFile(resolvedManifest, publication.MaxManifestBytes)
	if err != nil {
		return PublicationVerifyResult{}, err
	}
	manifest, err := publication.Decode(manifestBytes)
	if err != nil {
		return PublicationVerifyResult{}, app.NewError(app.CodeVerification, "publication manifest is invalid", err)
	}
	if manifest.LedgerSchema.Version != ledgerschema.Version || manifest.LedgerSchema.Commit != ledgerschema.Commit || manifest.LedgerSchema.SHA256 != ledgerschema.SchemaSHA256 {
		return PublicationVerifyResult{}, app.NewError(app.CodeVerification, "publication manifest schema pin does not match this binary", nil)
	}
	expectedLedger, err := resolver.ResolveLabeled(manifest.LedgerPath, true, "packaged ledger")
	if err != nil || expectedLedger != resolvedLedger {
		return PublicationVerifyResult{}, app.NewError(app.CodeVerification, "selected package ledger does not match manifest ledger_path", err)
	}
	result := PublicationVerifyResult{ManifestPath: "manifest.json", ManifestSHA256: storage.ResourceDigest(manifestBytes), FileCount: len(manifest.Entries) + 1, Evidence: []ForecastVerification{}, Limitations: append([]string(nil), verificationLimitations...)}
	listed := map[string]struct{}{"manifest.json": {}}
	for _, entry := range manifest.Entries {
		absolute, err := resolver.ResolveLabeled(entry.Path, true, "package entry")
		if err != nil {
			return result, app.NewError(app.CodeVerification, "a listed package entry is missing or unsafe", err)
		}
		data, err := readBoundedFile(absolute, 64<<20)
		if err != nil {
			return result, app.NewError(app.CodeVerification, "a listed package entry cannot be read", err)
		}
		file := publicationFile(entry.Role, entry.Path, data)
		if file.Size != entry.Size || file.SHA256 != entry.Digest.Value {
			return result, app.NewError(app.CodeVerification, "a listed package entry has different bytes", nil)
		}
		listed[entry.Path] = struct{}{}
		result.Files = append(result.Files, file)
		result.TotalBytes += file.Size
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return app.NewError(app.CodeVerification, "package contains an unsafe file type", nil)
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if _, ok := listed[relative]; !ok {
			return app.WithDetails(app.NewError(app.CodeVerification, "package contains an unexpected file", nil), map[string]any{"path": relative})
		}
		return nil
	}); err != nil {
		return result, err
	}
	loaded, err := LoadAndValidateLedgerWithArtifactRoot(ctx, resolvedLedger, root)
	if err != nil {
		return result, err
	}
	result.LedgerID = loaded.Model.LedgerID
	for _, question := range loaded.Model.Questions {
		for _, forecast := range question.Forecasts {
			content := verifyPackageContent(root, loaded.Model, question, forecast)
			timing := verifyPackageTiming(ctx, root, loaded.Model, question, forecast, content)
			reveal := verifyRevealLayer(question, forecast, content)
			outcome := verifyOutcomeLayer(ctx, question, VerificationOptions{Offline: true})
			activity := verifyPackageActivity(ctx, root, loaded.Model, question, forecast)
			result.Evidence = append(result.Evidence, ForecastVerification{QuestionID: question.ID, ForecastID: forecast.ID, Layers: []VerificationLayer{content, activity, timing, reveal, outcome}})
		}
	}
	if ctx != nil && ctx.Err() != nil {
		return result, app.NewError(app.CodeInterrupted, "package verification was interrupted", ctx.Err())
	}
	temporaryReport := VerificationReport{Forecasts: result.Evidence}
	result.Overall, result.FailureCode = aggregateVerification(temporaryReport)
	return result, nil
}

func verifyPackageActivity(ctx context.Context, root string, model *ledger.Ledger, question ledger.Question, forecast ledger.Forecast) VerificationLayer {
	activity := deriveActivity(forecast)
	evidence := activityEvidence(activity)
	layer := VerificationLayer{Name: "activity", Evidence: evidence, Limitations: []string{"Activity evidence cannot prove completeness after every independent observation is removed."}}
	if forecast.ActivityCheckpoints == nil || len(*forecast.ActivityCheckpoints) == 0 {
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"activity.unbound"}
		return layer
	}
	hasVerified, hasPending := false, false
	checkpointResults := make([]map[string]any, 0, len(*forecast.ActivityCheckpoints))
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		artifact, err := BuildLifecycleTarget(model, question.ID, forecast.ID, checkpoint.HeadEventID)
		if err != nil {
			return failedLayerWithEvidence(layer.Name, "activity.target_build_failed", evidence)
		}
		declaredTarget := lifecycleIntegrityTarget(checkpoint.Integrity)
		if declaredTarget == nil || declaredTarget.Scope != artifact.Scope || declaredTarget.Canonicalization != TargetCanonicalization || declaredTarget.Digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
			return failedLayerWithEvidence(layer.Name, "activity.target_metadata_mismatch", evidence)
		}
		row := map[string]any{"checkpoint_id": checkpoint.ID, "head_event_id": checkpoint.HeadEventID, "target_path": declaredTarget.ArtifactPath, "target_sha256": artifact.SHA256}
		data, err := readConfinedArtifact(root, string(declaredTarget.ArtifactPath), maxTargetBytes)
		if err != nil {
			row["state"] = ActivityNotChecked
			checkpointResults = append(checkpointResults, row)
			evidence["checkpoints"] = checkpointResults
			layer.State, layer.ReasonCodes = LayerNotChecked, []string{"activity.declared_evidence_missing"}
			return layer
		}
		if !bytes.Equal(data, artifact.Bytes) {
			return failedLayerWithEvidence(layer.Name, "activity.target_mismatch", evidence)
		}
		var timestamps []ledger.RFC3161Timestamp
		if checkpoint.Integrity.Pending != nil {
			timestamps = checkpoint.Integrity.Pending.Timestamps
			row["state"] = ledger.IntegrityPending
		} else if checkpoint.Integrity.Verified != nil {
			timestamps = checkpoint.Integrity.Verified.Timestamps
			row["state"] = ledger.IntegrityVerified
		} else {
			evidence["checkpoints"] = checkpointResults
			return failedLayerWithEvidence(layer.Name, "activity.imported_failed", evidence)
		}
		results := inspectTimestampEntries(ctx, root, artifact.Bytes, timestamps)
		row["timestamps"] = results
		checkpointResults = append(checkpointResults, row)
		for _, result := range results {
			if result.CheckState == LayerFail {
				evidence["checkpoints"] = checkpointResults
				return failedLayerWithEvidence(layer.Name, "activity.timestamp_mismatch", evidence)
			}
			hasVerified = hasVerified || result.CheckState == LayerPass && result.State == ledger.RFC3161Verified
			hasPending = hasPending || result.CheckState != LayerPass || result.State == ledger.RFC3161Pending
		}
	}
	evidence["checkpoints"] = checkpointResults
	layer.State, layer.ReasonCodes = classifyActivityEvidence(activity, hasVerified, hasPending, false)
	return layer
}

func verifyPackageContent(root string, model *ledger.Ledger, question ledger.Question, forecast ledger.Forecast) VerificationLayer {
	target := recordedForecastTarget(model, question.ID, forecast.ID)
	if target == nil {
		return VerificationLayer{Name: "content_binding", State: LayerNotApplicable, ReasonCodes: []string{"content.no_retained_target"}}
	}
	artifact, err := BuildForecastTarget(model, question.ID, forecast.ID)
	if err != nil || *target != TargetMetadataFor(artifact) {
		return failedLayer("content_binding", "content.target_metadata_mismatch", err)
	}
	data, err := readConfinedArtifact(root, string(target.ArtifactPath), maxTargetBytes)
	if err != nil || !bytes.Equal(data, artifact.Bytes) {
		return failedLayer("content_binding", "content.target_mismatch", err)
	}
	return VerificationLayer{Name: "content_binding", State: LayerPass, ReasonCodes: []string{"content.target_matches"}, Evidence: map[string]any{"path": target.ArtifactPath, "sha256": artifact.SHA256}}
}

func verifyPackageTiming(ctx context.Context, root string, model *ledger.Ledger, question ledger.Question, forecast ledger.Forecast, content VerificationLayer) VerificationLayer {
	if forecast.Integrity.Unanchored != nil {
		return VerificationLayer{Name: "existence_timing", State: LayerNotApplicable, ReasonCodes: []string{"timing.unanchored"}}
	}
	if content.State != LayerPass {
		return VerificationLayer{Name: "existence_timing", State: LayerNotChecked, ReasonCodes: []string{"timing.blocked_by_content"}}
	}
	var timestamps []ledger.RFC3161Timestamp
	if forecast.Integrity.Pending != nil {
		timestamps = forecast.Integrity.Pending.Timestamps
	} else if forecast.Integrity.Verified != nil {
		timestamps = forecast.Integrity.Verified.Timestamps
	} else {
		return failedLayer("existence_timing", "timing.imported_failed", nil)
	}
	artifact, _ := BuildForecastTarget(model, question.ID, forecast.ID)
	results := inspectTimestampEntries(ctx, root, artifact.Bytes, timestamps)
	hasPending, hasLate := false, false
	for _, item := range results {
		if item.CheckState == LayerPass {
			if question.Resolution != nil && question.Resolution.Resolved != nil && item.GenTime != nil {
				known, _ := ParseTimestamp(question.Resolution.Resolved.OutcomeKnownAt, "outcome_known_at")
				generated, parseErr := ParseTimestamp(*item.GenTime, "gen_time")
				if parseErr != nil || !generated.Before(known) {
					hasLate = true
					continue
				}
			}
			return VerificationLayer{Name: "existence_timing", State: LayerPass, ReasonCodes: []string{"timing.rfc3161_verified"}, Evidence: map[string]any{"timestamps": results}, Limitations: timestampLimitations()}
		}
		if item.CheckState == LayerPending || item.CheckState == LayerNotChecked {
			hasPending = true
		}
	}
	if hasPending {
		return VerificationLayer{Name: "existence_timing", State: LayerPending, ReasonCodes: []string{"timing.local_evidence_incomplete"}, Evidence: map[string]any{"timestamps": results}}
	}
	if hasLate {
		return failedLayerWithEvidence("existence_timing", "timing.not_before_outcome", map[string]any{"timestamps": results})
	}
	return VerificationLayer{Name: "existence_timing", State: LayerFail, ReasonCodes: []string{"timing.all_responses_failed"}, Evidence: map[string]any{"timestamps": results}}
}

func addPublicationFile(files map[string]PublicationFile, file PublicationFile) error {
	if existing, ok := files[file.Path]; ok {
		if existing.Role != file.Role || existing.SHA256 != file.SHA256 || existing.Size != file.Size {
			return app.NewError(app.CodeConflict, "publication artifacts collide at one package path", nil)
		}
		return nil
	}
	files[file.Path] = file
	return nil
}

func publicationFile(role, path string, data []byte) PublicationFile {
	digest := sha256.Sum256(data)
	return PublicationFile{Role: role, Path: path, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), bytes: append([]byte(nil), data...)}
}

func readConfinedArtifact(root, relative string, limit int64) ([]byte, error) {
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return nil, err
	}
	absolute, err := resolver.ResolveLabeled(relative, true, "package entry")
	if err != nil {
		return nil, err
	}
	return readBoundedFile(absolute, limit)
}
