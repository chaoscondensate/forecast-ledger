package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/chaoscondensate/forecast-ledger/internal/publication"
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
	HeadEventID      ledger.Slug              `json:"head_event_id,omitempty"`
	Scope            TargetScope              `json:"scope"`
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
	Scope        TargetScope
	HeadEventID  ledger.Slug
}

type TimestampVerifyOptions struct {
	DryRun      bool
	Effects     Effects
	Scope       TargetScope
	HeadEventID ledger.Slug
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
		if !strings.HasPrefix(options.CABundlePath, "trust/") {
			return timestampSelection{}, app.NewError(app.CodeInvalidData, "--ca-bundle must be inside the managed trust/ directory", nil)
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

func timestampEvidencePathsForArtifact(artifact TargetArtifact, endpoint string) (ledger.RelativePath, ledger.RelativePath, error) {
	evidenceID := artifact.ForecastID
	if artifact.Scope == LifecycleTargetSchema {
		evidenceID = ledger.Slug(string(artifact.ForecastID) + ".lifecycle." + string(artifact.HeadEventID))
	}
	return timestampEvidencePathsForEndpoint(evidenceID, endpoint)
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
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	selection, err := resolveTimestampSelection(options)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	if options.Offline {
		return TimestampArtifactResult{}, app.NewError(app.CodeNetworkDisabled, "timestamp stamp requires network access", nil)
	}
	subject, err := timestampPreflightScoped(loaded.Model, questionID, forecastID, options.Scope, options.HeadEventID)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	artifact := subject.Artifact
	root := filepath.Dir(loaded.Path)
	reconciled, err := ReconcileEvidenceStore(ctx, loaded)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	if err := timestampReconciliationError(reconciled, selection, artifact); err != nil {
		return TimestampArtifactResult{}, err
	}
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
	result.Effects = []SideEffect{{Kind: EffectEvidenceIndex, Action: EffectReplace, Status: EffectDeferred, Path: publication.EvidenceIndexPath}}
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
		requestPath, responsePath, pathErr := timestampEvidencePathsForArtifact(artifact, candidate.TSAURL)
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
		for _, existing := range timestampSubjectTimestamps(subject) {
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
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return planned, err
	}
	subject, err := timestampPreflightScoped(loaded.Model, questionID, forecastID, options.Scope, options.HeadEventID)
	if err != nil {
		return planned, err
	}
	artifact := subject.Artifact
	root := filepath.Dir(loaded.Path)
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return planned, err
	}

	// Reuse complete matching evidence before entropy, network, or writes.
	for index, entry := range planned.Entries {
		for _, existing := range timestampSubjectTimestamps(subject) {
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
		committed, commitErr := commitTimestampEvidence(ctx, loaded.Path, questionID, forecastID, options.Scope, options.HeadEventID, artifact, entry, requestBytes, responseBytes, caBytes, metadata, verifiedAt, verifyErr == nil, selection.Mode != timestampSelectionCustom, planned)
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
	return TimestampStatusForScoped(ctx, path, questionID, forecastID, TargetScopeForecast, "")
}

func TimestampStatusForScoped(ctx context.Context, path string, questionID, forecastID ledger.Slug, scope TargetScope, headEventID ledger.Slug) (TimestampArtifactResult, error) {
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	subject, err := timestampPreflightScoped(loaded.Model, questionID, forecastID, scope, headEventID)
	if err != nil {
		return TimestampArtifactResult{}, err
	}
	artifact, forecast := subject.Artifact, subject.Forecast
	result := baseTimestampResult(artifact)
	root := filepath.Dir(loaded.Path)
	result.TargetPresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath))))
	switch {
	case subject.LifecycleCheckpoint == nil && artifact.Scope == LifecycleTargetSchema:
		result.State = TimestampUnanchored
		result.NextActions = []string{"timestamp stamp --scope lifecycle --head " + string(headEventID)}
		return result, nil
	case artifact.Scope == ForecastEnvelopeSchema && forecast.Integrity.Unanchored != nil:
		result.State = TimestampUnanchored
		result.NextActions = []string{"timestamp stamp --tsa-url <url> --ca-bundle <relative.pem>"}
		return result, nil
	case timestampSubjectFailed(subject):
		result.State = TimestampFailed
		result.NextActions = []string{"forecast add --supersedes-forecast-id " + string(forecastID)}
		return result, nil
	}
	result.Entries = inspectTimestampEntries(ctx, root, artifact.Bytes, timestampSubjectTimestamps(subject))
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
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return TimestampVerifyResult{}, err
	}
	subject, err := timestampPreflightScoped(loaded.Model, questionID, forecastID, options.Scope, options.HeadEventID)
	if err != nil {
		return TimestampVerifyResult{}, err
	}
	artifact := subject.Artifact
	result := TimestampVerifyResult{TimestampArtifactResult: baseTimestampResult(artifact)}
	root := filepath.Dir(loaded.Path)
	result.TargetPresent = regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath))))
	entries := timestampSubjectTimestamps(subject)
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
	committed, err := promoteTimestampEntries(ctx, loaded.Path, questionID, forecastID, options.Scope, options.HeadEventID, artifact, verified, verifiedAt, result.TimestampArtifactResult)
	result.TimestampArtifactResult = committed
	return result, err
}

func timestampPreflight(model *ledger.Ledger, questionID, forecastID ledger.Slug) (TargetArtifact, ledger.Forecast, error) {
	subject, err := timestampPreflightScoped(model, questionID, forecastID, TargetScopeForecast, "")
	return subject.Artifact, subject.Forecast, err
}

type timestampSubject struct {
	Artifact            TargetArtifact
	Forecast            ledger.Forecast
	LifecycleCheckpoint *ledger.ActivityCheckpoint
}

func timestampPreflightScoped(model *ledger.Ledger, questionID, forecastID ledger.Slug, scope TargetScope, headEventID ledger.Slug) (timestampSubject, error) {
	if scope == "" {
		scope = TargetScopeForecast
	}
	if err := validateTargetSelection(scope, false, questionID, forecastID, headEventID); err != nil {
		return timestampSubject{}, err
	}
	artifact, err := BuildForecastTarget(model, questionID, forecastID)
	if scope == TargetScopeLifecycle {
		artifact, err = BuildLifecycleTarget(model, questionID, forecastID, headEventID)
	}
	if err != nil {
		return timestampSubject{}, err
	}
	_, _, _, forecast, err := selectForecast(model, questionID, forecastID)
	if err != nil {
		return timestampSubject{}, err
	}
	subject := timestampSubject{Artifact: artifact, Forecast: forecast}
	if scope == TargetScopeForecast {
		target := recordedForecastTarget(model, questionID, forecastID)
		if target == nil {
			return timestampSubject{}, app.NewError(app.CodeConflict, "timestamp stamp requires a retained forecast target; run target build first", nil)
		}
		if target.Scope != artifact.Scope || target.Canonicalization != TargetCanonicalization || target.Digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
			return timestampSubject{}, app.NewError(app.CodeVerification, "recorded forecast target metadata does not match its canonical envelope", nil)
		}
		artifact.RelativePath = target.ArtifactPath
		subject.Artifact = artifact
	}
	if scope == TargetScopeLifecycle && forecast.ActivityCheckpoints != nil {
		for index := range *forecast.ActivityCheckpoints {
			if (*forecast.ActivityCheckpoints)[index].HeadEventID == headEventID {
				checkpoint := (*forecast.ActivityCheckpoints)[index]
				if target := lifecycleIntegrityTarget(checkpoint.Integrity); target != nil {
					if target.Scope != artifact.Scope || target.Canonicalization != TargetCanonicalization || target.Digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
						return timestampSubject{}, app.NewError(app.CodeVerification, "recorded lifecycle target metadata does not match its canonical prefix", nil)
					}
					artifact.RelativePath = target.ArtifactPath
				}
				subject.LifecycleCheckpoint = &checkpoint
				subject.Artifact = artifact
				break
			}
		}
	}
	if scope == TargetScopeLifecycle && subject.LifecycleCheckpoint == nil {
		return timestampSubject{}, app.NewError(app.CodeConflict, "timestamp stamp requires a retained lifecycle checkpoint; run target build first", nil)
	}
	if timestampSubjectFailed(subject) {
		return timestampSubject{}, app.NewError(app.CodeConflict, "failed integrity is terminal for the selected evidence scope", nil)
	}
	return subject, nil
}

func lifecycleIntegrityTarget(integrity ledger.LifecycleIntegrity) *ledger.LifecycleTarget {
	switch {
	case integrity.Retained != nil:
		return &integrity.Retained.Target
	case integrity.Pending != nil:
		return &integrity.Pending.Target
	case integrity.Verified != nil:
		return &integrity.Verified.Target
	case integrity.Failed != nil:
		return integrity.Failed.Target
	default:
		return nil
	}
}

func timestampReconciliationError(result EvidenceReconciliation, selection timestampSelection, artifact TargetArtifact) error {
	if result.State != ReconciliationIncomplete {
		return reconciliationError(result)
	}
	allowed := make(map[string]struct{}, len(selection.Candidates))
	for _, candidate := range selection.Candidates {
		allowed[string(candidate.CABundlePath)] = struct{}{}
		requestPath, responsePath, err := timestampEvidencePathsForArtifact(artifact, candidate.TSAURL)
		if err != nil {
			return err
		}
		allowed[string(requestPath)] = struct{}{}
		allowed[string(responsePath)] = struct{}{}
	}
	for _, issue := range result.Issues {
		if issue.Code != "evidence.unindexed_artifact" {
			return reconciliationError(result)
		}
		if _, ok := allowed[issue.Path]; !ok {
			return reconciliationError(result)
		}
	}
	return nil
}

func buildTimestampEvidenceIndex(existing *publication.EvidenceIndex, artifact TargetArtifact, entry TimestampEntryResult, requestBytes, responseBytes, caBytes []byte) ([]byte, error) {
	if existing == nil || entry.CABundlePath == nil {
		return nil, app.NewError(app.CodeVerification, "timestamp evidence requires an existing retained target index", nil)
	}
	index := *existing
	index.Entries = append([]publication.EvidenceEntry(nil), existing.Entries...)
	trustPath := string(*entry.CABundlePath)
	format := publication.PEMCertificateBundle
	entries := []publication.EvidenceEntry{
		{Role: publication.RoleRFC3161Request, Path: string(entry.RequestPath), Size: int64(len(requestBytes)), Digest: publication.Digest{Algorithm: "sha-256", Value: storage.ResourceDigest(requestBytes)}, TargetRef: &publication.TargetReference{TargetPath: string(artifact.RelativePath)}, Request: &publication.RequestBinding{HashAlgorithm: "sha256", MessageImprintSHA256: artifact.SHA256}},
		{Role: publication.RoleRFC3161Response, Path: string(entry.ResponsePath), Size: int64(len(responseBytes)), Digest: publication.Digest{Algorithm: "sha-256", Value: storage.ResourceDigest(responseBytes)}, Response: &publication.ResponseReferences{TargetPath: string(artifact.RelativePath), RequestPath: string(entry.RequestPath), TrustPath: &trustPath}, TSA: &publication.ResponseBinding{TSAURL: entry.TSAURL}},
		{Role: publication.RoleX509CABundle, Path: trustPath, Size: int64(len(caBytes)), Digest: publication.Digest{Algorithm: "sha-256", Value: storage.ResourceDigest(caBytes)}, Format: &format},
	}
	byPath := make(map[string]publication.EvidenceEntry, len(index.Entries))
	for _, current := range index.Entries {
		byPath[current.Path] = current
	}
	for _, candidate := range entries {
		if current, ok := byPath[candidate.Path]; ok {
			left, _ := json.Marshal(current)
			right, _ := json.Marshal(candidate)
			if !bytes.Equal(left, right) {
				return nil, app.NewError(app.CodeConflict, "evidence index path already has different timestamp metadata", nil)
			}
			continue
		}
		index.Entries = append(index.Entries, candidate)
		byPath[candidate.Path] = candidate
	}
	publication.SortEvidenceEntries(index.Entries)
	encoded, err := publication.EncodeEvidenceIndex(index, false)
	if err != nil {
		return nil, app.NewError(app.CodeInvalidData, "timestamp evidence index is invalid", err)
	}
	return encoded, nil
}

func commitTimestampEvidence(ctx context.Context, path string, questionID, forecastID ledger.Slug, scope TargetScope, headEventID ledger.Slug, artifact TargetArtifact, entry TimestampEntryResult, requestBytes, responseBytes, caBytes []byte, metadata rfc3161.Metadata, verifiedAt ledger.Timestamp, verified, materializeCA bool, result TimestampArtifactResult) (TimestampArtifactResult, error) {
	root := filepath.Dir(path)
	if entry.CABundlePath == nil {
		return result, app.NewError(app.CodeInternal, "timestamp CA bundle path is missing", nil)
	}
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return result, app.NewError(app.CodeConflict, "ledger changed while the timestamp authority request was in flight", err)
	}
	reconciled, err := ReconcileEvidenceStore(ctx, loaded)
	if err != nil {
		return result, err
	}
	selection := timestampSelection{Mode: timestampSelectionNamed, Candidates: []timestampCandidate{{TSAURL: entry.TSAURL, CABundlePath: *entry.CABundlePath}}}
	if !materializeCA {
		selection.Mode = timestampSelectionCustom
	}
	if err := timestampReconciliationError(reconciled, selection, artifact); err != nil {
		return result, err
	}
	indexBytes, err := buildTimestampEvidenceIndex(reconciled.Index, artifact, entry, requestBytes, responseBytes, caBytes)
	if err != nil {
		return result, err
	}
	resolver, err := storage.NewPathResolver(root)
	if err != nil {
		return result, err
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
	indexAbsolute, err := resolver.ResolveLabeled(publication.EvidenceIndexPath, true, "evidence index")
	if err != nil {
		return result, err
	}
	journal := filepath.Join(root, "."+filepath.Base(path)+".timestamp-resources.json")
	resources := make([]storage.ResourceEntry, 0, 5)
	if materializeCA {
		resources = append(resources, resourceEntry(storage.ResourceTimestampTrust, caAbsolute))
	}
	indexResource := resourceEntry(storage.ResourceEvidenceIndex, indexAbsolute)
	indexResource.BeforeSHA256 = storage.ResourceDigest(reconciled.Store.IndexBytes)
	ledgerResource := resourceEntry(storage.ResourceLedger, path)
	ledgerResource.BeforeSHA256 = storage.ResourceDigest(loaded.Document.Raw)
	resources = append(resources,
		resourceEntry(storage.ResourceTimestampRequest, requestAbsolute),
		resourceEntry(storage.ResourceTimestampResponse, responseAbsolute),
		indexResource,
		ledgerResource,
	)
	var plan *storage.ResourcePlan
	indexReplaced := false
	originalIndex := bytes.Clone(reconciled.Store.IndexBytes)
	rollback := func(cause error) (TimestampArtifactResult, error) {
		if plan == nil {
			return result, cause
		}
		if indexReplaced {
			if _, restoreErr := storage.ReplaceDeterministicFile(indexAbsolute, originalIndex, 0o600, publication.MaxEvidenceIndexBytes); restoreErr != nil {
				result.Recovery = Recovery{State: RecoveryRequired, Message: "Timestamp evidence index rollback needs attention.", Paths: []string{filepath.Base(journal)}}
				return result, errors.Join(cause, restoreErr)
			}
		}
		if _, recoverErr := storage.RecoverResourcePlan(context.Background(), journal); recoverErr != nil {
			result.Recovery = Recovery{State: RecoveryRequired, Message: "Timestamp resource rollback needs attention.", Paths: []string{filepath.Base(journal)}}
			return result, errors.Join(cause, recoverErr)
		}
		result.Recovery = Recovery{State: RecoveryNone}
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
		currentSubject, preflightErr := timestampPreflightScoped(model, questionID, forecastID, scope, headEventID)
		if preflightErr != nil || currentSubject.Artifact.SHA256 != artifact.SHA256 || !bytes.Equal(currentSubject.Artifact.Bytes, artifact.Bytes) {
			return nil, app.NewError(app.CodeConflict, "selected target changed while the timestamp authority request was in flight", nil)
		}
		currentCA, caErr := readOptionalBoundedFile(caAbsolute, maxTimestampCABundleBytes)
		if caErr != nil || currentCA != nil && !bytes.Equal(currentCA, caBytes) || !materializeCA && currentCA == nil {
			return nil, app.NewError(app.CodeConflict, "timestamp CA bundle changed while the request was in flight", nil)
		}
		if target := recordedTargetMetadata(model, artifact); target != nil && (target.scope != artifact.Scope || target.canonicalization != TargetCanonicalization || target.path != artifact.RelativePath || target.digest.Value != ledger.Hex32(artifact.SHA256)) {
			return nil, app.NewError(app.CodeConflict, "recorded target metadata changed before timestamp commit", nil)
		}
		currentIndex, indexErr := readBoundedFile(indexAbsolute, publication.MaxEvidenceIndexBytes)
		if indexErr != nil || !bytes.Equal(currentIndex, originalIndex) {
			return nil, app.NewError(app.CodeConflict, "evidence index changed while the timestamp authority request was in flight", indexErr)
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(requestAbsolute), 0o755); mkdirErr != nil {
			return nil, app.NewError(app.CodeIO, "timestamp artifact directory cannot be created", mkdirErr)
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
		writeItems := make([]struct {
			path string
			data []byte
			mode os.FileMode
			max  int64
		}, 0, 3)
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
		for _, item := range writeItems {
			written, writeErr := storage.EnsureDeterministicFile(item.path, item.data, item.mode, item.max)
			if writeErr != nil {
				return nil, writeErr
			}
			if written.State == storage.DeterministicCreated {
				if markErr := plan.MarkCreated(item.path, written.SHA256); markErr != nil {
					return nil, markErr
				}
			}
		}
		indexWrite, writeErr := storage.ReplaceDeterministicFile(indexAbsolute, indexBytes, 0o600, publication.MaxEvidenceIndexBytes)
		if writeErr != nil {
			return nil, writeErr
		}
		indexReplaced = indexWrite.State == storage.DeterministicReplaced
		if indexReplaced {
			if markErr := plan.MarkReplaced(indexAbsolute, indexWrite.SHA256); markErr != nil {
				return nil, markErr
			}
		}
		timestamp := timestampRecord(entry, metadata, verified)
		if artifact.Scope == ForecastEnvelopeSchema {
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
		}
		return patchLifecycleTimestamp(parsed, questionPosition, forecastPosition, forecast, artifact, timestamp, verifiedAt)
	}})
	if err != nil {
		return rollback(err)
	}
	if plan == nil {
		return result, app.NewError(app.CodeInternal, "timestamp resource plan was not committed", nil)
	}
	for _, resource := range resources {
		if err := plan.MarkCommitted(resource.Path); err != nil {
			result.Recovery = Recovery{State: RecoveryRequired, Message: "Timestamp evidence was committed, but journal completion needs attention.", Paths: []string{filepath.Base(journal)}}
			return result, err
		}
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
	result.Effects = []SideEffect{{Kind: EffectEvidenceIndex, Action: EffectReplace, Status: EffectCompleted, Path: publication.EvidenceIndexPath}}
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

func promoteTimestampEntries(ctx context.Context, path string, questionID, forecastID ledger.Slug, scope TargetScope, headEventID ledger.Slug, artifact TargetArtifact, verified map[string]rfc3161.Metadata, verifiedAt ledger.Timestamp, result TimestampArtifactResult) (TimestampArtifactResult, error) {
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
		recorded := recordedTargetMetadata(model, artifact)
		if recorded == nil || recorded.scope != artifact.Scope || recorded.path != artifact.RelativePath || recorded.digest.Value != ledger.Hex32(artifact.SHA256) {
			return nil, app.NewError(app.CodeVerification, "stored target metadata does not match the selected forecast", nil)
		}
		subject, preflightErr := timestampPreflightScoped(model, questionID, forecastID, scope, headEventID)
		if preflightErr != nil {
			return nil, preflightErr
		}
		timestamps := timestampSubjectTimestamps(subject)
		for index := range timestamps {
			metadata, ok := verified[timestampEntryKey(timestamps[index])]
			if !ok {
				continue
			}
			applyVerifiedMetadata(&timestamps[index], metadata)
		}
		if artifact.Scope == ForecastEnvelopeSchema {
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
		}
		return patchLifecyclePromotion(parsed, questionPosition, forecastPosition, forecast, artifact, timestamps, verifiedAt)
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

func lifecycleIntegrityTimestamps(integrity ledger.LifecycleIntegrity) []ledger.RFC3161Timestamp {
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

func timestampSubjectTimestamps(subject timestampSubject) []ledger.RFC3161Timestamp {
	if subject.Artifact.Scope == LifecycleTargetSchema {
		if subject.LifecycleCheckpoint == nil {
			return nil
		}
		return lifecycleIntegrityTimestamps(subject.LifecycleCheckpoint.Integrity)
	}
	return integrityTimestamps(subject.Forecast.Integrity)
}

func timestampSubjectFailed(subject timestampSubject) bool {
	if subject.Artifact.Scope == LifecycleTargetSchema {
		return subject.LifecycleCheckpoint != nil && subject.LifecycleCheckpoint.Integrity.Failed != nil
	}
	return subject.Forecast.Integrity.Failed != nil
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

func lifecycleIntegrityWithTimestamp(current ledger.LifecycleIntegrity, target ledger.LifecycleTarget, timestamp ledger.RFC3161Timestamp, verifiedAt ledger.Timestamp) ledger.LifecycleIntegrity {
	timestamps := lifecycleIntegrityTimestamps(current)
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
		return ledger.LifecycleIntegrity{Verified: &ledger.VerifiedLifecycleIntegrity{Status: ledger.IntegrityVerified, Target: target, Timestamps: timestamps, VerifiedAt: verifiedAt, ExternalAnchors: external}}
	}
	return ledger.LifecycleIntegrity{Pending: &ledger.PendingLifecycleIntegrity{Status: ledger.IntegrityPending, Target: target, Timestamps: timestamps, ExternalAnchors: external}}
}

func patchLifecycleTimestamp(parsed *document.Document, questionPosition, forecastPosition int, forecast ledger.Forecast, artifact TargetArtifact, timestamp ledger.RFC3161Timestamp, recordedAt ledger.Timestamp) ([]byte, error) {
	base := fmt.Sprintf("/questions/%d/forecasts/%d/activity_checkpoints", questionPosition, forecastPosition)
	if forecast.ActivityCheckpoints != nil {
		for index, checkpoint := range *forecast.ActivityCheckpoints {
			if checkpoint.HeadEventID != artifact.HeadEventID {
				continue
			}
			updated := lifecycleIntegrityWithTimestamp(checkpoint.Integrity, LifecycleTargetMetadataFor(artifact), timestamp, recordedAt)
			value, err := jsonPatchValue(updated)
			if err != nil {
				return nil, err
			}
			return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchReplace, Pointer: fmt.Sprintf("%s/%d/integrity", base, index), Value: value}})
		}
	}
	checkpointRecorded := lifecycleCheckpointRecordedAt(forecast, artifact.HeadEventID, recordedAt)
	checkpoint := ledger.ActivityCheckpoint{
		ID: ledger.Slug("checkpoint-" + string(artifact.HeadEventID)), HeadEventID: artifact.HeadEventID, RecordedAt: checkpointRecorded,
		Integrity: lifecycleIntegrityWithTimestamp(ledger.LifecycleIntegrity{}, LifecycleTargetMetadataFor(artifact), timestamp, recordedAt),
	}
	value, err := jsonPatchValue(checkpoint)
	if err != nil {
		return nil, err
	}
	pointer := base
	if forecast.ActivityCheckpoints != nil {
		pointer += "/-"
		return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchAdd, Pointer: pointer, Value: value}})
	}
	return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchAdd, Pointer: pointer, Value: []any{value}}})
}

func patchLifecyclePromotion(parsed *document.Document, questionPosition, forecastPosition int, forecast ledger.Forecast, artifact TargetArtifact, timestamps []ledger.RFC3161Timestamp, verifiedAt ledger.Timestamp) ([]byte, error) {
	if forecast.ActivityCheckpoints == nil {
		return nil, app.NewError(app.CodeVerification, "activity checkpoint is missing", nil)
	}
	for index, checkpoint := range *forecast.ActivityCheckpoints {
		if checkpoint.HeadEventID != artifact.HeadEventID {
			continue
		}
		var external *[]ledger.ExternalAnchor
		if checkpoint.Integrity.Pending != nil {
			external = checkpoint.Integrity.Pending.ExternalAnchors
		} else if checkpoint.Integrity.Verified != nil {
			external = checkpoint.Integrity.Verified.ExternalAnchors
		}
		updated := ledger.LifecycleIntegrity{Verified: &ledger.VerifiedLifecycleIntegrity{Status: ledger.IntegrityVerified, Target: LifecycleTargetMetadataFor(artifact), Timestamps: timestamps, VerifiedAt: verifiedAt, ExternalAnchors: external}}
		value, err := jsonPatchValue(updated)
		if err != nil {
			return nil, err
		}
		pointer := fmt.Sprintf("/questions/%d/forecasts/%d/activity_checkpoints/%d/integrity", questionPosition, forecastPosition, index)
		return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchReplace, Pointer: pointer, Value: value}})
	}
	return nil, app.NewError(app.CodeVerification, "activity checkpoint head is missing", nil)
}

func lifecycleCheckpointRecordedAt(forecast ledger.Forecast, head ledger.Slug, observed ledger.Timestamp) ledger.Timestamp {
	observedTime, observedErr := ledger.ParseTimestamp(observed)
	if forecast.LifecycleEvents == nil {
		return observed
	}
	for _, event := range *forecast.LifecycleEvents {
		if event.ID != head {
			continue
		}
		headTime, headErr := ledger.ParseTimestamp(event.RecordedAt)
		if observedErr != nil || headErr == nil && observedTime.Before(headTime) {
			return event.RecordedAt
		}
		break
	}
	return observed
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
	scope := TargetScopeForecast
	if artifact.Scope == LifecycleTargetSchema {
		scope = TargetScopeLifecycle
	}
	return TimestampArtifactResult{QuestionID: artifact.QuestionID, ForecastID: artifact.ForecastID, HeadEventID: artifact.HeadEventID, Scope: scope, State: TimestampPending, TargetPath: artifact.RelativePath, TargetSHA256: artifact.SHA256, Recovery: Recovery{State: RecoveryNone}}
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
	rollback := storage.ResourceRollbackNone
	if owned {
		rollback = storage.ResourceRollbackRemoveOwned
	}
	return storage.ResourceEntry{Kind: kind, Type: storage.ResourceFile, Path: path, Owned: owned, Rollback: rollback, State: storage.ResourcePlanned}
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
