package service

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

func TestPublishedV2UnionShapesHaveJSONAndYAMLParity(t *testing.T) {
	models := []*ledger.Ledger{}
	_, individual := rootUpdateFixture(t, "individual-ledger.json")
	models = append(models, individual)
	relationshipBytes, err := fs.ReadFile(contractschema.Conformance(), "tests/conformance/valid/relationships-and-datetime.json")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(relationshipBytes), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	relationships, err := validation.DecodeLedger(parsed)
	if err != nil {
		t.Fatal(err)
	}
	models = append(models, relationships)

	domains := map[ledger.OutcomeKind]bool{}
	representations := map[ledger.RepresentationKind]bool{}
	relationshipKinds := map[ledger.RelationshipKind]bool{}
	resolutionStatuses := map[ledger.ResolutionStatus]bool{}
	var sawBins, sawProvenance, sawLifecycle bool
	for index, model := range models {
		t.Run(fmt.Sprintf("fixture-%d", index), func(t *testing.T) {
			jsonPath, yamlPath := newFormatParityLedgers(t, model)
			assertFormatParityLedgers(t, jsonPath, yamlPath)
		})
		for _, question := range model.Questions {
			for _, revision := range question.Revisions {
				domains[revision.OutcomeSpace.Kind] = true
				sawProvenance = sawProvenance || revision.Provenance != nil
				if revision.Domain.Numeric != nil {
					sawBins = sawBins || revision.Domain.Numeric.BinSets != nil
				}
				if revision.Domain.Date != nil {
					sawBins = sawBins || revision.Domain.Date.BinSets != nil
				}
				if revision.Domain.Datetime != nil {
					sawBins = sawBins || revision.Domain.Datetime.BinSets != nil
				}
			}
			for _, forecast := range question.Forecasts {
				sawProvenance = sawProvenance || forecast.Provenance != nil
				sawLifecycle = sawLifecycle || forecast.LifecycleEvents != nil
				if forecast.Representations != nil {
					for _, representation := range *forecast.Representations {
						representations[representationKind(representation)] = true
					}
				}
			}
			if question.Resolution != nil {
				switch {
				case question.Resolution.Resolved != nil:
					resolutionStatuses[question.Resolution.Resolved.Status] = true
				case question.Resolution.Unresolved != nil:
					resolutionStatuses[question.Resolution.Unresolved.Status] = true
				case question.Resolution.NotApplicable != nil:
					resolutionStatuses[question.Resolution.NotApplicable.Status] = true
				}
			}
		}
		if model.Relationships != nil {
			for _, relationship := range *model.Relationships {
				if relationship.GroupMembership != nil {
					relationshipKinds[relationship.GroupMembership.Kind] = true
				}
				if relationship.Conditional != nil {
					relationshipKinds[relationship.Conditional.Kind] = true
				}
			}
		}
	}
	assertEnumCoverage(t, domains, []ledger.OutcomeKind{ledger.OutcomeBinary, ledger.OutcomeCategorical, ledger.OutcomeOrdinal, ledger.OutcomeNumeric, ledger.OutcomeDate, ledger.OutcomeDatetime})
	assertEnumCoverage(t, representations, []ledger.RepresentationKind{ledger.RepresentationProbability, ledger.RepresentationPMF, ledger.RepresentationBinnedPMF, ledger.RepresentationQuantiles, ledger.RepresentationCDF, ledger.RepresentationPoint, ledger.RepresentationCredibleIntervals})
	assertEnumCoverage(t, relationshipKinds, []ledger.RelationshipKind{ledger.RelationshipGroupMembership, ledger.RelationshipConditional})
	if !sawBins || !sawProvenance || !sawLifecycle || !resolutionStatuses[ledger.ResolutionResolved] || !resolutionStatuses[ledger.ResolutionNotApplicable] {
		t.Fatalf("fixture coverage incomplete: bins=%v provenance=%v lifecycle=%v resolutions=%v", sawBins, sawProvenance, sawLifecycle, resolutionStatuses)
	}
}

func TestRelationshipAddNormalizesUnionShapeForWholeCollectionAndAppend(t *testing.T) {
	_, individual := rootUpdateFixture(t, "individual-ledger.json")
	conditionalBytes, err := fs.ReadFile(contractschema.Conformance(), "tests/conformance/valid/relationships-and-datetime.json")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := document.ParseJSON(bytes.NewReader(conditionalBytes), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	conditionalModel, err := validation.DecodeLedger(parsed)
	if err != nil {
		t.Fatal(err)
	}

	low := "low"
	tests := []struct {
		name         string
		model        *ledger.Ledger
		relationship ledger.Relationship
		shape        string
	}{
		{name: "group-membership absent", model: func() *ledger.Ledger { cloned, _ := cloneLedger(individual); cloned.Relationships = nil; return cloned }(), relationship: ledger.Relationship{GroupMembership: &ledger.GroupMembership{ID: "membership-coalition", Kind: ledger.RelationshipGroupMembership, GroupID: "macro-2026", QuestionID: "q-election-coalition"}}, shape: "- id: membership-coalition\nkind: group_membership\ngroup_id: macro-2026\nquestion_id: q-election-coalition"},
		{name: "group-membership append", model: individual, relationship: ledger.Relationship{GroupMembership: &ledger.GroupMembership{ID: "membership-coalition", Kind: ledger.RelationshipGroupMembership, GroupID: "macro-2026", QuestionID: "q-election-coalition"}}, shape: "- id: membership-coalition\nkind: group_membership\ngroup_id: macro-2026\nquestion_id: q-election-coalition"},
		{name: "conditional absent", model: func() *ledger.Ledger {
			cloned, _ := cloneLedger(conditionalModel)
			cloned.Relationships = nil
			cloned.Questions[1].Status = ledger.QuestionOpen
			cloned.Questions[1].Resolution = nil
			return cloned
		}(), relationship: ledger.Relationship{Conditional: &ledger.ConditionalRelationship{ID: "condition-low-release", Kind: ledger.RelationshipConditional, ParentQuestionID: "q-demand-level", ParentQuestionRevisionID: "qr-demand-level-1", ParentOutcome: ledger.ScalarValue{String: &low}, ChildQuestionID: "q-release-moment"}}, shape: "- id: condition-low-release\nkind: conditional\nparent_question_id: q-demand-level\nparent_question_revision_id: qr-demand-level-1\nparent_outcome: low\nchild_question_id: q-release-moment"},
		{name: "conditional append", model: conditionalModel, relationship: ledger.Relationship{Conditional: &ledger.ConditionalRelationship{ID: "condition-low-release", Kind: ledger.RelationshipConditional, ParentQuestionID: "q-demand-level", ParentQuestionRevisionID: "qr-demand-level-1", ParentOutcome: ledger.ScalarValue{String: &low}, ChildQuestionID: "q-release-moment"}}, shape: "- id: condition-low-release\nkind: conditional\nparent_question_id: q-demand-level\nparent_question_revision_id: qr-demand-level-1\nparent_outcome: low\nchild_question_id: q-release-moment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jsonPath, yamlPath := newFormatParityLedgers(t, test.model)
			yamlBefore, err := os.ReadFile(yamlPath)
			if err != nil {
				t.Fatal(err)
			}
			yamlBefore = append([]byte("# unrelated source comment\n"), yamlBefore...)
			if err := os.WriteFile(yamlPath, yamlBefore, 0o600); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{jsonPath, yamlPath} {
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				planned, err := PlanRelationshipAddFile(t.Context(), path, RelationshipInput{Relationship: test.relationship})
				if err != nil || !planned.Changed || len(planned.ChangedPointers) != 1 {
					t.Fatalf("plan %s = %#v, %v", filepath.Ext(path), planned, err)
				}
				afterPlan, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, afterPlan) {
					t.Fatalf("dry run changed %s: %v", filepath.Ext(path), err)
				}
				if _, err := CommitRelationshipAddFile(t.Context(), path, RelationshipInput{Relationship: test.relationship}); err != nil {
					t.Fatalf("commit %s: %v", filepath.Ext(path), err)
				}
			}
			yamlAfter, err := os.ReadFile(yamlPath)
			if err != nil {
				t.Fatal(err)
			}
			text := string(yamlAfter)
			lines := strings.Split(text, "\n")
			for index := range lines {
				lines[index] = strings.TrimLeft(lines[index], " ")
			}
			ordered := strings.Join(lines, "\n")
			if !strings.HasPrefix(text, "# unrelated source comment\n") || !strings.Contains(ordered, test.shape) {
				t.Fatalf("YAML shape/source preservation mismatch:\n%s", text)
			}
			if strings.Contains(text, "group_membership:\n") || strings.Contains(text, "conditional:\n") {
				t.Fatalf("YAML exposed union wrapper:\n%s", text)
			}
			assertFormatParityLedgers(t, jsonPath, yamlPath)
		})
	}
}

func TestInvalidRelationshipAddIsAtomic(t *testing.T) {
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	jsonPath, yamlPath := newFormatParityLedgers(t, model)
	invalid := RelationshipInput{Relationship: ledger.Relationship{GroupMembership: &ledger.GroupMembership{
		ID: "missing-question", Kind: ledger.RelationshipGroupMembership, GroupID: "macro-2026", QuestionID: "q-does-not-exist",
	}}}
	for _, path := range []string{jsonPath, yamlPath} {
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CommitRelationshipAddFile(t.Context(), path, invalid); err == nil {
			t.Fatalf("invalid relationship was accepted for %s", filepath.Ext(path))
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("invalid relationship changed %s: %v", filepath.Ext(path), err)
		}
	}
}

func TestUnresolvedResolutionBranchesHaveJSONAndYAMLParity(t *testing.T) {
	for _, status := range []ledger.ResolutionStatus{ledger.ResolutionAmbiguous, ledger.ResolutionVoid} {
		t.Run(string(status), func(t *testing.T) {
			jsonPath, yamlPath := newFormatParityLedgers(t, testPublicInitialLedger(t))
			for _, path := range []string{jsonPath, yamlPath} {
				if _, err := CommitQuestionUpdateFile(context.Background(), path, "q-one", QuestionPatchInput{Status: Optional[ledger.QuestionStatus]{Set: true, Value: ledger.QuestionClosed}}); err != nil {
					t.Fatal(err)
				}
				if _, err := CommitQuestionUnresolvedFile(context.Background(), path, "q-one", status, UnresolvedResolutionInput{Reason: "The outcome cannot be determined."}, "2027-01-01T00:00:00Z"); err != nil {
					t.Fatal(err)
				}
			}
			assertFormatParityLedgers(t, jsonPath, yamlPath)
		})
	}
}

func assertEnumCoverage[T ~string](t *testing.T, got map[T]bool, want []T) {
	t.Helper()
	missing := []string{}
	for _, value := range want {
		if !got[value] {
			missing = append(missing, string(value))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("missing union branches: %s", strings.Join(missing, ", "))
	}
}

func TestQuestionReplacementLifecycleHasJSONAndYAMLParity(t *testing.T) {
	jsonPath, yamlPath := formatParityLedgers(t)
	questionID := ledger.Slug("q-election-coalition")
	_, fixture := rootUpdateFixture(t, "individual-ledger.json")
	base := fixture.Questions[1].Revisions[1]
	revision := RevisionInput{
		ID: "qr-election-coalition-3", EffectiveAt: "2026-08-21T08:00:00+01:00", RecordedAt: timestampPointer("2026-08-21T08:02:00+01:00"),
		Title: "Updated coalition question", ResolutionCriteria: base.ResolutionCriteria,
		ExpectedResolutionAt: "2026-10-20T12:00:00+01:00", OutcomeSpace: base.OutcomeSpace, Domain: base.Domain,
	}
	for _, path := range []string{jsonPath, yamlPath} {
		if _, err := CommitQuestionReviseFile(context.Background(), path, questionID, revision, "2026-08-21T08:02:00+01:00"); err != nil {
			if applicationErr, ok := err.(*app.Error); ok {
				t.Fatalf("question revise %s: %v", filepath.Ext(path), applicationErr.Cause)
			}
			t.Fatalf("question revise %s: %#v", filepath.Ext(path), err)
		}
		update := QuestionPatchInput{Tags: Optional[[]ledger.Slug]{Set: true, Value: []ledger.Slug{"reviewed", "coalition"}}, Status: Optional[ledger.QuestionStatus]{Set: true, Value: ledger.QuestionClosed}}
		if _, err := CommitQuestionUpdateFile(context.Background(), path, questionID, update); err != nil {
			t.Fatalf("question update %s: %v", filepath.Ext(path), err)
		}
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)

	outcome := "centre-left"
	resolution := ResolutionInput{
		QuestionRevisionID: "qr-election-coalition-3", Outcome: ledger.ScalarValue{String: &outcome},
		OutcomeKnownAt: "2026-10-15T12:00:00+01:00", RecordedAt: timestampPointer("2026-10-15T12:05:00+01:00"),
		Sources: []EvidenceSourceInput{{
			Title: "Official appointment", URL: "https://example.test/result", RetrievedAt: "2026-10-15T12:04:00+01:00",
		}},
	}
	for _, path := range []string{jsonPath, yamlPath} {
		if _, err := CommitQuestionResolveFile(context.Background(), path, questionID, resolution, "2026-10-15T12:05:00+01:00"); err != nil {
			t.Fatalf("question resolve %s: %v", filepath.Ext(path), err)
		}
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)

	dispute := UnresolvedResolutionInput{Reason: "The appointment is under review.", RecordedAt: timestampPointer("2026-10-16T00:00:00+01:00")}
	for _, path := range []string{jsonPath, yamlPath} {
		if _, err := CommitQuestionUnresolvedFile(context.Background(), path, questionID, ledger.ResolutionDisputed, dispute, "2026-10-16T00:00:00+01:00"); err != nil {
			t.Fatalf("question dispute %s: %v", filepath.Ext(path), err)
		}
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)
}

func TestRootAndPlatformReplacementsHaveJSONAndYAMLParity(t *testing.T) {
	jsonPath, yamlPath := formatParityLedgers(t)
	website := "https://example.test/updated"
	rootUpdate := RootMetadataPatchInput{
		Title:           Optional[string]{Set: true, Value: "Updated parity ledger"},
		DefaultTimezone: Optional[string]{Set: true, Value: "UTC"},
		Forecaster: Optional[ForecasterMetadataPatchInput]{Set: true, Value: ForecasterMetadataPatchInput{
			Name:    Optional[string]{Set: true, Value: "Updated Forecaster"},
			Contact: Optional[ledger.Contact]{Set: true, Value: ledger.Contact{Website: &website}},
		}},
	}
	platformUpdate := PlatformPatchInput{
		Name: Optional[string]{Set: true, Value: "Updated Metaculus platform"},
		Kind: Optional[ledger.PlatformKind]{Set: true, Value: ledger.PlatformInternal},
		Account: Optional[PlatformAccountPatchInput]{Set: true, Value: PlatformAccountPatchInput{
			Username: Optional[string]{Set: true, Value: "parity-user"},
		}},
	}
	for _, path := range []string{jsonPath, yamlPath} {
		if _, err := CommitRootMetadataFileUpdate(context.Background(), path, rootUpdate); err != nil {
			t.Fatalf("root update %s: %v", filepath.Ext(path), err)
		}
		if _, err := CommitPlatformUpdateFile(context.Background(), path, "metaculus", platformUpdate); err != nil {
			t.Fatalf("platform update %s: %v", filepath.Ext(path), err)
		}
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)
}

func TestForecastRevealReplacementHasJSONAndYAMLParity(t *testing.T) {
	root, err := BuildLedgerRoot(InitRootRequest{LedgerID: "replacement-parity", Timezone: "UTC", ForecasterID: "me", ForecasterName: "Me"}, fixedTestClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	input := binaryInitialQuestion()
	input.InitialForecast.Visibility = ledger.VisibilitySealed
	rationale, comment := "PRIVATE-REVEAL-RATIONALE", "PRIVATE-REVEAL-COMMENT"
	factors := []string{"PRIVATE-REVEAL-FACTOR"}
	input.InitialForecast.Rationale, input.InitialForecast.Comment, input.InitialForecast.KeyFactors = &rationale, &comment, &factors
	build, err := BuildInitialSealedLedger(context.Background(), root, input, Effects{
		Clock: fixedTestClock{}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x52}, 76))},
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonPath, yamlPath := newFormatParityLedgers(t, build.Ledger)
	var targetBytes [][]byte
	for _, path := range []string{jsonPath, yamlPath} {
		keyPath := filepath.Join(filepath.Dir(path), "forecast.key")
		if err := storage.CreateProtectedFile(keyPath, build.KeyFile); err != nil {
			t.Fatal(err)
		}
		if _, err := CommitTargetBuild(context.Background(), path, false, "q-one", "f-one"); err != nil {
			t.Fatalf("target build %s: %v", filepath.Ext(path), err)
		}
		result, revealErr := CommitForecastRevealFile(context.Background(), path, keyPath, "q-one", "f-one", "2026-02-01T00:00:00Z")
		publicResult := fmt.Sprintf("%#v %v", result, revealErr)
		if revealErr != nil || !result.Changed {
			if applicationErr, ok := revealErr.(*app.Error); ok {
				t.Fatalf("reveal %s: %s (%v)", filepath.Ext(path), publicResult, applicationErr.Cause)
			}
			t.Fatalf("reveal %s: %s (%#v)", filepath.Ext(path), publicResult, revealErr)
		}
		for _, forbidden := range []string{rationale, comment, factors[0], keyPath, filepath.Dir(path)} {
			if strings.Contains(publicResult, forbidden) {
				t.Fatalf("reveal result leaked %q: %s", forbidden, publicResult)
			}
		}
		retained, err := os.ReadFile(filepath.Join(filepath.Dir(path), "proofs", "targets", "f-one.json"))
		if err != nil {
			t.Fatal(err)
		}
		targetBytes = append(targetBytes, retained)
	}
	if !bytes.Equal(targetBytes[0], targetBytes[1]) {
		t.Fatal("JSON and YAML reveal workflows produced different canonical target bytes")
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)
}

func TestTimestampIntegrityReplacementHasJSONAndYAMLParity(t *testing.T) {
	jsonPath, yamlPath := formatParityLedgers(t)
	tsaURL := "https://tsa.example.test/stamp"
	requestPath, _, err := TimestampEvidencePaths("f-election-coalition-001", tsaURL)
	if err != nil {
		t.Fatal(err)
	}
	var targetBytes [][]byte
	var results []TimestampArtifactResult
	for _, path := range []string{jsonPath, yamlPath} {
		directory := filepath.Dir(path)
		if _, err := CommitTargetBuild(t.Context(), path, false, "q-election-coalition", "f-election-coalition-001"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(timestampTestCAAbsolute(t, directory), timestampFixture(t, "root.pem"), 0o600); err != nil {
			t.Fatal(err)
		}
		absoluteRequest := filepath.Join(directory, filepath.FromSlash(string(requestPath)))
		if err := os.MkdirAll(filepath.Dir(absoluteRequest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absoluteRequest, timestampFixture(t, "request.tsq"), 0o600); err != nil {
			t.Fatal(err)
		}
		transport := &countingRoundTripper{response: timestampFixture(t, "response.tsr")}
		result, err := CommitTimestampStamp(t.Context(), path, "q-election-coalition", "f-election-coalition-001", TimestampStampOptions{
			TSAURL: tsaURL, CABundlePath: testTimestampCAPath,
			Effects:    Effects{Clock: fixedTestClock{value: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)}, Random: deterministicTestRandom{reader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}},
			HTTPClient: testTimestampHTTPClient(transport),
		})
		if err != nil || result.State != TimestampVerified || transport.requests != 1 {
			t.Fatalf("timestamp stamp %s: result=%#v err=%v requests=%d", filepath.Ext(path), result, err, transport.requests)
		}
		results = append(results, result)
		retained, err := os.ReadFile(filepath.Join(directory, "proofs", "targets", "f-election-coalition-001.json"))
		if err != nil {
			t.Fatal(err)
		}
		targetBytes = append(targetBytes, retained)
	}
	if results[0].State != results[1].State || !reflect.DeepEqual(results[0].Entries, results[1].Entries) {
		t.Fatalf("timestamp results differ: JSON=%#v YAML=%#v", results[0], results[1])
	}
	if !bytes.Equal(targetBytes[0], targetBytes[1]) {
		t.Fatal("JSON and YAML timestamp workflows produced different canonical target bytes")
	}
	assertFormatParityLedgers(t, jsonPath, yamlPath)
}

func formatParityLedgers(t *testing.T) (string, string) {
	t.Helper()
	_, model := rootUpdateFixture(t, "individual-ledger.json")
	return newFormatParityLedgers(t, model)
}

func newFormatParityLedgers(t *testing.T, model *ledger.Ledger) (string, string) {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, 2)
	for _, extension := range []string{".json", ".yaml"} {
		directory := filepath.Join(root, strings.TrimPrefix(extension, "."))
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "ledger"+extension)
		if _, err := CommitNewLedger(path, model); err != nil {
			t.Fatalf("create %s parity ledger: %v", extension, err)
		}
		paths = append(paths, path)
	}
	return paths[0], paths[1]
}

func assertFormatParityLedgers(t *testing.T, jsonPath, yamlPath string) {
	t.Helper()
	jsonLedger, err := LoadAndValidateLedger(context.Background(), jsonPath, nil)
	if err != nil {
		t.Fatalf("load JSON ledger: %v", err)
	}
	yamlLedger, err := LoadAndValidateLedger(context.Background(), yamlPath, nil)
	if err != nil {
		t.Fatalf("load YAML ledger: %v", err)
	}
	if !reflect.DeepEqual(jsonLedger.Model, yamlLedger.Model) {
		t.Fatalf("JSON and YAML ledger models differ:\nJSON: %#v\nYAML: %#v", jsonLedger.Model, yamlLedger.Model)
	}
}
