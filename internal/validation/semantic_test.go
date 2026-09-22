package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestAllPublishedV2FixturesPassSemanticValidation(t *testing.T) {
	for _, name := range []string{
		"examples/valid/empty-ledger.json",
		"examples/valid/individual-ledger.json",
		"examples/valid/question-without-forecasts.yaml",
		"examples/valid/team-ledger.yaml",
		"tests/conformance/valid/lifecycle-checkpoints.json",
		"tests/conformance/valid/relationships-and-datetime.json",
		"tests/conformance/valid/revealed-representation-only.json",
	} {
		t.Run(name, func(t *testing.T) {
			model := loadValidLedger(t, name)
			issues, err := ValidateSemantics(model, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(issues) != 0 {
				t.Fatalf("valid fixture has semantic issues: %#v", issues)
			}
		})
	}
}

func TestSemanticValidationChecksRootRevisionAndForecastRules(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	model.DefaultTimezone = "Not/A-Timezone"
	model.Questions[1].ID = model.Questions[0].ID
	question := &model.Questions[1]
	question.CurrentRevisionID = question.Revisions[0].ID
	duplicateRevision := question.Revisions[0]
	question.Revisions = append(question.Revisions, duplicateRevision)
	question.Revisions[1].Domain = ledger.Domain{Binary: &ledger.BinaryDomain{Kind: ledger.OutcomeBinary}}
	model.Questions[2].Forecasts[0].QuestionRevisionID = "missing"
	question.Forecasts[1].RecordedAt = "2026-01-01T00:00:00Z"
	missing := ledger.Slug("missing")
	question.Forecasts[0].SupersedesForecastID = &missing

	issues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		"semantic.timezone",
		"semantic.duplicate_question_id",
		"semantic.duplicate_revision_id",
		"semantic.current_revision_last",
		"semantic.revision_effective_order",
		"semantic.domain_kind",
		"semantic.forecast_revision",
		"semantic.forecast_order",
		"semantic.supersedes",
	} {
		if !hasSemanticCode(issues, code) {
			t.Errorf("missing %s in %#v", code, issues)
		}
	}
}

func TestSemanticValidationChecksRepresentationRules(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	pmf := (*model.Questions[1].Forecasts[0].Representations)[0].PMF
	pmf.OptionSetRef.Version = 9
	pmf.Entries[0].Probability = "0.47"
	pmf.Entries[1].OptionID = pmf.Entries[0].OptionID

	representations := *model.Questions[2].Forecasts[0].Representations
	representations[1].Quantiles.Points[1].Level = representations[1].Quantiles.Points[0].Level
	representations[1].Quantiles.Points[2].Value = scalarString("700")
	representations[2].CDF.Points[1].Probability = "0.05"
	representations[2].CDF.Points[2].Value = representations[2].CDF.Points[1].Value
	representations[2].CDF.LeftTailProbability = "0.2"
	representations[2].CDF.RightTailProbability = "0.1"
	representations[3].CredibleIntervals.Intervals[0].Lower = scalarString("925")
	representations[3].CredibleIntervals.Intervals[0].Upper = scalarString("770")
	representations[4].BinnedPMF.Entries[1].BinID = representations[4].BinnedPMF.Entries[0].BinID
	representations[4].BinnedPMF.RightTailProbability = "0.07"
	duplicate := representations[0]
	representations = append(representations, duplicate)
	model.Questions[2].Forecasts[0].Representations = &representations

	issues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		"semantic.option_set_ref",
		"semantic.duplicate_pmf_option",
		"semantic.pmf_coverage",
		"semantic.probability_sum",
		"semantic.quantile_level_order",
		"semantic.quantile_value_order",
		"semantic.cdf_probability_order",
		"semantic.cdf_value_order",
		"semantic.cdf_left_tail",
		"semantic.cdf_right_tail",
		"semantic.interval_order",
		"semantic.duplicate_binned_pmf_bin",
		"semantic.binned_pmf_coverage",
		"semantic.binned_probability_sum",
		"semantic.duplicate_representation_kind",
	} {
		if !hasSemanticCode(issues, code) {
			t.Errorf("missing %s in %#v", code, issues)
		}
	}
}

func TestSemanticValidationChecksLifecycleRelationshipsAndResolution(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	events := model.Questions[1].Forecasts[1].LifecycleEvents
	(*events)[1].Type = ledger.LifecycleExpired
	(*events)[1].ID = (*events)[0].ID
	(*events)[1].EffectiveAt = "2026-01-01T00:00:00Z"

	child := &model.Questions[1]
	parentOutcome := scalarBool(true)
	condition := ledger.Relationship{Conditional: &ledger.ConditionalRelationship{
		ID: "conditional-cycle", Kind: ledger.RelationshipConditional,
		ParentQuestionID: child.ID, ParentQuestionRevisionID: child.Revisions[0].ID,
		ParentOutcome: scalarString("missing-option"), ChildQuestionID: model.Questions[0].ID,
	}}
	*model.Relationships = append(*model.Relationships, condition, ledger.Relationship{Conditional: &ledger.ConditionalRelationship{
		ID: "conditional-back", Kind: ledger.RelationshipConditional,
		ParentQuestionID: model.Questions[0].ID, ParentQuestionRevisionID: model.Questions[0].Revisions[0].ID,
		ParentOutcome: parentOutcome, ChildQuestionID: child.ID,
	}})
	model.Questions[0].Resolution.Resolved.RecordedAt = "2026-01-01T00:00:00Z"
	model.Questions[0].Resolution.Resolved.Outcome = scalarString("not-bool")

	issues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		"semantic.duplicate_lifecycle_event_id",
		"semantic.lifecycle_transition",
		"semantic.lifecycle_effective_order",
		"semantic.relationship_outcome",
		"semantic.relationship_cycle",
		"semantic.resolution_chronology",
		"semantic.resolution_outcome",
	} {
		if !hasSemanticCode(issues, code) {
			t.Errorf("missing %s in %#v", code, issues)
		}
	}
}

func TestSemanticValidationChecksLifecycleChronologyAndCheckpointPrefixes(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	forecast := &model.Questions[1].Forecasts[1]
	(*forecast.LifecycleEvents)[0].EffectiveAt = "2026-08-20T13:00:00+01:00"
	(*forecast.LifecycleEvents)[0].RecordedAt = "2026-08-20T13:01:00+01:00"
	digest := ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(strings.Repeat("0", 64))}
	target := func(path ledger.RelativePath) ledger.LifecycleIntegrity {
		return ledger.LifecycleIntegrity{Pending: &ledger.PendingLifecycleIntegrity{
			Status:     ledger.IntegrityPending,
			Target:     ledger.LifecycleTarget{Scope: "forecast-lifecycle/v1", Canonicalization: "RFC8785", ArtifactPath: path, Digest: digest},
			Timestamps: []ledger.RFC3161Timestamp{},
		}}
	}
	checkpoints := []ledger.ActivityCheckpoint{
		{ID: "checkpoint-same", HeadEventID: "event-election-reaffirmed", RecordedAt: "2026-08-25T11:02:00+01:00", Integrity: target("proofs/targets/f-election-coalition-002.lifecycle.event-election-reaffirmed.json")},
		{ID: "checkpoint-same", HeadEventID: "event-election-withdrawn", RecordedAt: "2026-08-19T09:00:00+01:00", Integrity: target("proofs/targets/wrong.json")},
	}
	forecast.ActivityCheckpoints = &checkpoints
	issues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		"semantic.lifecycle_forecast_chronology",
		"semantic.activity_checkpoint_order",
		"semantic.activity_checkpoint_chronology",
		"semantic.duplicate_activity_checkpoint_id",
		"semantic.activity_target_digest",
	} {
		if !hasSemanticCode(issues, code) {
			t.Errorf("missing %s in %#v", code, issues)
		}
	}
}

func FuzzConditionalRelationshipGraph(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2})
	f.Add([]byte{0, 1, 1, 0})
	f.Fuzz(func(t *testing.T, edges []byte) {
		if len(edges) > 128 {
			edges = edges[:128]
		}
		graph := make(map[ledger.Slug][]ledger.Slug)
		selfCycle := false
		for index := 0; index+1 < len(edges); index += 2 {
			from := ledger.Slug(fmt.Sprintf("q-%d", edges[index]%32))
			to := ledger.Slug(fmt.Sprintf("q-%d", edges[index+1]%32))
			graph[from] = append(graph[from], to)
			if _, ok := graph[to]; !ok {
				graph[to] = nil
			}
			selfCycle = selfCycle || from == to
		}
		if selfCycle && !conditionalCycle(graph) {
			t.Fatal("self-referential relationship graph was not classified as cyclic")
		}
		// A second pass must be stable even though map iteration order varies.
		if first, second := conditionalCycle(graph), conditionalCycle(graph); first != second {
			t.Fatalf("cycle classification was not deterministic: %v then %v", first, second)
		}
	})
}

func TestSemanticValidationChecksConfinedSnapshotAndTargetDigests(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	data := []byte("retained bytes")
	digest := sha256.Sum256(data)
	digestValue := ledger.Hex32(hex.EncodeToString(digest[:]))
	model.Questions[0].Revisions[0].Provenance = &ledger.Provenance{
		Platform: "local", RemoteObjectID: "one", RetrievedAt: "2026-01-01T00:00:00Z",
		Snapshot: &ledger.ArtifactSnapshot{ArtifactPath: "snapshots/one.json", MediaType: "application/json", Digest: ledger.Digest{Algorithm: "sha-256", Value: digestValue}},
	}
	model.Platforms["local"] = ledger.Platform{Name: "Local", Kind: ledger.PlatformInternal}
	model.Questions[0].Forecasts[0].Integrity = ledger.Integrity{Pending: &ledger.PendingIntegrity{
		Status:     ledger.IntegrityPending,
		Target:     ledger.ForecastTarget{Scope: "forecast-envelope/v2", Canonicalization: "RFC8785", ArtifactPath: "targets/one.json", Digest: ledger.Digest{Algorithm: "sha-256", Value: digestValue}},
		Timestamps: []ledger.RFC3161Timestamp{{Type: "rfc3161", State: ledger.RFC3161Pending}},
	}}
	artifacts := fstest.MapFS{
		"snapshots/one.json": &fstest.MapFile{Data: data},
		"targets/one.json":   &fstest.MapFile{Data: data},
	}
	issues, err := ValidateSemantics(model, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if hasSemanticCode(issues, "semantic.artifact_digest") || hasSemanticCode(issues, "semantic.artifact_missing") {
		t.Fatalf("matching artifacts rejected: %#v", issues)
	}
	artifacts["snapshots/one.json"] = &fstest.MapFile{Data: []byte("tampered")}
	issues, err = ValidateSemantics(model, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSemanticCode(issues, "semantic.artifact_digest") {
		t.Fatalf("digest mismatch not reported: %#v", issues)
	}
	model.Questions[0].Revisions[0].Provenance.Snapshot.ArtifactPath = "../escape"
	issues, err = ValidateSemantics(model, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSemanticCode(issues, "semantic.artifact_path") {
		t.Fatalf("unconfined path not rejected: %#v", issues)
	}
}

func TestVerifiedTimestampMustStrictlyPredateKnownOutcome(t *testing.T) {
	model := loadValidLedger(t, "examples/valid/individual-ledger.json")
	known := model.Questions[0].Resolution.Resolved.OutcomeKnownAt
	model.Questions[0].Forecasts[0].Integrity = verifiedIntegrityAt(known)
	issues, err := ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSemanticCode(issues, "semantic.timestamp_chronology") {
		t.Fatalf("equal timestamp was accepted: %#v", issues)
	}
	model.Questions[0].Forecasts[0].Integrity = verifiedIntegrityAt("2026-09-17T11:59:59+01:00")
	issues, err = ValidateSemantics(model, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hasSemanticCode(issues, "semantic.timestamp_chronology") {
		t.Fatalf("earlier timestamp was rejected: %#v", issues)
	}
}

func loadValidLedger(t *testing.T, name string) *ledger.Ledger {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), name)
	if err != nil {
		t.Fatal(err)
	}
	var parsed *document.Document
	if strings.HasSuffix(name, ".json") {
		parsed, err = document.ParseJSON(strings.NewReader(string(data)), document.DefaultLimits)
	} else {
		parsed, err = document.ParseYAML(strings.NewReader(string(data)), document.DefaultLimits)
	}
	if err != nil {
		t.Fatal(err)
	}
	structural, err := DefaultStructuralValidator()
	if err != nil {
		t.Fatal(err)
	}
	issues, err := structural.Validate(parsed.Root.Any())
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("fixture is not structurally valid: %#v", issues)
	}
	model, err := DecodeLedger(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func hasSemanticCode(issues []SemanticIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func scalarString(value string) ledger.ScalarValue { return ledger.ScalarValue{String: &value} }
func scalarBool(value bool) ledger.ScalarValue     { return ledger.ScalarValue{Boolean: &value} }

func verifiedIntegrityAt(value ledger.Timestamp) ledger.Integrity {
	policy, serial := "1.2.3", "1"
	ca := ledger.RelativePath("ca.pem")
	return ledger.Integrity{Verified: &ledger.VerifiedIntegrity{
		Status:     ledger.IntegrityVerified,
		Target:     ledger.ForecastTarget{Scope: "forecast-envelope/v2", Canonicalization: "RFC8785", ArtifactPath: "target.json", Digest: ledger.Digest{Algorithm: "sha-256", Value: ledger.Hex32(strings.Repeat("0", 64))}},
		Timestamps: []ledger.RFC3161Timestamp{{Type: "rfc3161", State: ledger.RFC3161Verified, GenTime: &value, PolicyOID: &policy, SerialNumber: &serial, CABundlePath: &ca}},
		VerifiedAt: value,
	}}
}
