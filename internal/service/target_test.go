package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	targetbytes "github.com/chaoscondensate/forecast-ledger/internal/target"
)

func TestForecastTargetMatchesPublishedV220LifecycleVectors(t *testing.T) {
	for _, name := range []string{
		"forecast-envelope-v3-public-lifecycle.json",
		"forecast-envelope-v3-sealed-lifecycle.json",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/"+name)
			if err != nil {
				t.Fatal(err)
			}
			var vector struct {
				QuestionID       ledger.Slug             `json:"question_id"`
				QuestionRevision ledger.QuestionRevision `json:"question_revision"`
				Forecast         ledger.Forecast         `json:"forecast"`
				Expected         struct {
					CanonicalEnvelope string `json:"canonical_envelope"`
					SHA256            string `json:"sha256"`
				} `json:"expected"`
			}
			if err := json.Unmarshal(data, &vector); err != nil {
				t.Fatal(err)
			}
			forecasts := []ledger.Forecast{}
			if vector.Forecast.SupersedesForecastID != nil {
				forecasts = append(forecasts, ledger.Forecast{ID: *vector.Forecast.SupersedesForecastID, QuestionRevisionID: vector.QuestionRevision.ID})
			}
			forecasts = append(forecasts, vector.Forecast)
			model := &ledger.Ledger{Questions: []ledger.Question{{
				ID: vector.QuestionID, CurrentRevisionID: vector.QuestionRevision.ID,
				Revisions: []ledger.QuestionRevision{vector.QuestionRevision}, Forecasts: forecasts,
			}}}
			withLifecycle, err := BuildForecastTarget(model, vector.QuestionID, vector.Forecast.ID)
			if err != nil {
				t.Fatal(err)
			}
			if string(withLifecycle.Bytes) != vector.Expected.CanonicalEnvelope || withLifecycle.SHA256 != vector.Expected.SHA256 {
				t.Fatalf("published target mismatch\nbytes: %s\ndigest: %s", withLifecycle.Bytes, withLifecycle.SHA256)
			}
			model.Questions[0].Forecasts[len(model.Questions[0].Forecasts)-1].LifecycleEvents = nil
			withoutLifecycle, err := BuildForecastTarget(model, vector.QuestionID, vector.Forecast.ID)
			if err != nil || !bytes.Equal(withLifecycle.Bytes, withoutLifecycle.Bytes) || withLifecycle.SHA256 != withoutLifecycle.SHA256 {
				t.Fatalf("lifecycle activity changed target: %v", err)
			}
		})
	}
}

func TestLifecycleTargetMatchesPublishedV220Vectors(t *testing.T) {
	data, err := fs.ReadFile(contractschema.Conformance(), "tests/vectors/forecast-lifecycle-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		QuestionID       ledger.Slug             `json:"question_id"`
		QuestionRevision ledger.QuestionRevision `json:"question_revision"`
		Forecast         ledger.Forecast         `json:"forecast"`
		Checkpoints      []struct {
			Name        string      `json:"name"`
			HeadEventID ledger.Slug `json:"head_event_id"`
			Expected    struct {
				CanonicalTarget string `json:"canonical_target"`
				SHA256          string `json:"sha256"`
			} `json:"expected"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	model := &ledger.Ledger{Questions: []ledger.Question{{ID: vector.QuestionID, CurrentRevisionID: vector.QuestionRevision.ID, Revisions: []ledger.QuestionRevision{vector.QuestionRevision}, Forecasts: []ledger.Forecast{vector.Forecast}}}}
	for _, checkpoint := range vector.Checkpoints {
		t.Run(checkpoint.Name, func(t *testing.T) {
			artifact, err := BuildLifecycleTarget(model, vector.QuestionID, vector.Forecast.ID, checkpoint.HeadEventID)
			if err != nil {
				t.Fatal(err)
			}
			if string(artifact.Bytes) != checkpoint.Expected.CanonicalTarget || artifact.SHA256 != checkpoint.Expected.SHA256 {
				t.Fatalf("lifecycle target differs: sha=%s bytes=%s", artifact.SHA256, artifact.Bytes)
			}
		})
	}
}

func TestLifecycleTargetBuildAndCheckUseHeadSpecificNonOverwritingPaths(t *testing.T) {
	model := testPublicInitialLedger(t)
	first, err := BuildForecastLifecycle(model, "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-withdrawn", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildForecastLifecycle(first.Ledger, "q-one", "f-one", ledger.LifecycleReaffirmed, LifecycleInput{ID: "event-reaffirmed", EffectiveAt: "2026-02-02T00:00:00Z"}, "2026-02-02T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	writeLedgerModel(t, path, second.Ledger)
	for _, head := range []ledger.Slug{"event-withdrawn", "event-reaffirmed"} {
		result, err := CommitTargetBuildScoped(t.Context(), path, TargetScopeLifecycle, false, "q-one", "f-one", head, ledger.Slug("checkpoint-"+string(head)), "2026-02-03T00:00:00Z")
		if err != nil || len(result.Targets) != 1 || result.Targets[0].HeadEventID != head || result.Targets[0].Scope != LifecycleTargetSchema {
			t.Fatalf("build %s = %#v, %v", head, result, err)
		}
		checked, err := CheckTargetsScoped(t.Context(), path, TargetScopeLifecycle, false, "q-one", "f-one", head)
		if err != nil || checked.Targets[0].Valid == nil || !*checked.Targets[0].Valid {
			t.Fatalf("check %s = %#v, %v", head, checked, err)
		}
	}
	firstPath := filepath.Join(filepath.Dir(path), "proofs", "targets", "f-one.lifecycle.event-withdrawn.json")
	secondPath := filepath.Join(filepath.Dir(path), "proofs", "targets", "f-one.lifecycle.event-reaffirmed.json")
	firstBytes, firstErr := os.ReadFile(firstPath)
	secondBytes, secondErr := os.ReadFile(secondPath)
	if firstErr != nil || secondErr != nil || bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("head-specific targets were not retained independently: %v %v", firstErr, secondErr)
	}
}

func TestLifecycleTargetCheckHonorsPortableDeclaredArtifactPaths(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.Conformance(), "tests/conformance/valid/lifecycle-checkpoints.json")
	if err != nil {
		t.Fatal(err)
	}
	var model ledger.Ledger
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(ledgerPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	forecast := &model.Questions[0].Forecasts[0]
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		artifact, err := BuildLifecycleTarget(&model, model.Questions[0].ID, forecast.ID, checkpoint.HeadEventID)
		if err != nil {
			t.Fatal(err)
		}
		declared := lifecycleIntegrityTarget(checkpoint.Integrity)
		path := filepath.Join(directory, filepath.FromSlash(string(declared.ArtifactPath)))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, artifact.Bytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, checkpoint := range *forecast.ActivityCheckpoints {
		declared := lifecycleIntegrityTarget(checkpoint.Integrity)
		result, err := CheckTargetsScoped(t.Context(), ledgerPath, TargetScopeLifecycle, false, model.Questions[0].ID, forecast.ID, checkpoint.HeadEventID)
		if err != nil || len(result.Targets) != 1 || result.Targets[0].Valid == nil || !*result.Targets[0].Valid || result.Targets[0].Path != declared.ArtifactPath {
			t.Fatalf("portable checkpoint %s = %#v, %v", checkpoint.HeadEventID, result, err)
		}
	}
	first := (*forecast.ActivityCheckpoints)[0]
	declared := lifecycleIntegrityTarget(first.Integrity)
	if err := os.Remove(filepath.Join(directory, filepath.FromSlash(string(declared.ArtifactPath)))); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := CommitTargetBuildScoped(t.Context(), ledgerPath, TargetScopeLifecycle, false, model.Questions[0].ID, forecast.ID, first.HeadEventID, "checkpoint-rebuilt", "2026-09-05T00:00:00Z")
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("missing indexed lifecycle target must block mutation = %#v, %v", rebuilt, err)
	}
}

func TestForecastTargetProjectionClassifiesEveryModelField(t *testing.T) {
	type classification string
	const (
		included classification = "included"
		excluded classification = "excluded"
		secret   classification = "secret"
	)

	assertClassified := func(t *testing.T, model any, want map[string]classification) {
		t.Helper()
		typeOf := reflect.TypeOf(model)
		if typeOf.Kind() == reflect.Pointer {
			typeOf = typeOf.Elem()
		}
		got := make([]string, 0, typeOf.NumField())
		for index := 0; index < typeOf.NumField(); index++ {
			name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			got = append(got, name)
			if _, ok := want[name]; !ok {
				t.Errorf("%s.%s (%q) has no target projection classification", typeOf.Name(), typeOf.Field(index).Name, name)
			}
		}
		for name, class := range want {
			if class != included && class != excluded && class != secret {
				t.Errorf("%s.%s has unknown classification %q", typeOf.Name(), name, class)
			}
			if !containsString(got, name) {
				t.Errorf("classification remains for removed %s field %q", typeOf.Name(), name)
			}
		}
	}

	assertClassified(t, ledger.QuestionRevision{}, map[string]classification{
		"id": included, "effective_at": included, "recorded_at": included,
		"title": included, "resolution_criteria": included, "forecasting_opens_at": included,
		"expected_resolution_at": included, "outcome_space": included, "domain": included,
		"provenance": included,
	})
	assertClassified(t, ledger.Forecast{}, map[string]classification{
		"id": included, "question_revision_id": included, "forecasted_at": included,
		"recorded_at": included, "visibility": included, "representations": included,
		"rationale": included, "key_factors": included, "comment": included,
		"public_note": included, "supersedes_forecast_id": included, "provenance": included,
		"lifecycle_events": excluded, "activity_checkpoints": excluded, "commitment": included, "integrity": excluded,
	})
	assertClassified(t, ledger.SealedCommitment{}, map[string]classification{
		"scheme": included, "commitment_hash": included, "encryption": included,
		"key_hint": excluded,
	})
	assertClassified(t, ledger.RevealedCommitment{}, map[string]classification{
		"scheme": included, "commitment_hash": included, "encryption": included,
		"key_hint": excluded, "revealed_at": excluded, "revealed_key": secret,
	})

	projectionFields := jsonFieldNames(targetbytes.ForecastProjection{})
	sort.Strings(projectionFields)
	wantProjection := []string{
		"comment", "commitment", "forecasted_at", "id", "key_factors",
		"provenance", "public_note", "question_revision_id", "rationale", "recorded_at",
		"representations", "supersedes_forecast_id", "visibility",
	}
	if !reflect.DeepEqual(projectionFields, wantProjection) {
		t.Fatalf("target forecast projection fields = %v; want %v", projectionFields, wantProjection)
	}
}

func FuzzBuildForecastTargetDeterminism(f *testing.F) {
	f.Add("Will the event happen?", "public context")
	f.Add("Прогноз с Unicode", "comma, quote, and newline\n")
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "fuzz-ledger", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, "2026-01-01T00:00:00Z")
	if err != nil {
		f.Fatal(err)
	}
	seed, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, title, publicNote string) {
		if len(title) > 2048 || len(publicNote) > 2048 {
			t.Skip()
		}
		model, err := cloneLedger(seed)
		if err != nil {
			t.Fatal(err)
		}
		model.Questions[0].Revisions[0].Title = title
		model.Questions[0].Forecasts[0].PublicNote = &publicNote
		first, err := BuildForecastTarget(model, "q-one", "f-one")
		if err != nil {
			t.Fatal(err)
		}
		second, err := BuildForecastTarget(model, "q-one", "f-one")
		if err != nil {
			t.Fatal(err)
		}
		if first.SHA256 != second.SHA256 || !bytes.Equal(first.Bytes, second.Bytes) {
			t.Fatal("target construction is not deterministic")
		}
		if _, err := document.ParseJSON(bytes.NewReader(first.Bytes), document.DefaultLimits); err != nil {
			t.Fatalf("target is not valid bounded JSON: %v", err)
		}
	})
}

func FuzzBuildLifecycleTargetPrefixDeterminism(f *testing.F) {
	f.Add("review started", "review finished")
	f.Add("Unicode: прогноз", "line one\nline two")
	root, err := BuildLedgerRootAt(InitRootRequest{LedgerID: "fuzz-ledger", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, "2026-01-01T00:00:00Z")
	if err != nil {
		f.Fatal(err)
	}
	seed, err := BuildInitialPublicLedger(root, binaryInitialQuestion())
	if err != nil {
		f.Fatal(err)
	}
	first, err := BuildForecastLifecycle(seed, "q-one", "f-one", ledger.LifecycleWithdrawn, LifecycleInput{ID: "event-withdrawn", EffectiveAt: "2026-02-01T00:00:00Z"}, "2026-02-01T00:00:01Z")
	if err != nil {
		f.Fatal(err)
	}
	second, err := BuildForecastLifecycle(first.Ledger, "q-one", "f-one", ledger.LifecycleReaffirmed, LifecycleInput{ID: "event-reaffirmed", EffectiveAt: "2026-02-02T00:00:00Z"}, "2026-02-02T00:00:01Z")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, firstReason, secondReason string) {
		if len(firstReason) > 2048 || len(secondReason) > 2048 {
			t.Skip()
		}
		model, err := cloneLedger(second.Ledger)
		if err != nil {
			t.Fatal(err)
		}
		events := model.Questions[0].Forecasts[0].LifecycleEvents
		(*events)[0].Reason, (*events)[1].Reason = &firstReason, &secondReason
		withdrawal, err := BuildLifecycleTarget(model, "q-one", "f-one", "event-withdrawn")
		if err != nil {
			t.Fatal(err)
		}
		repeat, err := BuildLifecycleTarget(model, "q-one", "f-one", "event-withdrawn")
		if err != nil || withdrawal.SHA256 != repeat.SHA256 || !bytes.Equal(withdrawal.Bytes, repeat.Bytes) {
			t.Fatal("lifecycle target construction is not deterministic")
		}
		changedLater := secondReason + " changed"
		(*events)[1].Reason = &changedLater
		prefix, err := BuildLifecycleTarget(model, "q-one", "f-one", "event-withdrawn")
		if err != nil || !bytes.Equal(withdrawal.Bytes, prefix.Bytes) {
			t.Fatal("later event changed an earlier lifecycle prefix")
		}
		if _, err := document.ParseJSON(bytes.NewReader(prefix.Bytes), document.DefaultLimits); err != nil {
			t.Fatalf("lifecycle target is not valid bounded JSON: %v", err)
		}
	})
}

func jsonFieldNames(value any) []string {
	typeOf := reflect.TypeOf(value)
	names := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestForecastTargetUsesExactClosedProjectionAndExcludedStatus(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	target, err := BuildForecastTarget(model, "q-election-coalition", "f-election-coalition-001")
	if err != nil {
		t.Fatal(err)
	}
	if target.SHA256 == "" || target.Size == 0 {
		t.Fatalf("target digest is empty: sha256=%s size=%d", target.SHA256, target.Size)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(target.Bytes), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	root := parsed.Root.Any().(map[string]any)
	if len(root) != 3 || root["schema"] != ForecastEnvelopeSchema {
		t.Fatalf("target root = %#v", root)
	}
	question := root["question"].(map[string]any)
	if question["id"] != "q-election-coalition" || question["revision"].(map[string]any)["id"] != "qr-election-coalition-1" {
		t.Fatalf("target question binding = %#v", question)
	}
	forecast := root["forecast"].(map[string]any)
	if _, exists := forecast["integrity"]; exists {
		t.Fatal("target forecast contains integrity")
	}
	changed, err := cloneLedger(model)
	if err != nil {
		t.Fatal(err)
	}
	changed.Questions[1].Status = ledger.QuestionClosed
	changedTarget, err := BuildForecastTarget(changed, "q-election-coalition", "f-election-coalition-001")
	if err != nil || !bytes.Equal(target.Bytes, changedTarget.Bytes) {
		t.Fatalf("excluded status changed target: %v", err)
	}
	changed.Questions[1].Forecasts[0].RecordedAt = "2026-08-06T13:03:00+01:00"
	changedTarget, err = BuildForecastTarget(changed, "q-election-coalition", "f-election-coalition-001")
	if err != nil || bytes.Equal(target.Bytes, changedTarget.Bytes) {
		t.Fatalf("included forecast field did not change target: %v", err)
	}
}

func TestRevealedTargetContinuesOriginalSealedBytes(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "research", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	rationale, comment := "private rationale", "private comment"
	factors := []string{"factor"}
	input.InitialForecast.Rationale, input.InitialForecast.Comment, input.InitialForecast.KeyFactors = &rationale, &comment, &factors
	build, err := BuildInitialSealedLedger(context.Background(), root, input, Effects{Clock: fixedTestClock{}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x23}, 76))}})
	if err != nil {
		t.Fatal(err)
	}
	sealedTarget, err := BuildForecastTarget(build.Ledger, "q-one", "f-one")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := BuildForecastLifecycle(build.Ledger, "q-one", "f-one", ledger.LifecycleExpired, LifecycleInput{ID: "event-sealed-expired", EffectiveAt: "2026-01-15T00:00:00Z"}, "2026-01-15T00:00:01Z")
	if err != nil {
		t.Fatal(err)
	}
	expiredTarget, err := BuildForecastTarget(expired.Ledger, "q-one", "f-one")
	if err != nil || !bytes.Equal(sealedTarget.Bytes, expiredTarget.Bytes) {
		t.Fatalf("sealed lifecycle changed target: %v", err)
	}
	revealed, err := BuildForecastReveal(build.Ledger, "q-one", "f-one", build.KeyFile, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	revealedTarget, err := BuildForecastTarget(revealed.Ledger, "q-one", "f-one")
	if err != nil || !bytes.Equal(sealedTarget.Bytes, revealedTarget.Bytes) {
		t.Fatalf("revealed target changed: %v\nsealed=%s\nrevealed=%s", err, sealedTarget.Bytes, revealedTarget.Bytes)
	}
	current := revealed.Ledger
	for index, event := range []struct {
		kind ledger.LifecycleEventType
		id   ledger.Slug
		at   ledger.Timestamp
	}{
		{ledger.LifecycleWithdrawn, "event-revealed-withdrawn", "2026-02-02T00:00:00Z"},
		{ledger.LifecycleReaffirmed, "event-revealed-reaffirmed", "2026-02-03T00:00:00Z"},
		{ledger.LifecycleExpired, "event-revealed-expired", "2026-02-04T00:00:00Z"},
	} {
		mutation, buildErr := BuildForecastLifecycle(current, "q-one", "f-one", event.kind, LifecycleInput{ID: event.id, EffectiveAt: event.at}, ledger.Timestamp(time.Date(2026, 2, 2+index, 0, 0, 1, 0, time.UTC).Format(time.RFC3339)))
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		current = mutation.Ledger
		after, targetErr := BuildForecastTarget(current, "q-one", "f-one")
		if targetErr != nil || !bytes.Equal(revealedTarget.Bytes, after.Bytes) || revealedTarget.SHA256 != after.SHA256 {
			t.Fatalf("revealed lifecycle %s changed target: %v", event.kind, targetErr)
		}
	}
}

func TestTargetBuildCheckIdempotencyCollisionAndDryRun(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanTargetBuild(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001")
	if err != nil || len(plan.Targets) != 1 {
		t.Fatalf("plan = %#v, %v", plan, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("plan created proofs directory: %v", err)
	}
	created, err := CommitTargetBuild(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001")
	if err != nil || created.Targets[0].State != storage.DeterministicCreated {
		t.Fatalf("created = %#v, %v", created, err)
	}
	checked, err := CheckTargets(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001")
	if err != nil || checked.Targets[0].Valid == nil || !*checked.Targets[0].Valid {
		t.Fatalf("checked = %#v, %v", checked, err)
	}
	retried, err := CommitTargetBuild(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001")
	if err != nil || retried.Targets[0].State != storage.DeterministicUnchanged {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	targetPath := filepath.Join(directory, "proofs", "targets", "f-election-coalition-001.json")
	if err := os.WriteFile(targetPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, inspectErr := InspectTargets(context.Background(), path, true, "", "")
	if inspectErr != nil || inspection.FailureCode != app.CodeVerification || len(inspection.Targets) < 3 {
		t.Fatalf("tampered all inspection = %#v, %v", inspection, inspectErr)
	}
	var failedRows int
	for _, row := range inspection.Targets {
		if row.State == storage.DeterministicState(LayerFail) {
			failedRows++
		}
	}
	if failedRows != 1 {
		t.Fatalf("tampered all rows = %#v", inspection.Targets)
	}
	if _, err := CheckTargets(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001"); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("tampered check error = %v", err)
	}
	if _, err := CommitTargetBuild(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001"); app.ErrorCodeOf(err) != app.CodeVerification {
		t.Fatalf("tampered build error = %v", err)
	}
}

func TestTargetInspectionReportsUnretainedRowsAndKeepsLedgerOrder(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitTargetBuild(context.Background(), path, false, "q-election-coalition", "f-election-coalition-001"); err != nil {
		t.Fatal(err)
	}
	result, err := InspectTargets(context.Background(), path, true, "", "")
	if err != nil || result.FailureCode != "" || len(result.Targets) < 3 {
		t.Fatalf("inspection = %#v, %v", result, err)
	}
	if result.Targets[0].ForecastID != "f-central-bank-cut-001" || result.Targets[0].State != storage.DeterministicState(LayerNotApplicable) {
		t.Fatalf("first target = %#v", result.Targets[0])
	}
	var built, unretained *TargetResult
	for index := range result.Targets {
		if result.Targets[index].ForecastID == "f-election-coalition-001" {
			built = &result.Targets[index]
		}
		if result.Targets[index].State == storage.DeterministicState(LayerNotApplicable) {
			unretained = &result.Targets[index]
		}
	}
	if built == nil || built.State != storage.DeterministicState(LayerPass) {
		t.Fatalf("built target row = %#v", built)
	}
	if unretained == nil || unretained.ReasonCodes[0] != "content.no_retained_target" || unretained.Guidance == "" {
		t.Fatalf("unretained target = %#v", unretained)
	}
	if _, err := CheckTargets(context.Background(), path, false, "q-quarterly-revenue", "f-quarterly-revenue-001"); app.ErrorCodeOf(err) != app.CodeNotFound {
		t.Fatalf("strict target check error = %v", err)
	}
}

func TestTargetBuildAllPreflightsBeforeCreatingAnything(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "proofs", "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "proofs", "targets", "f-quarterly-revenue-001.json"), []byte("collision"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitTargetBuild(context.Background(), path, true, "", ""); app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("all collision error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "proofs", "targets"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "f-quarterly-revenue-001.json" {
		t.Fatalf("all build created partial artifacts: %#v, %v", entries, err)
	}
}

func TestTargetBuildCancellationCreatesNothing(t *testing.T) {
	raw, err := fs.ReadFile(contractschema.ValidExamples(), "individual-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CommitTargetBuild(ctx, path, false, "q-election-coalition", "f-election-coalition-001"); app.ErrorCodeOf(err) != app.CodeInterrupted {
		t.Fatalf("canceled target build error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("canceled target build created directory: %v", err)
	}
}
