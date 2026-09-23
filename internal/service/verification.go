package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/forecastcrypto"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/netpolicy"
)

type LayerState string

const (
	LayerPass          LayerState = "pass"
	LayerFail          LayerState = "fail"
	LayerPending       LayerState = "pending"
	LayerNotApplicable LayerState = "not_applicable"
	LayerNotChecked    LayerState = "not_checked"
)

type VerificationLayer struct {
	Name        string         `json:"name"`
	State       LayerState     `json:"state"`
	ReasonCodes []string       `json:"reason_codes,omitempty"`
	Evidence    map[string]any `json:"evidence,omitempty"`
	Limitations []string       `json:"limitations,omitempty"`
}

type ForecastVerification struct {
	QuestionID ledger.Slug         `json:"question_id"`
	ForecastID ledger.Slug         `json:"forecast_id"`
	Layers     []VerificationLayer `json:"layers"`
}

type VerificationOverall string

const (
	VerificationPass       VerificationOverall = "pass"
	VerificationFail       VerificationOverall = "fail"
	VerificationPending    VerificationOverall = "pending"
	VerificationIncomplete VerificationOverall = "incomplete"
	VerificationNoEvidence VerificationOverall = "no_evidence"
)

type VerificationReport struct {
	LedgerID       ledger.Slug            `json:"ledger_id"`
	Overall        VerificationOverall    `json:"overall"`
	Document       VerificationLayer      `json:"document"`
	Reconciliation VerificationLayer      `json:"reconciliation"`
	Forecasts      []ForecastVerification `json:"forecasts"`
	Limitations    []string               `json:"limitations"`
	FailureCode    app.ErrorCode          `json:"-"`
}

type VerificationOptions struct {
	Offline      bool
	CheckSources bool
	HTTPClient   *http.Client
	QuestionID   ledger.Slug
	ForecastID   ledger.Slug
}

var verificationLimitations = []string{
	"Forecast Ledger v2 does not prove authorship.",
	"It does not prove that the ledger or forecast set is complete.",
	"It does not prove forecast truth or calibration.",
	"Forecast and outcome times are self-reported; verified RFC 3161 evidence supplies a signed generation time for the exact target.",
	"Outcome-source checks do not establish authority or substantive truth.",
	"Filesystem, archive, hosting, source-control, and external-anchor times are not cryptographic existence evidence.",
	"A valid timestamp signature and certificate chain do not prove that the timestamp authority clock was honest.",
	"The retained CA bundle does not establish current revocation status or long-term legal validity.",
}

func VerifyLedgerEvidence(ctx context.Context, path string, options VerificationOptions) (VerificationReport, error) {
	loaded, err := loadAndValidateLedgerForEvidence(ctx, path)
	if err != nil {
		return VerificationReport{}, err
	}
	report := VerificationReport{
		LedgerID:    loaded.Model.LedgerID,
		Document:    VerificationLayer{Name: "document", State: LayerPass, ReasonCodes: []string{"document.valid"}, Evidence: map[string]any{"schema_version": loaded.Model.SchemaVersion, "format": loaded.Document.Format}},
		Forecasts:   []ForecastVerification{},
		Limitations: append([]string(nil), verificationLimitations...),
	}
	reconciled, err := ReconcileEvidenceStore(ctx, loaded)
	if err != nil {
		return report, err
	}
	report.Reconciliation = reconciliationLayer(reconciled)
	selectedQuestions, err := selectVerificationQuestions(loaded.Model, options.QuestionID, options.ForecastID)
	if err != nil {
		return report, err
	}
	for _, question := range selectedQuestions {
		for _, forecast := range question.Forecasts {
			if options.ForecastID != "" && forecast.ID != options.ForecastID {
				continue
			}
			item := ForecastVerification{QuestionID: question.ID, ForecastID: forecast.ID}
			content := verifyContentLayer(ctx, loaded, question, forecast)
			item.Layers = append(item.Layers, content)
			item.Layers = append(item.Layers, verifyActivityLayer(ctx, loaded, question, forecast, reconciled))
			item.Layers = append(item.Layers, verifyTimingLayer(ctx, loaded, question, forecast, content))
			item.Layers = append(item.Layers, verifyRevealLayer(question, forecast, content))
			item.Layers = append(item.Layers, verifyOutcomeLayer(ctx, question, options))
			report.Forecasts = append(report.Forecasts, item)
		}
	}
	if ctx != nil && ctx.Err() != nil {
		return report, app.NewError(app.CodeInterrupted, "verification was interrupted", ctx.Err())
	}
	report.Overall, report.FailureCode = aggregateVerification(report)
	return report, nil
}

func verifyActivityLayer(ctx context.Context, loaded *LoadedLedger, question ledger.Question, forecast ledger.Forecast, reconciled EvidenceReconciliation) VerificationLayer {
	activity := deriveActivity(forecast)
	evidence := activityEvidence(activity)
	layer := VerificationLayer{Name: "activity", Evidence: evidence, Limitations: []string{"Current activity is derived from the retained event stream; absence of all independent observations cannot prove that an earlier event never existed."}}
	if forecast.ActivityCheckpoints == nil || len(*forecast.ActivityCheckpoints) == 0 {
		if detachedLifecycleEvidenceFor(reconciled, question.ID, forecast.ID) {
			layer.State, layer.ReasonCodes = LayerFail, []string{"activity.retained_evidence_unreferenced"}
			return layer
		}
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"activity.unbound"}
		return layer
	}
	root := filepath.Dir(loaded.Path)
	hasVerified, hasPending, hasNotChecked := false, false, false
	checkpointResults := make([]map[string]any, 0, len(*forecast.ActivityCheckpoints))
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		artifact, err := BuildLifecycleTarget(loaded.Model, question.ID, forecast.ID, checkpoint.HeadEventID)
		if err != nil {
			return failedLayerWithEvidence(layer.Name, "activity.target_build_failed", evidence)
		}
		declaredTarget := lifecycleIntegrityTarget(checkpoint.Integrity)
		if declaredTarget == nil || declaredTarget.Scope != artifact.Scope || declaredTarget.Canonicalization != TargetCanonicalization || declaredTarget.Digest != (ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(artifact.SHA256)}) {
			return failedLayerWithEvidence(layer.Name, "activity.target_metadata_mismatch", evidence)
		}
		row := map[string]any{"checkpoint_id": checkpoint.ID, "head_event_id": checkpoint.HeadEventID, "target_path": declaredTarget.ArtifactPath, "target_sha256": artifact.SHA256}
		targetBytes, err := readConfinedArtifact(root, string(declaredTarget.ArtifactPath), maxTargetBytes)
		if err != nil {
			row["state"] = ActivityNotChecked
			checkpointResults = append(checkpointResults, row)
			hasNotChecked = true
			continue
		}
		if !bytes.Equal(targetBytes, artifact.Bytes) {
			evidence["checkpoints"] = append(checkpointResults, row)
			return failedLayerWithEvidence(layer.Name, "activity.target_mismatch", evidence)
		}
		var timestamps []ledger.RFC3161Timestamp
		integrityState := ledger.IntegrityPending
		switch {
		case checkpoint.Integrity.Pending != nil:
			timestamps = checkpoint.Integrity.Pending.Timestamps
		case checkpoint.Integrity.Verified != nil:
			integrityState = ledger.IntegrityVerified
			timestamps = checkpoint.Integrity.Verified.Timestamps
		case checkpoint.Integrity.Failed != nil:
			evidence["checkpoints"] = checkpointResults
			return failedLayerWithEvidence(layer.Name, "activity.imported_failed", evidence)
		}
		results := inspectTimestampEntries(ctx, root, artifact.Bytes, timestamps)
		row["timestamps"] = results
		row["state"] = integrityState
		checkpointResults = append(checkpointResults, row)
		verifiedHere, pendingHere := false, len(results) == 0
		for _, result := range results {
			if result.CheckState == LayerFail {
				evidence["checkpoints"] = checkpointResults
				return failedLayerWithEvidence(layer.Name, "activity.timestamp_mismatch", evidence)
			}
			if result.CheckState == LayerPass && result.State == ledger.RFC3161Verified {
				verifiedHere = true
			}
			if result.CheckState == LayerPending || result.CheckState == LayerNotChecked || result.State == ledger.RFC3161Pending {
				pendingHere = true
			}
		}
		hasVerified = hasVerified || verifiedHere
		hasPending = hasPending || pendingHere
	}
	evidence["checkpoints"] = checkpointResults
	layer.State, layer.ReasonCodes = classifyActivityEvidence(activity, hasVerified, hasPending, hasNotChecked)
	return layer
}

func activityEvidence(activity ActivityResult) map[string]any {
	evidence := map[string]any{
		"active": activity.Active, "event_count": activity.EventCount, "coverage": activity.Coverage,
	}
	if activity.LastEventID != "" {
		evidence["last_event_id"], evidence["last_event_type"] = activity.LastEventID, activity.LastEventType
		evidence["last_effective_at"], evidence["last_recorded_at"] = activity.LastEffectiveAt, activity.LastRecordedAt
	}
	if activity.CoveredHead != "" {
		evidence["covered_head"] = activity.CoveredHead
	}
	return evidence
}

func classifyActivityEvidence(activity ActivityResult, hasVerified, hasPending, hasNotChecked bool) (LayerState, []string) {
	if hasNotChecked {
		return LayerNotChecked, []string{"activity.declared_evidence_missing"}
	}
	if activity.Coverage == ActivityPartial && hasVerified {
		return LayerPass, []string{"activity.prefix_verified_current_head_uncovered"}
	}
	if activity.Coverage == ActivityPending || hasPending && !hasVerified {
		return LayerPending, []string{"activity.evidence_pending"}
	}
	if hasVerified {
		return LayerPass, []string{"activity.current_head_verified"}
	}
	return LayerNotChecked, []string{"activity.evidence_not_checked"}
}

func selectVerificationQuestions(model *ledger.Ledger, questionID, forecastID ledger.Slug) ([]ledger.Question, error) {
	if forecastID != "" && questionID == "" {
		return nil, app.NewError(app.CodeUsage, "--forecast requires --question", nil)
	}
	if questionID == "" {
		return append([]ledger.Question(nil), model.Questions...), nil
	}
	_, question, err := selectQuestion(model, questionID)
	if err != nil {
		return nil, err
	}
	if forecastID != "" {
		found := false
		for _, forecast := range question.Forecasts {
			if forecast.ID == forecastID {
				found = true
				break
			}
		}
		if !found {
			return nil, app.NewError(app.CodeNotFound, "forecast was not found in the selected question", nil)
		}
	}
	return []ledger.Question{question}, nil
}

func verifyContentLayer(ctx context.Context, loaded *LoadedLedger, question ledger.Question, forecast ledger.Forecast) VerificationLayer {
	layer := VerificationLayer{Name: "content_binding"}
	artifact, err := BuildForecastTarget(loaded.Model, question.ID, forecast.ID)
	if err != nil {
		return failedLayer(layer.Name, "content.target_build_failed", err)
	}
	root := filepath.Dir(loaded.Path)
	hasTarget := regularFileExists(filepath.Join(root, filepath.FromSlash(string(artifact.RelativePath)))) || recordedForecastTarget(loaded.Model, question.ID, forecast.ID) != nil
	if !hasTarget {
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"content.no_retained_target"}
		return layer
	}
	if _, err := CheckTargets(ctx, loaded.Path, false, question.ID, forecast.ID); err != nil {
		return failedLayer(layer.Name, "content.target_mismatch", err)
	}
	layer.State = LayerPass
	layer.ReasonCodes = []string{"content.target_matches"}
	layer.Evidence = map[string]any{"path": artifact.RelativePath, "sha256": artifact.SHA256, "scope": ForecastEnvelopeSchema, "canonicalization": TargetCanonicalization}
	return layer
}

func verifyTimingLayer(ctx context.Context, loaded *LoadedLedger, question ledger.Question, forecast ledger.Forecast, content VerificationLayer) VerificationLayer {
	layer := VerificationLayer{Name: "existence_timing", Limitations: timestampLimitations()}
	if forecast.Integrity.Unanchored != nil {
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"timing.unanchored"}
		return layer
	}
	if forecast.Integrity.Failed != nil {
		return failedLayer(layer.Name, "timing.imported_failed", nil)
	}
	if content.State != LayerPass {
		layer.State, layer.ReasonCodes = LayerNotChecked, []string{"timing.blocked_by_content"}
		return layer
	}
	artifact, _ := BuildForecastTarget(loaded.Model, question.ID, forecast.ID)
	entries := integrityTimestamps(forecast.Integrity)
	results := inspectTimestampEntries(ctx, filepath.Dir(loaded.Path), artifact.Bytes, entries)
	hasPending, hasLate := false, false
	evidence := map[string]any{"target_path": artifact.RelativePath, "target_binding": "pass", "timestamps": results}
	if forecast.Integrity.Verified != nil {
		evidence["verified_at"] = forecast.Integrity.Verified.VerifiedAt
	}
	for _, item := range results {
		if item.CheckState != LayerPass {
			hasPending = hasPending || item.CheckState == LayerPending || item.CheckState == LayerNotChecked
			continue
		}
		if question.Resolution != nil && question.Resolution.Resolved != nil && item.GenTime != nil {
			known, _ := ParseTimestamp(question.Resolution.Resolved.OutcomeKnownAt, "outcome_known_at")
			generated, parseErr := ParseTimestamp(*item.GenTime, "gen_time")
			if parseErr != nil || !generated.Before(known) {
				hasLate = true
				continue
			}
		}
		layer.State, layer.ReasonCodes, layer.Evidence = LayerPass, []string{"timing.rfc3161_verified"}, evidence
		return layer
	}
	if hasPending {
		layer.State, layer.ReasonCodes, layer.Evidence = LayerPending, []string{"timing.local_evidence_incomplete"}, map[string]any{"timestamps": results}
		return layer
	}
	if hasLate {
		return failedLayerWithEvidence(layer.Name, "timing.not_before_outcome", evidence)
	}
	return VerificationLayer{Name: layer.Name, State: LayerFail, ReasonCodes: []string{"timing.all_responses_failed"}, Evidence: map[string]any{"timestamps": results}, Limitations: layer.Limitations}
}

func verifyRevealLayer(question ledger.Question, forecast ledger.Forecast, content VerificationLayer) VerificationLayer {
	layer := VerificationLayer{Name: "reveal"}
	if forecast.Visibility != ledger.VisibilityRevealed {
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"reveal.not_revealed"}
		return layer
	}
	if content.State != LayerPass {
		layer.State, layer.ReasonCodes = LayerNotChecked, []string{"reveal.blocked_by_content"}
		return layer
	}
	if forecast.Commitment == nil || forecast.Commitment.Revealed == nil {
		return failedLayer(layer.Name, "reveal.commitment_missing", nil)
	}
	revealed := forecast.Commitment.Revealed
	key, err := hex.DecodeString(string(revealed.RevealedKey))
	if err != nil {
		return failedLayer(layer.Name, "reveal.key_invalid", err)
	}
	keyFile, err := forecastcrypto.EncodeKeyFile(question.ID, forecast.QuestionRevisionID, forecast.ID, revealed.CommitmentHash.Value, key)
	clear(key)
	if err != nil {
		return failedLayer(layer.Name, "reveal.key_invalid", err)
	}
	opened, err := forecastcrypto.Open(keyFile, question.ID, forecast.QuestionRevisionID, forecast.ID, ledger.SealedCommitment{Scheme: revealed.Scheme, CommitmentHash: revealed.CommitmentHash, Encryption: revealed.Encryption, KeyHint: revealed.KeyHint})
	clear(keyFile)
	if err != nil {
		reason := string(forecastcrypto.FailureStageOf(err))
		if reason == "" {
			reason = "reveal.commitment_malformed"
		}
		return failedLayer(layer.Name, reason, nil)
	}
	if err := validateRevealedBundle(question, forecast, opened.Bundle); err != nil {
		return failedLayer(layer.Name, "reveal.mirror_mismatch", err)
	}
	layer.State, layer.ReasonCodes = LayerPass, []string{"reveal.authenticated"}
	layer.Evidence = map[string]any{"scheme": forecast.Commitment.Revealed.Scheme, "commitment_sha256": forecast.Commitment.Revealed.CommitmentHash.Value}
	return layer
}

func verifyOutcomeLayer(ctx context.Context, question ledger.Question, options VerificationOptions) VerificationLayer {
	layer := VerificationLayer{Name: "outcome_evidence", Limitations: []string{"Source availability and digest checks do not establish authority or substantive truth."}}
	if question.Resolution == nil {
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"outcome.unresolved"}
		return layer
	}
	var sources []ledger.ResolutionSource
	if question.Resolution.Resolved != nil {
		sources = question.Resolution.Resolved.Sources
	} else if question.Resolution.Unresolved != nil && question.Resolution.Unresolved.Sources != nil {
		sources = *question.Resolution.Unresolved.Sources
	}
	layer.State, layer.ReasonCodes = LayerPass, []string{"outcome.metadata_valid"}
	layer.Evidence = map[string]any{"source_count": len(sources), "status": question.Status}
	if !options.CheckSources {
		return layer
	}
	if options.Offline {
		layer.State, layer.ReasonCodes = LayerNotChecked, []string{"outcome.offline"}
		return layer
	}
	client := options.HTTPClient
	if client == nil {
		client = boundedOutcomeClient()
	}
	for index, source := range sources {
		data, finalURL, err := fetchOutcomeSource(ctx, client, source.URL)
		if err != nil {
			layer.State, layer.ReasonCodes = LayerNotChecked, []string{"outcome.source_unavailable"}
			layer.Evidence = map[string]any{"source_index": index}
			return layer
		}
		if source.ContentDigest != nil {
			digest := sha256.Sum256(data)
			if source.ContentDigest.Algorithm != "sha-256" || !strings.EqualFold(hex.EncodeToString(digest[:]), string(source.ContentDigest.Value)) {
				return failedLayer(layer.Name, "outcome.digest_mismatch", nil)
			}
		}
		layer.Evidence[fmt.Sprintf("source_%d_final_url", index)] = finalURL
	}
	layer.ReasonCodes = []string{"outcome.sources_reachable"}
	return layer
}

func aggregateVerification(report VerificationReport) (VerificationOverall, app.ErrorCode) {
	hasPending, hasNotChecked, sourceUnavailable, applicable := false, false, false, 0
	if report.Reconciliation.State != "" && report.Reconciliation.State != LayerNotApplicable {
		applicable++
		switch report.Reconciliation.State {
		case LayerFail:
			return VerificationFail, app.CodeVerification
		case LayerPending:
			hasPending = true
		case LayerNotChecked:
			hasNotChecked = true
		}
	}
	for _, forecast := range report.Forecasts {
		for _, layer := range forecast.Layers {
			if layer.State != LayerNotApplicable {
				applicable++
			}
			if layer.State == LayerFail {
				return VerificationFail, app.CodeVerification
			}
			if layer.State == LayerPending {
				hasPending = true
			}
			if layer.State == LayerNotChecked {
				hasNotChecked = true
				for _, reason := range layer.ReasonCodes {
					if strings.Contains(reason, "source_unavailable") {
						sourceUnavailable = true
					}
				}
			}
		}
	}
	if hasNotChecked {
		if sourceUnavailable {
			return VerificationIncomplete, app.CodeNetwork
		}
		return VerificationIncomplete, app.CodeIncomplete
	}
	if hasPending {
		return VerificationPending, app.CodePending
	}
	if applicable == 0 {
		return VerificationNoEvidence, app.CodeIncomplete
	}
	return VerificationPass, ""
}

func reconciliationLayer(result EvidenceReconciliation) VerificationLayer {
	layer := VerificationLayer{Name: "evidence_reconciliation", Evidence: map[string]any{"index_present": result.IndexPresent, "declarations": result.Declarations, "indexed": result.Indexed, "files": result.Files}}
	codes := make([]string, 0, len(result.Issues))
	for _, issue := range result.Issues {
		codes = append(codes, issue.Code)
	}
	switch result.State {
	case ReconciliationPass:
		layer.State, layer.ReasonCodes = LayerPass, []string{"evidence.reconciled"}
	case ReconciliationNoEvidence:
		layer.State, layer.ReasonCodes = LayerNotApplicable, []string{"evidence.no_evidence"}
	case ReconciliationIncomplete:
		layer.State, layer.ReasonCodes = LayerNotChecked, codes
	case ReconciliationFail:
		layer.State, layer.ReasonCodes = LayerFail, codes
	default:
		layer.State, layer.ReasonCodes = LayerFail, []string{"evidence.reconciliation_invalid"}
	}
	return layer
}

func detachedLifecycleEvidenceFor(result EvidenceReconciliation, questionID, forecastID ledger.Slug) bool {
	if result.Index == nil {
		return false
	}
	issues := make(map[string]struct{})
	for _, issue := range result.Issues {
		if issue.Code == "activity.retained_evidence_unreferenced" {
			issues[issue.Path] = struct{}{}
		}
	}
	for _, entry := range result.Index.Entries {
		if _, ok := issues[entry.Path]; ok && entry.Lifecycle != nil && entry.Lifecycle.QuestionID == string(questionID) && entry.Lifecycle.ForecastID == string(forecastID) {
			return true
		}
	}
	return false
}

func failedLayer(name, reason string, err error) VerificationLayer {
	layer := VerificationLayer{Name: name, State: LayerFail, ReasonCodes: []string{reason}}
	if err != nil {
		layer.Evidence = map[string]any{"error": safeVerificationError(err)}
	}
	return layer
}

func failedLayerWithEvidence(name, reason string, evidence map[string]any) VerificationLayer {
	return VerificationLayer{Name: name, State: LayerFail, ReasonCodes: []string{reason}, Evidence: evidence}
}

func safeVerificationError(err error) string {
	if err == nil {
		return ""
	}
	return strings.SplitN(err.Error(), ":", 2)[0]
}

func boundedOutcomeClient() *http.Client {
	return boundedOutcomeClientWith(net.DefaultResolver, &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second})
}

type outcomeResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type outcomeDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

func boundedOutcomeClientWith(resolver outcomeResolver, dialer outcomeDialer) *http.Client {
	transport := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 20 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("outcome source address is invalid")
		}
		addresses, err := resolver.LookupIPAddr(ctx, host)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil || len(addresses) == 0 {
			return nil, fmt.Errorf("outcome source host could not be resolved")
		}
		for _, candidate := range addresses {
			if !netpolicy.PublicIP(candidate.IP) {
				return nil, fmt.Errorf("outcome source host is not public")
			}
		}
		var lastErr error
		for _, candidate := range addresses {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if err := validatePublicSourceURL(request.URL); err != nil {
			return err
		}
		if len(via) == 0 || normalizedSourceOrigin(request.URL) != normalizedSourceOrigin(via[0].URL) {
			return fmt.Errorf("outcome source redirect is outside the original origin")
		}
		return nil
	}}
}

func fetchOutcomeSource(ctx context.Context, client *http.Client, source string) ([]byte, string, error) {
	parsed, err := url.Parse(source)
	if err != nil || validatePublicSourceURL(parsed) != nil {
		return nil, "", fmt.Errorf("outcome source URL is not a safe public HTTPS URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("User-Agent", "forecast-ledger/outcome-check-v1")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("outcome source returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, "", fmt.Errorf("outcome source response is invalid or too large")
	}
	return data, response.Request.URL.String(), nil
}

func validatePublicSourceURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("source URL must be public HTTPS")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("source URL must include a host")
	}
	return nil
}

func normalizedSourceOrigin(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	return "https://" + net.JoinHostPort(strings.ToLower(parsed.Hostname()), port)
}
