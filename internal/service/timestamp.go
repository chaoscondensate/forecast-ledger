package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/timestamp/rfc3161"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

const (
	maxTimestampRequestBytes  = int64(rfc3161.MaxRequestBytes)
	maxTimestampResponseBytes = int64(rfc3161.MaxResponseBytes)
	maxTimestampCABundleBytes = int64(rfc3161.MaxCABundleBytes)
)

type TimestampState string

const (
	TimestampUnanchored   TimestampState = "unanchored"
	TimestampPending      TimestampState = "pending"
	TimestampVerified     TimestampState = "verified"
	TimestampFailed       TimestampState = "failed"
	TimestampInconsistent TimestampState = "inconsistent"
)

type TimestampEntryResult struct {
	ProviderID        string                       `json:"provider_id,omitempty"`
	TSAURL            string                       `json:"tsa_url"`
	State             ledger.RFC3161TimestampState `json:"state"`
	RequestPath       ledger.RelativePath          `json:"request_path"`
	ResponsePath      ledger.RelativePath          `json:"response_path"`
	CABundlePath      *ledger.RelativePath         `json:"ca_bundle_path,omitempty"`
	RequestPresent    bool                         `json:"request_present"`
	ResponsePresent   bool                         `json:"response_present"`
	CABundlePresent   bool                         `json:"ca_bundle_present"`
	CheckState        LayerState                   `json:"check_state"`
	ReasonCodes       []string                     `json:"reason_codes,omitempty"`
	GenTime           *ledger.Timestamp            `json:"gen_time,omitempty"`
	PolicyOID         *string                      `json:"policy_oid,omitempty"`
	SerialNumber      *string                      `json:"serial_number,omitempty"`
	SignerSubject     string                       `json:"signer_subject,omitempty"`
	SignerFingerprint string                       `json:"signer_fingerprint_sha256,omitempty"`
	CABundleSHA256    string                       `json:"ca_bundle_sha256,omitempty"`
}

type TimestampRequestSummary struct {
	RequestCount int    `json:"request_count"`
	TSAOrigin    string `json:"tsa_origin,omitempty"`
}

type TimestampAttemptResult struct {
	ProviderID string `json:"provider_id"`
	Ordinal    int    `json:"ordinal"`
	Attempted  bool   `json:"attempted"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type TimestampArtifactResult struct {
	QuestionID       ledger.Slug              `json:"question_id"`
	ForecastID       ledger.Slug              `json:"forecast_id"`
	SelectionMode    string                   `json:"selection_mode,omitempty"`
	SelectedProvider string                   `json:"selected_provider,omitempty"`
	Attempts         []TimestampAttemptResult `json:"attempts,omitempty"`
	State            TimestampState           `json:"state"`
	TargetPath       ledger.RelativePath      `json:"target_path"`
	TargetSHA256     string                   `json:"target_sha256"`
	TargetPresent    bool                     `json:"target_present"`
	Entries          []TimestampEntryResult   `json:"timestamps,omitempty"`
	RequestSummary   TimestampRequestSummary  `json:"request_summary,omitempty"`
	NextActions      []string                 `json:"next_actions,omitempty"`
	Warnings         []Warning                `json:"warnings,omitempty"`
	Effects          []SideEffect             `json:"effects,omitempty"`
	Recovery         Recovery                 `json:"recovery,omitempty"`
	FailureCode      app.ErrorCode            `json:"-"`
}

type TimestampVerifyResult struct {
	TimestampArtifactResult
	Verification VerificationLayer `json:"verification"`
	FailureCode  app.ErrorCode     `json:"-"`
}

type TimestampStampOptions struct {
	DryRun       bool
	Offline      bool
	TSAProvider  string
	TSAURL       string
	CABundlePath string
	Effects      Effects
	HTTPClient   *rfc3161.HTTPClient
}

type TimestampVerifyOptions struct {
	DryRun  bool
	Effects Effects
}

type timestampPaths struct {
	Request  ledger.RelativePath
	Response ledger.RelativePath
}

const (
	timestampSelectionAuto   = "auto"
	timestampSelectionNamed  = "named"
	timestampSelectionCustom = "custom"
)

type timestampCandidate struct {
	ProviderID   string
	TSAURL       string
	CABundlePath ledger.RelativePath
	CABundle     []byte
	Profile      *rfc3161.ProviderProfile
}

type timestampSelection struct {
	Mode       string
	Candidates []timestampCandidate
}

func resolveTimestampSelection(options TimestampStampOptions) (timestampSelection, error) {
	hasURL, hasBundle := options.TSAURL != "", options.CABundlePath != ""
	if hasURL != hasBundle {
		return timestampSelection{}, app.NewError(app.CodeInvalidData, "--tsa-url and --ca-bundle must be provided together", nil)
	}
	if hasURL {
		if options.TSAProvider != "" {
			return timestampSelection{}, app.NewError(app.CodeInvalidData, "--tsa-provider cannot be combined with --tsa-url or --ca-bundle", nil)
		}
		normalized, err := rfc3161.NormalizeEndpoint(options.TSAURL)
		if err != nil {
			return timestampSelection{}, app.NewError(app.CodeInvalidData, "--tsa-url must name a public HTTPS timestamp authority without credentials, query, or fragment", nil)
		}
		if err := storage.ValidateRelativePath(options.CABundlePath); err != nil {
			return timestampSelection{}, app.NewError(app.CodeInvalidData, "--ca-bundle must be a safe ledger-relative PEM file", err)
		}
		return timestampSelection{Mode: timestampSelectionCustom, Candidates: []timestampCandidate{{ProviderID: "custom", TSAURL: normalized, CABundlePath: ledger.RelativePath(options.CABundlePath)}}}, nil
	}
	providerID := options.TSAProvider
	if providerID == "" {
		providerID = rfc3161.ProviderAuto
	}
	var profiles []rfc3161.ProviderProfile
	switch providerID {
	case rfc3161.ProviderAuto:
		profiles = rfc3161.Providers()
	default:
		profile, ok := rfc3161.ProviderByID(providerID)
		if !ok {
			return timestampSelection{}, app.NewError(app.CodeInvalidData, "--tsa-provider is not a released provider", nil)
		}
		profiles = []rfc3161.ProviderProfile{profile}
	}
	if err := rfc3161.ValidateProviderCatalog(); err != nil {
		return timestampSelection{}, app.NewError(app.CodeInternal, "built-in timestamp provider catalog is invalid", err)
	}
	mode := timestampSelectionAuto
	if providerID != rfc3161.ProviderAuto {
		mode = timestampSelectionNamed
	}
	selection := timestampSelection{Mode: mode, Candidates: make([]timestampCandidate, len(profiles))}
	for index, profile := range profiles {
		profileCopy := profile
		selection.Candidates[index] = timestampCandidate{ProviderID: profile.ID(), TSAURL: profile.Endpoint(), CABundlePath: ledger.RelativePath(profile.TrustPath()), CABundle: profile.Bundle(), Profile: &profileCopy}
	}
	return selection, nil
}

func TimestampEvidencePaths(forecastID ledger.Slug, tsaURL string) (ledger.RelativePath, ledger.RelativePath, error) {
	normalized, err := rfc3161.NormalizeEndpoint(tsaURL)
	if err != nil {
		return "", "", app.NewError(app.CodeInvalidData, "timestamp authority URL is invalid", nil)
	}
	return timestampEvidencePathsForEndpoint(forecastID, normalized)
}

func timestampEvidencePathsForEndpoint(forecastID ledger.Slug, endpoint string) (ledger.RelativePath, ledger.RelativePath, error) {
	if endpoint == "" {
		return "", "", app.NewError(app.CodeInvalidData, "timestamp authority endpoint is missing", nil)
	}
	digest := sha256.Sum256([]byte(endpoint))
	directory := storage.DeterministicRelativePath("proofs/timestamps", string(forecastID)+"/"+hex.EncodeToString(digest[:8]))
	return ledger.RelativePath(directory + "/request.tsq"), ledger.RelativePath(directory + "/response.tsr"), nil
}

func PlanTimestampStamp(ctx context.Context, path string, questionID, forecastID ledger.Slug, options TimestampStampOptions) (TimestampArtifactResult, error) {
	selection, err := resolveTimestampSelection(options)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	if options.Offline {
		return TimestampArtifactResult{}, app.NewError(app.CodeNetworkDisabled, "timestamp stamp requires network access", nil)
	}
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	artifact, forecast, err := timestampPreflight(loaded.Model, questionID, forecastID)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	root := filepath.Dir(loaded.Path)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	if _, err := preflightTargetFile(root, artifact); err != nil {
		return TimestampArtifactResult{}, err
	}
	result := baseTimestampResult(artifact)
	result.SelectionMode = selection.Mode
	result.Entries = make([]TimestampEntryResult, 0, len(selection.Candidates))
	result.Attempts = make([]TimestampAttemptResult, 0, len(selection.Candidates))
	result.Effects = []SideEffect{{Kind: EffectTarget, Action: EffectCreate, Status: deferredOrUnchanged(regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath))))), Path: string(artifact.RelativePath), Rollback: RollbackCreatedPublic}}
	for index := range selection.Candidates {
		candidate := &selection.Candidates[index]
		caAbsolute, resolveErr := resolver.ResolveForCreate(string(candidate.CABundlePath))
		if resolveErr != nil {
			return result, resolveErr
		}
		if selection.Mode == timestampSelectionCustom {
			caAbsolute, resolveErr = resolver.ResolveLabeled(string(candidate.CABundlePath), true, "CA bundle")
			if resolveErr != nil {
				return result, app.NewError(app.CodeInvalidData, "timestamp CA bundle cannot be resolved inside the ledger root", resolveErr)
			}
			candidate.CABundle, resolveErr = readBoundedFile(caAbsolute, maxTimestampCABundleBytes)
			if resolveErr != nil {
				return result, resolveErr
			}
		} else if existing, readErr := readOptionalBoundedFile(caAbsolute, maxTimestampCABundleBytes); readErr != nil {
			return result, readErr
		} else if existing != nil && !bytes.Equal(existing, candidate.CABundle) {
			return result, app.NewError(app.CodeConflict, "built-in timestamp trust path contains different bytes", nil)
		}
		if validateErr := rfc3161.ValidateCABundle(candidate.CABundle, rfc3161.DefaultLimits()); validateErr != nil {
			return result, app.NewError(app.CodeInvalidData, "timestamp CA bundle is invalid", nil)
		}
		bundleDigest := sha256.Sum256(candidate.CABundle)
		requestPath, responsePath, pathErr := timestampEvidencePathsForEndpoint(forecastID, candidate.TSAURL)
		if pathErr != nil {
			return result, pathErr
		}
		if _, pathErr = resolver.ResolveForCreate(string(requestPath)); pathErr != nil {
			return result, pathErr
		}
		if _, pathErr = resolver.ResolveForCreate(string(responsePath)); pathErr != nil {
			return result, pathErr
		}
		entry := TimestampEntryResult{ProviderID: candidate.ProviderID, TSAURL: candidate.TSAURL, State: ledger.RFC3161Pending, RequestPath: requestPath, ResponsePath: responsePath, CABundlePath: relativePathPointer(candidate.CABundlePath), CABundleSHA256: hex.EncodeToString(bundleDigest[:]), CheckState: LayerNotChecked, CABundlePresent: regularFileExists(caAbsolute)}
		requestAbsolute := filepath.Join(root, filepath.FromSlash(string(requestPath)))
		responseAbsolute := filepath.Join(root, filepath.FromSlash(string(responsePath)))
		if data, readErr := readOptionalBoundedFile(requestAbsolute, maxTimestampRequestBytes); readErr != nil {
			return result, readErr
		} else if data != nil {
			entry.RequestPresent = true
			if _, parseErr := rfc3161.ParseRequest(data, artifact.Bytes, rfc3161.DefaultLimits()); parseErr != nil {
				return result, app.NewError(app.CodeConflict, "existing timestamp request does not match the selected target", nil)
			}
		}
		if data, readErr := readOptionalBoundedFile(responseAbsolute, maxTimestampResponseBytes); readErr != nil {
			return result, readErr
		} else if data != nil {
			entry.ResponsePresent = true
			if !entry.RequestPresent {
				return result, app.NewError(app.CodeConflict, "timestamp response exists without its request", nil)
			}
		}
		for _, existing := range integrityTimestamps(forecast.Integrity) {
			if existing.TSAURL == candidate.TSAURL && (existing.RequestPath != requestPath || existing.ResponsePath != responsePath || existing.CABundlePath == nil || *existing.CABundlePath != candidate.CABundlePath) {
				return result, app.NewError(app.CodeConflict, "timestamp authority already has different retained artifact paths", nil)
			}
		}
		result.Entries = append(result.Entries, entry)
		result.Attempts = append(result.Attempts, TimestampAttemptResult{ProviderID: candidate.ProviderID, Ordinal: index + 1})
		if selection.Mode != timestampSelectionCustom {
			result.Effects = append(result.Effects, SideEffect{Kind: EffectTimestampTrust, Action: EffectCreate, Status: deferredOrUnchanged(entry.CABundlePresent), Path: string(candidate.CABundlePath), Rollback: RollbackCreatedPublic})
		}
		result.Effects = append(result.Effects,
			SideEffect{Kind: EffectTimestampRequest, Action: EffectCreate, Status: deferredOrUnchanged(entry.RequestPresent), Path: string(requestPath), Rollback: RollbackCreatedPublic},
			SideEffect{Kind: EffectNetwork, Action: EffectContact, Status: EffectDeferred, SourceID: candidate.ProviderID},
			SideEffect{Kind: EffectTimestampResponse, Action: EffectCreate, Status: deferredOrUnchanged(entry.ResponsePresent), Path: string(responsePath), Rollback: RollbackCreatedPublic},
		)
	}
	result.Effects = append(result.Effects, SideEffect{Kind: EffectLedger, Action: EffectReplace, Status: EffectDeferred, Path: filepath.Base(loaded.Path)})
	return result, nil
}

func CommitTimestampStamp(ctx context.Context, path string, questionID, forecastID ledger.Slug, options TimestampStampOptions) (TimestampArtifactResult, error) {
	planned, err := PlanTimestampStamp(ctx, path, questionID, forecastID, options)
	if err != nil || options.DryRun {
		return planned, err
	}
	selection, err := resolveTimestampSelection(options)
	if err != nil {
		return planned, err
	}
	if err := options.Effects.Validate(); err != nil {
		options.Effects = ProductionEffects()
	}
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return planned, err
	}
	artifact, forecast, err := timestampPreflight(loaded.Model, questionID, forecastID)
	if err != nil {
		return planned, err
	}
	root := filepath.Dir(loaded.Path)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return planned, err
	}

	// Reuse complete matching evidence before entropy, network, or writes.
	for index, entry := range planned.Entries {
		for _, existing := range integrityTimestamps(forecast.Integrity) {
			if existing.TSAURL != entry.TSAURL || existing.RequestPath != entry.RequestPath || existing.ResponsePath != entry.ResponsePath || existing.CABundlePath == nil || entry.CABundlePath == nil || *existing.CABundlePath != *entry.CABundlePath {
				continue
			}
			metadata, verifyErr := verifyTimestampEntry(ctx, root, artifact.Bytes, existing)
			if verifyErr == nil && existing.State == ledger.RFC3161Verified && existing.GenTime != nil && existing.PolicyOID != nil && existing.SerialNumber != nil && rfc3161.MetadataMatches(metadata, string(*existing.GenTime), *existing.PolicyOID, *existing.SerialNumber, existing.HashAlgorithm) == nil {
				planned.State = TimestampVerified
				planned.SelectedProvider = planned.Entries[index].ProviderID
				planned.Entries = []TimestampEntryResult{entryResultFromMetadata(existing, metadata, LayerPass)}
				planned.Entries[0].ProviderID = selection.Candidates[index].ProviderID
				planned.Effects = nil
				planned.Attempts[index].ReasonCode = "timing.existing_evidence_reused"
				return planned, nil
			}
		}
	}

	client := rfc3161.HTTPClient{}
	if options.HTTPClient != nil {
		client = *options.HTTPClient
	}
	requestCount := 0
	hadInvalidResponse := false
	for index, candidate := range selection.Candidates {
		entry := planned.Entries[index]
		caBytes := candidate.CABundle
		if selection.Mode == timestampSelectionCustom {
			caAbsolute, resolveErr := resolver.ResolveLabeled(string(candidate.CABundlePath), true, "CA bundle")
			if resolveErr != nil {
				return planned, resolveErr
			}
			caBytes, err = readBoundedFile(caAbsolute, maxTimestampCABundleBytes)
			if err != nil {
				return planned, err
			}
		}
		requestAbsolute := filepath.Join(root, filepath.FromSlash(string(entry.RequestPath)))
		responseAbsolute := filepath.Join(root, filepath.FromSlash(string(entry.ResponsePath)))
		requestBytes, readErr := readOptionalBoundedFile(requestAbsolute, maxTimestampRequestBytes)
		if readErr != nil {
			return planned, readErr
		}
		if requestBytes == nil {
			requestBytes, _, err = rfc3161.CreateRequest(artifact.Bytes, &effectsReader{ctx: ctx, random: options.Effects.Random}, rfc3161.DefaultLimits())
			if err != nil {
				return planned, app.NewError(app.CodeIO, "timestamp request nonce could not be generated", nil)
			}
		}
		responseBytes, readErr := readOptionalBoundedFile(responseAbsolute, maxTimestampResponseBytes)
		if readErr != nil {
			return planned, readErr
		}
		if responseBytes == nil {
			planned.Attempts[index].Attempted = true
			var submitted rfc3161.SubmitResult
			var submitErr error
			if candidate.Profile != nil {
				submitted, submitErr = client.SubmitProvider(ctx, *candidate.Profile, requestBytes)
			} else {
				submitted, submitErr = client.Submit(ctx, entry.TSAURL, requestBytes)
			}
			requestCount++
			if submitErr != nil {
				reason := rfc3161.SafeReason(submitErr)
				unavailable := reason == rfc3161.ReasonTransport || reason == rfc3161.ReasonHTTPStatus || reason == rfc3161.ReasonRequestProfile
				if unavailable {
					planned.Entries[index].ReasonCodes = []string{"timing.tsa_unavailable"}
					planned.Attempts[index].ReasonCode = "timing.tsa_unavailable"
				} else {
					hadInvalidResponse = true
					planned.Entries[index].ReasonCodes = []string{string(reason)}
					planned.Attempts[index].ReasonCode = string(reason)
				}
				if selection.Mode == timestampSelectionCustom {
					planned.State = TimestampUnanchored
					planned.RequestSummary = TimestampRequestSummary{RequestCount: requestCount, TSAOrigin: safeTSAOrigin(entry.TSAURL)}
					planned.NextActions = []string{"retry timestamp stamp"}
					planned.Effects = nil
					if unavailable {
						planned.FailureCode = app.CodeNetwork
						return planned, app.NewError(app.CodeNetwork, "timestamp authority request failed", nil)
					}
					planned.FailureCode = app.CodeVerification
					return planned, app.NewError(app.CodeVerification, "timestamp authority returned an invalid response", nil)
				}
				continue
			}
			responseBytes = submitted.Response
		}
		metadata, verifyErr := rfc3161.Verify(ctx, artifact.Bytes, requestBytes, responseBytes, caBytes, rfc3161.DefaultLimits())
		if verifyErr != nil && selection.Mode != timestampSelectionCustom {
			hadInvalidResponse = true
			reason := string(rfc3161.SafeReason(verifyErr))
			planned.Entries[index].ReasonCodes = []string{reason}
			planned.Attempts[index].ReasonCode = reason
			continue
		}
		verifiedAt := ledger.Timestamp(options.Effects.Clock.Now().UTC().Format(time.RFC3339Nano))
		planned.SelectedProvider = candidate.ProviderID
		planned.RequestSummary = TimestampRequestSummary{RequestCount: requestCount, TSAOrigin: safeTSAOrigin(entry.TSAURL)}
		committed, commitErr := commitTimestampEvidence(ctx, loaded.Path, questionID, forecastID, artifact, entry, requestBytes, responseBytes, caBytes, metadata, verifiedAt, verifyErr == nil, selection.Mode != timestampSelectionCustom, planned)
		if commitErr != nil {
			return committed, commitErr
		}
		if verifyErr != nil {
			committed.Warnings = append(committed.Warnings, Warning{Code: string(rfc3161.SafeReason(verifyErr)), Message: "The response was retained as pending because complete local verification did not pass."})
			committed.FailureCode = app.CodePending
			return committed, app.NewError(app.CodePending, "timestamp response was retained but is not verified", nil)
		}
		return committed, nil
	}

	planned.State = TimestampUnanchored
	planned.RequestSummary = TimestampRequestSummary{RequestCount: requestCount}
	planned.NextActions = []string{"retry timestamp stamp"}
	planned.Effects = nil
	if hadInvalidResponse {
		planned.FailureCode = app.CodeVerification
		return planned, app.NewError(app.CodeVerification, "timestamp providers returned no locally verifiable response", nil)
	}
	planned.FailureCode = app.CodeNetwork
	return planned, app.NewError(app.CodeNetwork, "all timestamp providers were unavailable", nil)
}

func TimestampStatusFor(ctx context.Context, path string, questionID, forecastID ledger.Slug) (TimestampArtifactResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	artifact, forecast, err := timestampPreflight(loaded.Model, questionID, forecastID)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	result := baseTimestampResult(artifact)
	root := filepath.Dir(loaded.Path)
	result.TargetPresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath))))
	switch {
	case forecast.Integrity.Unanchored != nil:
		result.State = TimestampUnanchored
		result.NextActions = []string{"timestamp stamp --tsa-url <url> --ca-bundle <relative.pem>"}
		return result, nil
	case forecast.Integrity.Failed != nil:
		result.State = TimestampFailed
		result.NextActions = []string{"forecast add --supersedes-forecast-id " + string(forecastID)}
		return result, nil
	}
	result.Entries = inspectTimestampEntries(ctx, root, artifact.Bytes, integrityTimestamps(forecast.Integrity))
	result.State = stateFromEntries(result.Entries)
	if result.State == TimestampPending || result.State == TimestampInconsistent {
		result.NextActions = []string{"timestamp verify"}
	}
	return result, nil
}

func CommitTimestampVerify(ctx context.Context, path string, questionID, forecastID ledger.Slug, options TimestampVerifyOptions) (TimestampVerifyResult, error) {
	if err := options.Effects.Validate(); err != nil {
		options.Effects = ProductionEffects()
	}
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return TimestampVerifyResult{}, err
	}
	artifact, forecast, err := timestampPreflight(loaded.Model, questionID, forecastID)
	if err != nil {
		return TimestampVerifyResult{}, err
	}
	result := TimestampVerifyResult{TimestampArtifactResult: baseTimestampResult(artifact)}
	root := filepath.Dir(loaded.Path)
	result.TargetPresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath))))
	entries := integrityTimestamps(forecast.Integrity)
	if len(entries) == 0 {
		result.State = TimestampUnanchored
		result.Verification = VerificationLayer{Name: "existence_timing", State: LayerNotApplicable, ReasonCodes: []string{"timing.no_evidence"}}
		return result, nil
	}
	result.Entries = inspectTimestampEntries(ctx, root, artifact.Bytes, entries)
	verified := make(map[string]rfc3161.Metadata)
	hasPending := false
	allFailed := true
	for index, item := range result.Entries {
		key := timestampEntryKey(entries[index])
		switch item.CheckState {
		case LayerPass:
			metadata, verifyErr := verifyTimestampEntry(ctx, root, artifact.Bytes, entries[index])
			if verifyErr == nil {
				verified[key] = metadata
				allFailed = false
			}
		case LayerPending, LayerNotChecked:
			hasPending = true
			allFailed = false
		}
	}
	if len(verified) == 0 {
		if hasPending || !allFailed {
			result.State = TimestampPending
			result.Verification = VerificationLayer{Name: "existence_timing", State: LayerPending, ReasonCodes: []string{"timing.local_evidence_incomplete"}, Evidence: map[string]any{"timestamps": result.Entries}}
			result.FailureCode = app.CodePending
			return result, nil
		}
		result.State = TimestampInconsistent
		result.Verification = VerificationLayer{Name: "existence_timing", State: LayerFail, ReasonCodes: []string{"timing.all_responses_failed"}, Evidence: map[string]any{"timestamps": result.Entries}}
		result.FailureCode = app.CodeVerification
		return result, nil
	}
	result.State = TimestampVerified
	result.Verification = VerificationLayer{Name: "existence_timing", State: LayerPass, ReasonCodes: []string{"timing.rfc3161_verified"}, Evidence: map[string]any{"timestamps": result.Entries}, Limitations: timestampLimitations()}
	if options.DryRun {
		result.Effects = []SideEffect{{Kind: EffectLedger, Action: EffectReplace, Status: EffectDeferred, Path: filepath.Base(loaded.Path)}}
		return result, nil
	}
	verifiedAt := ledger.Timestamp(options.Effects.Clock.Now().UTC().Format(time.RFC3339Nano))
	committed, err := promoteTimestampEntries(ctx, loaded.Path, questionID, forecastID, artifact, verified, verifiedAt, result.TimestampArtifactResult)
	result.TimestampArtifactResult = committed
	return result, err
}

func timestampPreflight(model *ledger.Ledger, questionID, forecastID ledger.Slug) (TargetArtifact, ledger.Forecast, error) {
	artifact, err := BuildForecastTarget(model, questionID, forecastID)
	if err != nil {
		return TargetArtifact{}, ledger.Forecast{}, err
	}
	_, _, _, forecast, err := selectForecast(model, questionID, forecastID)
	if err != nil {
		return TargetArtifact{}, ledger.Forecast{}, err
	}
	if forecast.Integrity.Failed != nil {
		return TargetArtifact{}, ledger.Forecast{}, app.NewError(app.CodeConflict, "failed integrity is terminal; append a new forecast revision", nil)
	}
	return artifact, forecast, nil
}

func commitTimestampEvidence(ctx context.Context, path string, questionID, forecastID ledger.Slug, artifact TargetArtifact, entry TimestampEntryResult, requestBytes, responseBytes, caBytes []byte, metadata rfc3161.Metadata, verifiedAt ledger.Timestamp, verified, materializeCA bool, result TimestampArtifactResult) (TimestampArtifactResult, error) {
	root := filepath.Dir(path)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return result, err
	}
	if entry.CABundlePath == nil {
		return result, app.NewError(app.CodeInternal, "timestamp CA bundle path is missing", nil)
	}
	caAbsolute, err := resolver.ResolveForCreate(string(*entry.CABundlePath))
	if !materializeCA {
		caAbsolute, err = resolver.ResolveLabeled(string(*entry.CABundlePath), true, "CA bundle")
	}
	if err != nil {
		return result, err
	}
	requestAbsolute, err := resolver.ResolveForCreate(string(entry.RequestPath))
	if err != nil {
		return result, err
	}
	responseAbsolute, err := resolver.ResolveForCreate(string(entry.ResponsePath))
	if err != nil {
		return result, err
	}
	targetAbsolute, err := resolver.ResolveForCreate(string(artifact.RelativePath))
	if err != nil {
		return result, err
	}
	journal := filepath.Join(root, "."+filepath.Base(path)+".timestamp-resources.json")
	resources := []storage.ResourceEntry{
		resourceEntry(storage.ResourceTarget, targetAbsolute),
	}
	if materializeCA {
		resources = append(resources, resourceEntry(storage.ResourceTimestampTrust, caAbsolute))
	}
	resources = append(resources, resourceEntry(storage.ResourceTimestampRequest, requestAbsolute), resourceEntry(storage.ResourceTimestampResponse, responseAbsolute))
	var plan *storage.ResourcePlan
	finishRetained := func(cause error) (TimestampArtifactResult, error) {
		if plan == nil {
			return result, cause
		}
		if finishErr := plan.Finish(); finishErr != nil {
			result.Recovery = Recovery{State: RecoveryRequired, Message: "Timestamp artifacts were retained, but resource journal cleanup needs attention.", Paths: []string{filepath.Base(journal)}}
			return result, errors.Join(cause, finishErr)
		}
		paths := []string{string(artifact.RelativePath), string(entry.RequestPath), string(entry.ResponsePath)}
		if materializeCA {
			paths = append(paths, string(*entry.CABundlePath))
		}
		result.Recovery = Recovery{State: RecoveryRetained, Message: "Timestamp resources are durable and can be reused by a retry.", Paths: paths, Actions: []string{"retry timestamp stamp"}}
		return result, cause
	}
	artifacts := os.DirFS(root)
	err = storage.UpdateLedger(ctx, path, storage.TransactionOptions{Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) }, Mutate: func(parsed *document.Document) ([]byte, error) {
		model, decodeErr := validation.DecodeLedger(parsed)
		if decodeErr != nil {
			return nil, decodeErr
		}
		questionPosition, _, forecastPosition, forecast, selectErr := selectForecast(model, questionID, forecastID)
		if selectErr != nil {
			return nil, selectErr
		}
		if forecast.Integrity.Failed != nil {
			return nil, app.NewError(app.CodeConflict, "failed integrity is terminal; append a new forecast revision", nil)
		}
		currentArtifact, _, preflightErr := timestampPreflight(model, questionID, forecastID)
		if preflightErr != nil || currentArtifact.SHA256 != artifact.SHA256 || !bytes.Equal(currentArtifact.Bytes, artifact.Bytes) {
			return nil, app.NewError(app.CodeConflict, "forecast target changed while the timestamp authority request was in flight", nil)
		}
		currentCA, caErr := readOptionalBoundedFile(caAbsolute, maxTimestampCABundleBytes)
		if caErr != nil || currentCA != nil && !bytes.Equal(currentCA, caBytes) || !materializeCA && currentCA == nil {
			return nil, app.NewError(app.CodeConflict, "timestamp CA bundle changed while the request was in flight", nil)
		}
		if recordedForecastTarget(model, questionID, forecastID) != nil && !sameTargetMetadata(*recordedForecastTarget(model, questionID, forecastID), TargetMetadataFor(artifact)) {
			return nil, app.NewError(app.CodeConflict, "recorded target metadata changed before timestamp commit", nil)
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(requestAbsolute), 0o755); mkdirErr != nil {
			return nil, app.NewError(app.CodeIO, "timestamp artifact directory cannot be created", mkdirErr)
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(targetAbsolute), 0o755); mkdirErr != nil {
			return nil, app.NewError(app.CodeIO, "target artifact directory cannot be created", mkdirErr)
		}
		if materializeCA {
			if mkdirErr := os.MkdirAll(filepath.Dir(caAbsolute), 0o755); mkdirErr != nil {
				return nil, app.NewError(app.CodeIO, "timestamp trust directory cannot be created", mkdirErr)
			}
		}
		plan, decodeErr = storage.NewResourcePlan(journal, string(OperationTimestampStamp), resources)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if beginErr := plan.Begin(); beginErr != nil {
			plan = nil
			return nil, beginErr
		}
		writeItems := []struct {
			path string
			data []byte
			mode os.FileMode
			max  int64
		}{{targetAbsolute, artifact.Bytes, 0o644, maxTargetBytes}}
		if materializeCA {
			writeItems = append(writeItems, struct {
				path string
				data []byte
				mode os.FileMode
				max  int64
			}{caAbsolute, caBytes, 0o644, maxTimestampCABundleBytes})
		}
		writeItems = append(writeItems,
			struct {
				path string
				data []byte
				mode os.FileMode
				max  int64
			}{requestAbsolute, requestBytes, 0o644, maxTimestampRequestBytes},
			struct {
				path string
				data []byte
				mode os.FileMode
				max  int64
			}{responseAbsolute, responseBytes, 0o644, maxTimestampResponseBytes},
		)
		for index, item := range writeItems {
			written, writeErr := storage.EnsureDeterministicFile(item.path, item.data, item.mode, item.max)
			if writeErr != nil {
				return nil, writeErr
			}
			if written.State == storage.DeterministicCreated {
				if markErr := plan.MarkCreated(item.path, written.SHA256); markErr != nil {
					return nil, markErr
				}
			}
			resources[index].State = storage.ResourceCommitted
			if markErr := plan.MarkCommitted(item.path); markErr != nil {
				return nil, markErr
			}
		}
		timestamp := timestampRecord(entry, metadata, verified)
		updated, updateErr := integrityWithTimestamp(forecast.Integrity, TargetMetadataFor(artifact), timestamp, verifiedAt)
		if updateErr != nil {
			return nil, updateErr
		}
		value, encodeErr := jsonPatchValue(updated)
		if encodeErr != nil {
			return nil, encodeErr
		}
		pointer := fmt.Sprintf("/questions/%d/forecasts/%d/integrity", questionPosition, forecastPosition)
		return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchReplace, Pointer: pointer, Value: value}})
	}})
	if err != nil {
		return finishRetained(err)
	}
	if plan == nil {
		return result, app.NewError(app.CodeInternal, "timestamp resource plan was not committed", nil)
	}
	if err := plan.Finish(); err != nil {
		result.Recovery = Recovery{State: RecoveryRequired, Message: "Timestamp evidence was committed, but resource journal cleanup needs attention.", Paths: []string{filepath.Base(journal)}}
		return result, err
	}
	result.State = TimestampPending
	if verified {
		result.State = TimestampVerified
	}
	result.TargetPresent = true
	result.Entries = []TimestampEntryResult{entryResultFromMetadata(timestampRecord(entry, metadata, verified), metadata, LayerPass)}
	result.Entries[0].ProviderID = entry.ProviderID
	if !verified {
		result.Entries[0].CheckState = LayerFail
	}
	result.Effects = []SideEffect{
		{Kind: EffectTarget, Action: EffectCreate, Status: EffectCompleted, Path: string(artifact.RelativePath)},
	}
	if materializeCA {
		result.Effects = append(result.Effects, SideEffect{Kind: EffectTimestampTrust, Action: EffectCreate, Status: EffectCompleted, Path: string(*entry.CABundlePath)})
	}
	result.Effects = append(result.Effects,
		SideEffect{Kind: EffectTimestampRequest, Action: EffectCreate, Status: EffectCompleted, Path: string(entry.RequestPath)},
		SideEffect{Kind: EffectTimestampResponse, Action: EffectCreate, Status: EffectCompleted, Path: string(entry.ResponsePath)},
		SideEffect{Kind: EffectLedger, Action: EffectReplace, Status: EffectCompleted, Path: filepath.Base(path)},
	)
	result.Recovery = Recovery{State: RecoveryNone}
	return result, nil
}

func promoteTimestampEntries(ctx context.Context, path string, questionID, forecastID ledger.Slug, artifact TargetArtifact, verified map[string]rfc3161.Metadata, verifiedAt ledger.Timestamp, result TimestampArtifactResult) (TimestampArtifactResult, error) {
	root := filepath.Dir(path)
	err := storage.UpdateLedger(ctx, path, storage.TransactionOptions{Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, os.DirFS(root)) }, Mutate: func(parsed *document.Document) ([]byte, error) {
		model, decodeErr := validation.DecodeLedger(parsed)
		if decodeErr != nil {
			return nil, decodeErr
		}
		questionPosition, _, forecastPosition, forecast, selectErr := selectForecast(model, questionID, forecastID)
		if selectErr != nil {
			return nil, selectErr
		}
		if recordedForecastTarget(model, questionID, forecastID) == nil || !sameTargetMetadata(*recordedForecastTarget(model, questionID, forecastID), TargetMetadataFor(artifact)) {
			return nil, app.NewError(app.CodeVerification, "stored target metadata does not match the selected forecast", nil)
		}
		timestamps := integrityTimestamps(forecast.Integrity)
		for index := range timestamps {
			metadata, ok := verified[timestampEntryKey(timestamps[index])]
			if !ok {
				continue
			}
			applyVerifiedMetadata(&timestamps[index], metadata)
		}
		var external *[]ledger.ExternalAnchor
		if forecast.Integrity.Pending != nil {
			external = forecast.Integrity.Pending.ExternalAnchors
		} else if forecast.Integrity.Verified != nil {
			external = forecast.Integrity.Verified.ExternalAnchors
		}
		updated := ledger.Integrity{Verified: &ledger.VerifiedIntegrity{Status: ledger.IntegrityVerified, Target: TargetMetadataFor(artifact), Timestamps: timestamps, VerifiedAt: verifiedAt, ExternalAnchors: external}}
		value, encodeErr := jsonPatchValue(updated)
		if encodeErr != nil {
			return nil, encodeErr
		}
		pointer := fmt.Sprintf("/questions/%d/forecasts/%d/integrity", questionPosition, forecastPosition)
		return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchReplace, Pointer: pointer, Value: value}})
	}})
	if err != nil {
		return result, err
	}
	result.State = TimestampVerified
	result.Effects = []SideEffect{{Kind: EffectLedger, Action: EffectReplace, Status: EffectCompleted, Path: filepath.Base(path)}}
	return result, nil
}

func inspectTimestampEntries(ctx context.Context, root string, target []byte, entries []ledger.RFC3161Timestamp) []TimestampEntryResult {
	result := make([]TimestampEntryResult, len(entries))
	for index, entry := range entries {
		item := TimestampEntryResult{TSAURL: entry.TSAURL, State: entry.State, RequestPath: entry.RequestPath, ResponsePath: entry.ResponsePath, CABundlePath: cloneRelativePath(entry.CABundlePath), GenTime: cloneTimestamp(entry.GenTime), PolicyOID: cloneString(entry.PolicyOID), SerialNumber: cloneString(entry.SerialNumber), CheckState: LayerNotChecked}
		item.RequestPresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(entry.RequestPath))))
		item.ResponsePresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(entry.ResponsePath))))
		item.CABundlePresent = entry.CABundlePath != nil && regularFileExists(filepath.Join(root, filepath.FromSlash(string(*entry.CABundlePath))))
		if !item.RequestPresent || !item.ResponsePresent || !item.CABundlePresent {
			item.CheckState = LayerPending
			item.ReasonCodes = []string{"timing.retained_artifact_missing"}
			result[index] = item
			continue
		}
		metadata, err := verifyTimestampEntry(ctx, root, target, entry)
		if err != nil {
			item.CheckState = LayerFail
			item.ReasonCodes = []string{string(rfc3161.SafeReason(err))}
			result[index] = item
			continue
		}
		if entry.State == ledger.RFC3161Verified {
			if entry.GenTime == nil || entry.PolicyOID == nil || entry.SerialNumber == nil || rfc3161.MetadataMatches(metadata, string(*entry.GenTime), *entry.PolicyOID, *entry.SerialNumber, entry.HashAlgorithm) != nil {
				item.CheckState = LayerFail
				item.ReasonCodes = []string{string(rfc3161.ReasonMetadata)}
				result[index] = item
				continue
			}
		}
		item = entryResultFromMetadata(entry, metadata, LayerPass)
		result[index] = item
	}
	return result
}

func verifyTimestampEntry(ctx context.Context, root string, target []byte, entry ledger.RFC3161Timestamp) (rfc3161.Metadata, error) {
	if entry.CABundlePath == nil {
		return rfc3161.Metadata{}, &rfc3161.Error{Reason: rfc3161.ReasonTrustBundle, Message: "timestamp CA bundle path is missing"}
	}
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	requestPath, err := resolver.ResolveLabeled(string(entry.RequestPath), true, "timestamp request")
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	responsePath, err := resolver.ResolveLabeled(string(entry.ResponsePath), true, "timestamp response")
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	caPath, err := resolver.ResolveLabeled(string(*entry.CABundlePath), true, "CA bundle")
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	request, err := readBoundedFile(requestPath, maxTimestampRequestBytes)
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	response, err := readBoundedFile(responsePath, maxTimestampResponseBytes)
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	ca, err := readBoundedFile(caPath, maxTimestampCABundleBytes)
	if err != nil {
		return rfc3161.Metadata{}, err
	}
	return rfc3161.Verify(ctx, target, request, response, ca, rfc3161.DefaultLimits())
}

func integrityTimestamps(integrity ledger.Integrity) []ledger.RFC3161Timestamp {
	switch {
	case integrity.Pending != nil:
		return append([]ledger.RFC3161Timestamp(nil), integrity.Pending.Timestamps...)
	case integrity.Verified != nil:
		return append([]ledger.RFC3161Timestamp(nil), integrity.Verified.Timestamps...)
	case integrity.Failed != nil && integrity.Failed.Timestamps != nil:
		return append([]ledger.RFC3161Timestamp(nil), (*integrity.Failed.Timestamps)...)
	default:
		return nil
	}
}

func integrityWithTimestamp(current ledger.Integrity, target ledger.ForecastTarget, timestamp ledger.RFC3161Timestamp, verifiedAt ledger.Timestamp) (ledger.Integrity, error) {
	timestamps := integrityTimestamps(current)
	replaced := false
	for index := range timestamps {
		if timestampEntryKey(timestamps[index]) == timestampEntryKey(timestamp) {
			timestamps[index] = timestamp
			replaced = true
		}
	}
	if !replaced {
		timestamps = append(timestamps, timestamp)
	}
	var external *[]ledger.ExternalAnchor
	var existingVerifiedAt ledger.Timestamp
	if current.Pending != nil {
		external = current.Pending.ExternalAnchors
	}
	if current.Verified != nil {
		external = current.Verified.ExternalAnchors
		existingVerifiedAt = current.Verified.VerifiedAt
	}
	hasVerified := false
	for _, item := range timestamps {
		hasVerified = hasVerified || item.State == ledger.RFC3161Verified
	}
	if hasVerified {
		if existingVerifiedAt != "" {
			verifiedAt = existingVerifiedAt
		}
		return ledger.Integrity{Verified: &ledger.VerifiedIntegrity{Status: ledger.IntegrityVerified, Target: target, Timestamps: timestamps, VerifiedAt: verifiedAt, ExternalAnchors: external}}, nil
	}
	return ledger.Integrity{Pending: &ledger.PendingIntegrity{Status: ledger.IntegrityPending, Target: target, Timestamps: timestamps, ExternalAnchors: external}}, nil
}

func timestampRecord(entry TimestampEntryResult, metadata rfc3161.Metadata, verified bool) ledger.RFC3161Timestamp {
	result := ledger.RFC3161Timestamp{Type: "rfc3161", RequestPath: entry.RequestPath, ResponsePath: entry.ResponsePath, TSAURL: entry.TSAURL, HashAlgorithm: rfc3161.HashAlgorithm, State: ledger.RFC3161Pending, CABundlePath: cloneRelativePath(entry.CABundlePath)}
	if verified {
		applyVerifiedMetadata(&result, metadata)
	}
	return result
}

func applyVerifiedMetadata(target *ledger.RFC3161Timestamp, metadata rfc3161.Metadata) {
	genTime := ledger.Timestamp(metadata.GenTime.Format(time.RFC3339Nano))
	policy, serial := metadata.PolicyOID, metadata.SerialNumber
	target.State, target.HashAlgorithm = ledger.RFC3161Verified, rfc3161.HashAlgorithm
	target.GenTime, target.PolicyOID, target.SerialNumber = &genTime, &policy, &serial
}

func entryResultFromMetadata(entry ledger.RFC3161Timestamp, metadata rfc3161.Metadata, state LayerState) TimestampEntryResult {
	providerID := ""
	if profile, ok := rfc3161.ProviderByEndpoint(entry.TSAURL); ok {
		providerID = profile.ID()
	}
	return TimestampEntryResult{ProviderID: providerID, TSAURL: entry.TSAURL, State: entry.State, RequestPath: entry.RequestPath, ResponsePath: entry.ResponsePath, CABundlePath: cloneRelativePath(entry.CABundlePath), RequestPresent: true, ResponsePresent: true, CABundlePresent: entry.CABundlePath != nil, CheckState: state, GenTime: cloneTimestamp(entry.GenTime), PolicyOID: cloneString(entry.PolicyOID), SerialNumber: cloneString(entry.SerialNumber), SignerSubject: metadata.SignerSubject, SignerFingerprint: metadata.SignerFingerprint, CABundleSHA256: metadata.CABundleSHA256}
}

func baseTimestampResult(artifact TargetArtifact) TimestampArtifactResult {
	return TimestampArtifactResult{QuestionID: artifact.QuestionID, ForecastID: artifact.ForecastID, State: TimestampPending, TargetPath: artifact.RelativePath, TargetSHA256: artifact.SHA256, Recovery: Recovery{State: RecoveryNone}}
}

func stateFromEntries(entries []TimestampEntryResult) TimestampState {
	if len(entries) == 0 {
		return TimestampUnanchored
	}
	for _, entry := range entries {
		if entry.CheckState == LayerPass && entry.State == ledger.RFC3161Verified {
			return TimestampVerified
		}
	}
	for _, entry := range entries {
		if entry.CheckState == LayerPending || entry.CheckState == LayerNotChecked || entry.State == ledger.RFC3161Pending {
			return TimestampPending
		}
	}
	return TimestampInconsistent
}

func resourceEntry(kind storage.ResourceKind, path string) storage.ResourceEntry {
	owned := !regularFileExists(path)
	return storage.ResourceEntry{Kind: kind, Type: storage.ResourceFile, Path: path, Owned: owned, Rollback: storage.ResourceRollbackNone, State: storage.ResourcePlanned}
}

func readOptionalBoundedFile(path string, limit int64) ([]byte, error) {
	data, err := readBoundedFile(path, limit)
	if app.ErrorCodeOf(err) == app.CodeNotFound {
		return nil, nil
	}
	return data, err
}

func deferredOrUnchanged(exists bool) EffectStatus {
	if exists {
		return EffectUnchanged
	}
	return EffectDeferred
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func safeTSAOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "tsa"
	}
	return strings.ToLower(parsed.Hostname())
}

func timestampEntryKey(value ledger.RFC3161Timestamp) string {
	return value.TSAURL + "\x00" + string(value.RequestPath) + "\x00" + string(value.ResponsePath)
}

func relativePathPointer(value ledger.RelativePath) *ledger.RelativePath { return &value }

func cloneRelativePath(value *ledger.RelativePath) *ledger.RelativePath {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func sameTargetMetadata(left, right ledger.ForecastTarget) bool {
	return left.Scope == right.Scope && left.Canonicalization == right.Canonicalization && left.ArtifactPath == right.ArtifactPath && left.Digest == right.Digest
}

func timestampLimitations() []string {
	return []string{
		"A valid signature and retained certificate chain do not prove that the timestamp authority clock was honest.",
		"The retained CA bundle does not establish current revocation status or long-term legal validity.",
		"Timestamp evidence does not prove authorship, forecast completeness, outcome truth, or exact self-reported forecast time.",
	}
}

type effectsReader struct {
	ctx    context.Context
	random CSPRNG
	err    error
}

func (r *effectsReader) Read(destination []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	if r.random == nil {
		return 0, io.ErrUnexpectedEOF
	}
	if err := r.random.ReadFull(r.ctx, destination); err != nil {
		r.err = err
		return 0, err
	}
	return len(destination), nil
}
