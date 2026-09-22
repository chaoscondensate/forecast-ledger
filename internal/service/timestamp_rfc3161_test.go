package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/timestamp/rfc3161"
)

func TestRFC3161StampStatusVerifyMultipleTSAAndPublication(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	ca := timestampFixture(t, "root.pem")
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), ca, 0o600); err != nil {
		t.Fatal(err)
	}
	response := timestampFixture(t, "response.tsr")
	request := timestampFixture(t, "request.tsq")
	transport := &countingRoundTripper{response: response}
	httpClient := testTimestampHTTPClient(transport)
	effects := Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}}

	for index, tsaURL := range []string{"https://tsa-one.example.test/stamp", "https://tsa-two.example.test/stamp"} {
		requestPath, _, err := TimestampEvidencePaths("f-election-coalition-001", tsaURL)
		if err != nil {
			t.Fatal(err)
		}
		absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
		if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absoluteRequest, request, 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: tsaURL, CABundlePath: "tsa.pem", Effects: effects, HTTPClient: httpClient})
		if err != nil || result.State != TimestampVerified || len(result.Entries) != 1 || result.Entries[0].CheckState != LayerPass || result.RequestSummary.RequestCount != 1 {
			t.Fatalf("stamp %d = %#v, %v", index, result, err)
		}
	}
	if transport.requests != 2 {
		t.Fatalf("TSA request count = %d", transport.requests)
	}
	retryEffects := Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 1, 0, 0, time.UTC)}, Random: failingRandom{}}
	retry, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: "https://tsa-one.example.test/stamp", CABundlePath: "tsa.pem", Effects: retryEffects, HTTPClient: httpClient})
	if err != nil || retry.State != TimestampVerified || transport.requests != 2 {
		t.Fatalf("idempotent retry = %#v, %v; requests=%d", retry, err, transport.requests)
	}
	loaded, err := LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	forecast := loaded.Model.Questions[1].Forecasts[0]
	if forecast.Integrity.Verified == nil || len(forecast.Integrity.Verified.Timestamps) != 2 {
		t.Fatalf("verified integrity = %#v", forecast.Integrity)
	}
	status, err := TimestampStatusFor(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001")
	if err != nil || status.State != TimestampVerified || len(status.Entries) != 2 {
		t.Fatalf("status = %#v, %v", status, err)
	}
	verified, err := CommitTimestampVerify(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampVerifyOptions{Effects: effects})
	if err != nil || verified.Verification.State != LayerPass || len(verified.Verification.ReasonCodes) != 1 || verified.Verification.ReasonCodes[0] != "timing.rfc3161_verified" || transport.requests != 2 {
		t.Fatalf("local verify = %#v, %v; requests=%d", verified, err, transport.requests)
	}

	for index, event := range []struct {
		kind ledger.LifecycleEventType
		id   ledger.Slug
		at   time.Time
	}{
		{ledger.LifecycleWithdrawn, "event-evidence-withdrawn", time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)},
		{ledger.LifecycleReaffirmed, "event-evidence-reaffirmed", time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
		{ledger.LifecycleExpired, "event-evidence-expired", time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
	} {
		effective := ledger.Timestamp(event.at.Format(time.RFC3339))
		recorded := ledger.Timestamp(event.at.Add(time.Duration(index+1) * time.Second).Format(time.RFC3339))
		if _, err := CommitForecastLifecycleFile(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", event.kind, LifecycleInput{ID: event.id, EffectiveAt: effective}, recorded); err != nil {
			t.Fatalf("append %s after timestamp: %v", event.kind, err)
		}
	}
	report, err := VerifyLedgerEvidence(t.Context(), ledgerPath, VerificationOptions{QuestionID: "q-election-coalition", ForecastID: "f-election-coalition-001", Offline: true})
	if err != nil || report.Overall != VerificationPass || report.Forecasts[0].Layers[0].State != LayerPass || report.Forecasts[0].Layers[1].State != LayerNotApplicable || report.Forecasts[0].Layers[2].State != LayerPass {
		t.Fatalf("lifecycle evidence verification = %#v, %v", report, err)
	}
	loaded, err = LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := ShowForecast(loaded.Model, "q-election-coalition", "f-election-coalition-001")
	if err != nil || view.Summary.Active {
		t.Fatalf("expired forecast activity = %#v, %v", view.Summary, err)
	}

	timestamps := loaded.Model.Questions[1].Forecasts[0].Integrity.Verified.Timestamps
	originalSerials := []string{*timestamps[0].SerialNumber, *timestamps[1].SerialNumber}
	*timestamps[0].SerialNumber = originalSerials[0] + "0"
	writeLedgerModel(t, ledgerPath, loaded.Model)
	report, err = VerifyLedgerEvidence(t.Context(), ledgerPath, VerificationOptions{QuestionID: "q-election-coalition", ForecastID: "f-election-coalition-001", Offline: true})
	if err != nil || report.Overall != VerificationPass || report.Forecasts[0].Layers[2].State != LayerPass {
		t.Fatalf("one-of-two timestamp verification = %#v, %v", report, err)
	}
	*timestamps[1].SerialNumber = originalSerials[1] + "0"
	writeLedgerModel(t, ledgerPath, loaded.Model)
	report, err = VerifyLedgerEvidence(t.Context(), ledgerPath, VerificationOptions{QuestionID: "q-election-coalition", ForecastID: "f-election-coalition-001", Offline: true})
	if err != nil || report.Overall != VerificationFail || report.Forecasts[0].Layers[2].State != LayerFail {
		t.Fatalf("all metadata mismatch verification = %#v, %v", report, err)
	}
	*timestamps[0].SerialNumber, *timestamps[1].SerialNumber = originalSerials[0], originalSerials[1]
	writeLedgerModel(t, ledgerPath, loaded.Model)

	packageRoot := filepath.Join(directory, "package")
	built, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false)
	if err != nil || built.FileCount != 8 {
		t.Fatalf("package build = %#v, %v", built, err)
	}
	packageLedger := filepath.Join(packageRoot, "ledger", filepath.Base(ledgerPath))
	packageResult, err := VerifyPublicationPackage(t.Context(), packageLedger, filepath.Join(packageRoot, "manifest.json"))
	if err != nil || packageResult.Overall != VerificationPass || transport.requests != 2 {
		t.Fatalf("package verify = %#v, %v; requests=%d", packageResult, err, transport.requests)
	}
}

func TestLifecycleTimestampPendingCommitAppendsOneExactHeadCheckpoint(t *testing.T) {
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "ledger.json")
	mutation, err := BuildForecastLifecycle(testPublicInitialLedger(t), "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-withdrawn", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	writeLedgerModel(t, ledgerPath, mutation.Ledger)
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), timestampFixture(t, "root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := BuildLifecycleTarget(mutation.Ledger, "q-one", "f-one", "event-withdrawn")
	if err != nil {
		t.Fatal(err)
	}
	tsaURL := "https://tsa.example.test/stamp"
	requestPath, _, err := timestampEvidencePathsForArtifact(artifact, tsaURL)
	if err != nil {
		t.Fatal(err)
	}
	requestBytes, _, err := rfc3161.CreateRequest(artifact.Bytes, bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)), rfc3161.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	requestAbsolute := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(requestAbsolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestAbsolute, requestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	options := TimestampStampOptions{
		Scope: TargetScopeLifecycle, HeadEventID: "event-withdrawn", TSAURL: tsaURL, CABundlePath: "tsa.pem",
		Effects:    Effects{Clock: fixedTestClock{value: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}},
		HTTPClient: testTimestampHTTPClient(&countingRoundTripper{response: timestampFixture(t, "response.tsr")}),
	}
	result, stampErr := CommitTimestampStamp(t.Context(), ledgerPath, "q-one", "f-one", options)
	if app.ErrorCodeOf(stampErr) != app.CodePending || result.Scope != TargetScopeLifecycle || result.HeadEventID != "event-withdrawn" {
		t.Fatalf("lifecycle pending stamp = %#v, %v", result, stampErr)
	}
	loaded, err := LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := loaded.Model.Questions[0].Forecasts[0].ActivityCheckpoints
	if checkpoints == nil || len(*checkpoints) != 1 || (*checkpoints)[0].HeadEventID != "event-withdrawn" || (*checkpoints)[0].Integrity.Pending == nil {
		t.Fatalf("checkpoint = %#v", checkpoints)
	}
	if !strings.Contains(string((*checkpoints)[0].Integrity.Pending.Timestamps[0].RequestPath), "f-one.lifecycle.event-withdrawn") {
		t.Fatalf("request path = %s", (*checkpoints)[0].Integrity.Pending.Timestamps[0].RequestPath)
	}
	_, retryErr := CommitTimestampStamp(t.Context(), ledgerPath, "q-one", "f-one", options)
	if app.ErrorCodeOf(retryErr) != app.CodePending {
		t.Fatalf("retry error = %v", retryErr)
	}
	loaded, err = LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil || len(*loaded.Model.Questions[0].Forecasts[0].ActivityCheckpoints) != 1 {
		t.Fatalf("retry checkpoint count: %v", err)
	}
	packagePath := filepath.Join(directory, "package")
	publication, err := CommitPublicationBuild(t.Context(), ledgerPath, packagePath, false)
	if err != nil {
		t.Fatal(err)
	}
	wantFragments := []string{
		"proofs/targets/f-one.lifecycle.event-withdrawn.json",
		"proofs/timestamps/f-one.lifecycle.event-withdrawn/",
		"tsa.pem",
	}
	for _, fragment := range wantFragments {
		found := false
		for _, file := range publication.Files {
			if strings.Contains(file.Path, fragment) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("lifecycle publication is missing %q: %#v", fragment, publication.Files)
		}
	}
	targetPath := filepath.Join(directory, "proofs", "targets", "f-one.lifecycle.event-withdrawn.json")
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyLedgerEvidence(t.Context(), ledgerPath, VerificationOptions{QuestionID: "q-one", ForecastID: "f-one", Offline: true})
	if err != nil || report.Overall != VerificationIncomplete || len(report.Forecasts) != 1 || report.Forecasts[0].Layers[1].State != LayerNotChecked || !containsString(report.Forecasts[0].Layers[1].ReasonCodes, "activity.declared_evidence_missing") {
		t.Fatalf("missing lifecycle target verification = %#v, %v", report, err)
	}
}

func TestLifecycleTimestampCancellationCreatesNoEvidence(t *testing.T) {
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "ledger.json")
	mutation, err := BuildForecastLifecycle(testPublicInitialLedger(t), "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-withdrawn", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	writeLedgerModel(t, ledgerPath, mutation.Ledger)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = CommitTimestampStamp(ctx, ledgerPath, "q-one", "f-one", TimestampStampOptions{Scope: TargetScopeLifecycle, HeadEventID: "event-withdrawn"})
	if app.ErrorCodeOf(err) != app.CodeInterrupted {
		t.Fatalf("canceled lifecycle timestamp error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "proofs")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("canceled lifecycle timestamp created artifacts: %v", statErr)
	}
	loaded, err := LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil || loaded.Model.Questions[0].Forecasts[0].ActivityCheckpoints != nil {
		t.Fatalf("canceled lifecycle timestamp changed ledger: %#v, %v", loaded, err)
	}
}

func TestRFC3161DryRunOfflineOutageAndPendingRecovery(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), timestampFixture(t, "root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	spy := &countingRoundTripper{response: timestampFixture(t, "response.tsr")}
	client := testTimestampHTTPClient(spy)
	options := TimestampStampOptions{DryRun: true, TSAURL: "https://tsa.example.test/stamp", CABundlePath: "tsa.pem", Effects: Effects{Clock: fixedTestClock{}, Random: failingRandom{}}, HTTPClient: client}
	plan, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", options)
	if err != nil || spy.requests != 0 || len(plan.Effects) == 0 {
		t.Fatalf("dry run = %#v, %v; requests=%d", plan, err, spy.requests)
	}
	if _, err := os.Stat(filepath.Join(directory, "proofs")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry run created proofs: %v", err)
	}
	options.DryRun, options.Offline = false, true
	if _, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", options); app.ErrorCodeOf(err) != app.CodeNetworkDisabled || spy.requests != 0 {
		t.Fatalf("offline stamp = %v; requests=%d", err, spy.requests)
	}

	requestPath, responsePath, err := TimestampEvidencePaths("f-election-coalition-001", options.TSAURL)
	if err != nil {
		t.Fatal(err)
	}
	absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absoluteRequest, timestampFixture(t, "request.tsq"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.Offline = false
	spy.err = errors.New("simulated outage")
	outage, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", options)
	if app.ErrorCodeOf(err) != app.CodeNetwork || outage.FailureCode != app.CodeNetwork || outage.RequestSummary.RequestCount != 1 || len(outage.Entries) != 1 || outage.Entries[0].CheckState != LayerNotChecked || len(outage.Entries[0].ReasonCodes) != 1 || outage.Entries[0].ReasonCodes[0] != "timing.tsa_unavailable" {
		t.Fatalf("TSA outage = %#v, %v", outage, err)
	}
	if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(string(responsePath)))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("outage retained a nonexistent response: %v", err)
	}

	spy.err = nil
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), timestampFixture(t, "wrong-root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.Effects = Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}}
	pending, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", options)
	if app.ErrorCodeOf(err) != app.CodePending || pending.State != TimestampPending || len(pending.Entries) != 1 || pending.Entries[0].State != ledger.RFC3161Pending {
		t.Fatalf("pending retention = %#v, %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(string(responsePath)))); err != nil {
		t.Fatalf("pending response was not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".ledger.json.timestamp-resources.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("completed resource journal remains: %v", err)
	}
	status, err := TimestampStatusFor(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001")
	if err != nil || status.State != TimestampPending || status.Entries[0].CheckState != LayerFail {
		t.Fatalf("pending local status = %#v, %v", status, err)
	}
	responseBytes, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(string(responsePath))))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, filepath.FromSlash(string(responsePath)))); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyLedgerEvidence(t.Context(), ledgerPath, VerificationOptions{QuestionID: "q-election-coalition", ForecastID: "f-election-coalition-001", Offline: true})
	if err != nil || report.Overall != VerificationPending || report.Forecasts[0].Layers[2].State != LayerPending {
		t.Fatalf("missing retained response = %#v, %v", report, err)
	}
	if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(string(responsePath))), responseBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(directory, "pending-package")
	built, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false)
	if err != nil || built.EvidenceState != "pending" {
		t.Fatalf("pending package build = %#v, %v", built, err)
	}
	packageResult, err := VerifyPublicationPackage(t.Context(), filepath.Join(packageRoot, "ledger", filepath.Base(ledgerPath)), filepath.Join(packageRoot, "manifest.json"))
	if err != nil || packageResult.Overall != VerificationFail {
		t.Fatalf("pending package local trust failure = %#v, %v", packageResult, err)
	}
}

func TestRFC3161AutomaticFreeTSASelectionMaterializesTrustAndReusesEvidence(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	profile, ok := rfc3161.ProviderByID(rfc3161.ProviderFreeTSA)
	if !ok {
		t.Fatal("FreeTSA provider is missing")
	}
	plan, err := PlanTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{DryRun: true})
	if err != nil || len(plan.Entries) != 1 || plan.Entries[0].CABundleSHA256 != profile.BundleSHA256() || plan.Entries[0].ProviderID != rfc3161.ProviderFreeTSA || plan.SelectionMode != timestampSelectionAuto {
		t.Fatalf("automatic dry-run = %#v, %v", plan, err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "trust")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("dry-run created trust resources: %v", statErr)
	}
	requestPath, _, err := TimestampEvidencePaths("f-election-coalition-001", profile.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absoluteRequest, timestampFreeTSAFixture(t, "request.tsq"), 0o600); err != nil {
		t.Fatal(err)
	}
	transport := &countingRoundTripper{response: timestampFreeTSAFixture(t, "response.tsr")}
	effects := Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)}, Random: failingRandom{}}
	result, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{Effects: effects, HTTPClient: testTimestampHTTPClient(transport)})
	if err != nil || result.State != TimestampVerified || result.SelectionMode != timestampSelectionAuto || result.SelectedProvider != rfc3161.ProviderFreeTSA || result.RequestSummary.RequestCount != 1 || len(result.Attempts) != 1 || !result.Attempts[0].Attempted {
		t.Fatalf("automatic stamp = %#v, %v", result, err)
	}
	trustPath := filepath.Join(directory, filepath.FromSlash(profile.TrustPath()))
	trust, err := os.ReadFile(trustPath)
	if err != nil || !bytes.Equal(trust, profile.Bundle()) {
		t.Fatalf("materialized trust = %d bytes, %v", len(trust), err)
	}
	loaded, err := LoadAndValidateLedger(t.Context(), ledgerPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := loaded.Model.Questions[1].Forecasts[0].Integrity.Verified.Timestamps[0]
	if timestamp.CABundlePath == nil || string(*timestamp.CABundlePath) != profile.TrustPath() || timestamp.TSAURL != profile.Endpoint() {
		t.Fatalf("retained timestamp = %#v", timestamp)
	}

	transport.err = errors.New("network must not be used")
	retry, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{Effects: effects, HTTPClient: testTimestampHTTPClient(transport)})
	if err != nil || retry.State != TimestampVerified || retry.RequestSummary.RequestCount != 0 || transport.requests != 1 || retry.Attempts[0].ReasonCode != "timing.existing_evidence_reused" {
		t.Fatalf("automatic reuse = %#v, %v; requests=%d", retry, err, transport.requests)
	}
}

func TestRFC3161AutomaticFailureIsNoOpAndSelectionInputsAreExclusive(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	transport := &countingRoundTripper{err: errors.New("simulated outage")}
	effects := Effects{Clock: fixedTestClock{}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}}
	result, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{Effects: effects, HTTPClient: testTimestampHTTPClient(transport)})
	if app.ErrorCodeOf(err) != app.CodeNetwork || result.State != TimestampUnanchored || result.RequestSummary.RequestCount != 1 || result.Attempts[0].ReasonCode != "timing.tsa_unavailable" {
		t.Fatalf("automatic outage = %#v, %v", result, err)
	}
	for _, name := range []string{"proofs", "trust"} {
		if _, statErr := os.Stat(filepath.Join(directory, name)); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("automatic outage created %s: %v", name, statErr)
		}
	}
	invalidTransport := &countingRoundTripper{response: []byte("not a timestamp response")}
	invalid, invalidErr := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{Effects: effects, HTTPClient: testTimestampHTTPClient(invalidTransport)})
	if app.ErrorCodeOf(invalidErr) != app.CodeVerification || invalid.State != TimestampUnanchored || invalid.Attempts[0].ReasonCode != string(rfc3161.ReasonResponseMalformed) {
		t.Fatalf("automatic invalid response = %#v, %v", invalid, invalidErr)
	}
	for _, name := range []string{"proofs", "trust"} {
		if _, statErr := os.Stat(filepath.Join(directory, name)); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("invalid automatic response created %s: %v", name, statErr)
		}
	}
	if _, err := PlanTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAProvider: rfc3161.ProviderFreeTSA, TSAURL: "https://tsa.example.test", CABundlePath: "tsa.pem"}); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("mixed provider inputs = %v", err)
	}
	if _, err := PlanTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: "http://tsa.example.test", CABundlePath: "tsa.pem"}); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("custom HTTP = %v", err)
	}
}

func TestRFC3161PublicationRejectsMissingAndTamperedArtifacts(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	caPath := filepath.Join(directory, "tsa.pem")
	if err := os.WriteFile(caPath, timestampFixture(t, "root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	tsaURL := "https://tsa.example.test/stamp"
	requestPath, _, err := TimestampEvidencePaths("f-election-coalition-001", tsaURL)
	if err != nil {
		t.Fatal(err)
	}
	absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absoluteRequest, timestampFixture(t, "request.tsq"), 0o600); err != nil {
		t.Fatal(err)
	}
	effects := Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}}
	if _, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: tsaURL, CABundlePath: "tsa.pem", Effects: effects, HTTPClient: testTimestampHTTPClient(&countingRoundTripper{response: timestampFixture(t, "response.tsr")})}); err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(directory, "package")
	if _, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false); err != nil {
		t.Fatal(err)
	}
	packageLedger := filepath.Join(packageRoot, "ledger", filepath.Base(ledgerPath))
	manifestPath := filepath.Join(packageRoot, "manifest.json")

	responseFiles, err := filepath.Glob(filepath.Join(packageRoot, "proofs", "timestamps", "f-election-coalition-001", "*", "response.tsr"))
	if err != nil || len(responseFiles) != 1 {
		t.Fatalf("packaged response files = %v, %v", responseFiles, err)
	}
	responseBytes, err := os.ReadFile(responseFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(responseFiles[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("missing response error = %v", err)
	}
	if err := os.WriteFile(responseFiles[0], responseBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(append([]byte(nil), manifestBytes...), ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("tampered manifest error = %v", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	unexpected := filepath.Join(packageRoot, "unexpected.txt")
	if err := os.WriteFile(unexpected, []byte("not listed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("unexpected file error = %v", err)
	}
	if err := os.Remove(unexpected); err != nil {
		t.Fatal(err)
	}

	symlink := filepath.Join(packageRoot, "unsafe-link")
	if err := os.Symlink(filepath.Base(manifestPath), symlink); err == nil {
		if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
			t.Fatalf("symlink error = %v", err)
		}
		if err := os.Remove(symlink); err != nil {
			t.Fatal(err)
		}
	}

	movedResponse := filepath.Join(filepath.Dir(packageLedger), "response.tsr")
	if err := os.Rename(responseFiles[0], movedResponse); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("moved-under-ledger response error = %v", err)
	}
	if err := os.Rename(movedResponse, responseFiles[0]); err != nil {
		t.Fatal(err)
	}

	tamperedResponse := append([]byte(nil), responseBytes...)
	tamperedResponse[len(tamperedResponse)-1] ^= 0x01
	if err := os.WriteFile(responseFiles[0], tamperedResponse, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("tampered response error = %v", err)
	}
	if err := os.WriteFile(responseFiles[0], responseBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	packageCA := filepath.Join(packageRoot, "tsa.pem")
	if err := os.WriteFile(packageCA, timestampFixture(t, "wrong-root.pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationPackage(t.Context(), packageLedger, manifestPath); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("tampered trust bundle error = %v", err)
	}
}

func TestRFC3161StampRejectsInFlightLedgerChangeAndTargetCollision(t *testing.T) {
	directory, ledgerPath := timestampLedgerFixture(t)
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), timestampFixture(t, "root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	tsaURL := "https://tsa.example.test/stamp"
	requestPath, responsePath, err := TimestampEvidencePaths("f-election-coalition-001", tsaURL)
	if err != nil {
		t.Fatal(err)
	}
	absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
	if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absoluteRequest, timestampFixture(t, "request.tsq"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutated := false
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		loaded, loadErr := LoadAndValidateLedger(request.Context(), ledgerPath, nil)
		if loadErr != nil {
			return nil, loadErr
		}
		note := "changed while the TSA request was in flight"
		loaded.Model.Questions[1].Forecasts[0].PublicNote = &note
		writeLedgerModel(t, ledgerPath, loaded.Model)
		mutated = true
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(timestampFixture(t, "response.tsr"))), Request: request}, nil
	})
	effects := Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}}
	_, err = CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: tsaURL, CABundlePath: "tsa.pem", Effects: effects, HTTPClient: testTimestampHTTPClient(transport)})
	if !mutated || app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("in-flight ledger change = mutated %t, err %v", mutated, err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, filepath.FromSlash(string(responsePath)))); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("conflicted stamp retained response: %v", statErr)
	}

	directory, ledgerPath = timestampLedgerFixture(t)
	if err := os.WriteFile(filepath.Join(directory, "tsa.pem"), timestampFixture(t, "root.pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(directory, "proofs", "targets", "f-election-coalition-001.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("conflicting target"), 0o600); err != nil {
		t.Fatal(err)
	}
	spy := &countingRoundTripper{response: timestampFixture(t, "response.tsr")}
	if _, err := CommitTimestampStamp(t.Context(), ledgerPath, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{TSAURL: tsaURL, CABundlePath: "tsa.pem", Effects: effects, HTTPClient: testTimestampHTTPClient(spy)}); app.ErrorCodeOf(err) != app.CodeConflict || spy.requests != 0 {
		t.Fatalf("target collision = %v; requests=%d", err, spy.requests)
	}
	retained, err := os.ReadFile(targetPath)
	if err != nil || string(retained) != "conflicting target" {
		t.Fatalf("target collision overwrote bytes: %q, %v", retained, err)
	}
}

func TestEmptyPublicationPackageReturnsNoEvidence(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "empty-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(ledgerPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(directory, "package")
	if _, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicationPackage(t.Context(), filepath.Join(packageRoot, "ledger", "ledger.json"), filepath.Join(packageRoot, "manifest.json"))
	if err != nil || result.Overall != VerificationNoEvidence || result.FailureCode != app.CodeIncomplete || len(result.Evidence) != 0 {
		t.Fatalf("empty package verification = %#v, %v", result, err)
	}
}

func TestPublicationMatrixSealedRevealedInactiveAndMultiRepresentation(t *testing.T) {
	for _, reveal := range []bool{false, true} {
		name := "sealed"
		if reveal {
			name = "revealed"
		}
		t.Run(name+"-failed-evidence", func(t *testing.T) {
			build := testSealedInitialBuild(t)
			model := build.Ledger
			if reveal {
				revealed, err := BuildForecastReveal(model, "q-one", "f-one", build.KeyFile, "2026-02-01T00:00:00Z")
				if err != nil {
					t.Fatal(err)
				}
				model = revealed.Ledger
			}
			artifact, err := BuildForecastTarget(model, "q-one", "f-one")
			if err != nil {
				t.Fatal(err)
			}
			target := TargetMetadataFor(artifact)
			model.Questions[0].Forecasts[0].Integrity = ledger.Integrity{Failed: &ledger.FailedIntegrity{
				Status: ledger.IntegrityFailed, FailureReason: "timestamp verification failed", Target: &target,
			}}
			directory := t.TempDir()
			ledgerPath := filepath.Join(directory, "ledger.json")
			writeLedgerModel(t, ledgerPath, model)
			targetPath := filepath.Join(directory, filepath.FromSlash(string(artifact.RelativePath)))
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetPath, artifact.Bytes, 0o644); err != nil {
				t.Fatal(err)
			}
			result := buildAndVerifyPublication(t, ledgerPath)
			if result.Overall != VerificationFail || result.FailureCode != app.CodeVerification {
				t.Fatalf("%s package verification = %#v", name, result)
			}
		})
	}

	for _, test := range []struct {
		name        string
		model       func(*testing.T) *ledger.Ledger
		wantOverall VerificationOverall
		wantCode    app.ErrorCode
	}{
		{name: "inactive", wantOverall: VerificationNoEvidence, wantCode: app.CodeIncomplete, model: func(t *testing.T) *ledger.Ledger {
			mutation, err := BuildForecastLifecycle(testPublicInitialLedger(t), "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-one", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:01:00Z")
			if err != nil {
				t.Fatal(err)
			}
			return mutation.Ledger
		}},
		{name: "multi-representation", wantOverall: VerificationPass, model: func(t *testing.T) *ledger.Ledger {
			_, model := rootUpdateFixture(t, "individual-ledger.json")
			if representations := model.Questions[2].Forecasts[0].Representations; representations == nil || len(*representations) < 2 {
				t.Fatal("fixture is not multi-representation")
			}
			return model
		}},
	} {
		t.Run(test.name+"-offline-verify", func(t *testing.T) {
			directory := t.TempDir()
			ledgerPath := filepath.Join(directory, "ledger.json")
			writeLedgerModel(t, ledgerPath, test.model(t))
			result := buildAndVerifyPublication(t, ledgerPath)
			if result.Overall != test.wantOverall || result.FailureCode != test.wantCode {
				t.Fatalf("%s package verification = overall %q code %q", test.name, result.Overall, result.FailureCode)
			}
		})
	}
}

func buildAndVerifyPublication(t *testing.T, ledgerPath string) PublicationVerifyResult {
	t.Helper()
	packageRoot := filepath.Join(filepath.Dir(ledgerPath), "package")
	if _, err := CommitPublicationBuild(t.Context(), ledgerPath, packageRoot, false); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicationPackage(t.Context(), filepath.Join(packageRoot, "ledger", filepath.Base(ledgerPath)), filepath.Join(packageRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func timestampLedgerFixture(t *testing.T) (string, string) {
	t.Helper()
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory, path
}

func writeLedgerModel(t *testing.T, path string, model *ledger.Ledger) {
	t.Helper()
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func timestampFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "timestamp", "rfc3161", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func timestampFreeTSAFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "timestamp", "rfc3161", "testdata", "freetsa", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type countingRoundTripper struct {
	response []byte
	err      error
	requests int
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (transport *countingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	if transport.err != nil {
		return nil, transport.err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(transport.response)), Request: request}, nil
}

type publicResolver struct{}

func (publicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func testTimestampHTTPClient(transport http.RoundTripper) *rfc3161.HTTPClient {
	return &rfc3161.HTTPClient{Resolver: publicResolver{}, Client: &http.Client{Transport: transport}}
}

type failingRandom struct{}

func (failingRandom) ReadFull(context.Context, []byte) error {
	return errors.New("entropy must not be used")
}
