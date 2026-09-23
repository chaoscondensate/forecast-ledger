package ledger

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/document"
	contractschema "github.com/chaoscondensate/forecast-ledger/internal/schema"
)

func TestPublishedV2FixturesRoundTripThroughTypedModel(t *testing.T) {
	t.Parallel()

	tests := []string{
		"examples/valid/empty-ledger.json",
		"examples/valid/individual-ledger.json",
		"examples/valid/question-without-forecasts.yaml",
		"examples/valid/team-ledger.yaml",
		"tests/conformance/valid/lifecycle-checkpoints.json",
		"tests/conformance/valid/relationships-and-datetime.json",
		"tests/conformance/valid/retained-forecast.json",
		"tests/conformance/valid/revealed-representation-only.json",
	}
	for _, name := range tests {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := fixtureAsJSON(t, name)
			var model Ledger
			if err := json.Unmarshal(source, &model); err != nil {
				t.Fatalf("decode typed ledger: %v", err)
			}
			if model.SchemaVersion != "2.2.0" {
				t.Fatalf("schema version = %q", model.SchemaVersion)
			}
			encoded, err := json.Marshal(model)
			if err != nil {
				t.Fatalf("encode typed ledger: %v", err)
			}
			assertJSONEqual(t, source, encoded)
			assertNoJSONFloats(t, encoded)
		})
	}
}

func TestEveryDomainAndValuesPolicyUnionBranch(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"kind":"binary"}`,
		`{"kind":"categorical","option_set":{"id":"options","version":1,"options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}}`,
		`{"kind":"ordinal","option_set":{"id":"options","version":1,"options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}}`,
		`{"kind":"numeric","values":{"kind":"continuous"}}`,
		`{"kind":"numeric","values":{"kind":"step","step":"0.1","origin":"0"}}`,
		`{"kind":"date","values":{"kind":"allowed_values","values":["2026-01-01"]}}`,
		`{"kind":"datetime","values":{"kind":"step","step":"PT1S","origin":"2026-01-01T00:00:00Z"}}`,
	}
	for _, input := range tests {
		var value Domain
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("decode %s: %v", input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, []byte(input), encoded)
	}
}

func TestEveryForecastRepresentationUnionBranch(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"kind":"probability","outcome":true,"probability":"0.5"}`,
		`{"kind":"pmf","option_set_ref":{"id":"set","version":1},"entries":[{"option_id":"a","probability":"1"}]}`,
		`{"kind":"binned_pmf","bin_set_ref":{"id":"bins","version":1},"entries":[{"bin_id":"a","probability":"1"}],"left_tail_probability":"0","right_tail_probability":"0"}`,
		`{"kind":"quantiles","interpolation":"none","points":[{"level":"0.5","value":"12.5"}]}`,
		`{"kind":"cdf","interpolation":"step_right","points":[{"value":"12.5","probability":"1"}],"left_tail_probability":"0","right_tail_probability":"0"}`,
		`{"kind":"point","statistic":"median","value":"12.5"}`,
		`{"kind":"credible_intervals","intervals":[{"coverage":"0.8","interval_kind":"equal_tailed","lower":"1","upper":"2"}]}`,
	}
	for _, input := range tests {
		var value ForecastRepresentation
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("decode %s: %v", input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, []byte(input), encoded)
	}
}

func TestOtherClosedUnionBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		target any
	}{
		{"group relationship", `{"id":"r","kind":"group_membership","group_id":"g","question_id":"q"}`, new(Relationship)},
		{"conditional relationship", `{"id":"r","kind":"conditional","parent_question_id":"p","parent_question_revision_id":"pr","parent_outcome":true,"child_question_id":"c"}`, new(Relationship)},
		{"resolved", `{"status":"resolved","question_revision_id":"qr","outcome":true,"outcome_known_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:01Z","sources":[]}`, new(Resolution)},
		{"unresolved", `{"status":"void","reason":"bad source","recorded_at":"2026-01-01T00:00:01Z"}`, new(Resolution)},
		{"not applicable", `{"status":"not_applicable","relationship_id":"r","reason":"condition false","recorded_at":"2026-01-01T00:00:01Z"}`, new(Resolution)},
		{"unanchored", `{"status":"unanchored"}`, new(Integrity)},
		{"pending", `{"status":"pending","target":{"scope":"forecast-envelope/v3","canonicalization":"RFC8785","artifact_path":"target.json","digest":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"timestamps":[{"type":"rfc3161","request_path":"request.tsq","response_path":"response.tsr","tsa_url":"https://example.org/tsa","hash_algorithm":"sha256","state":"pending"}]}`, new(Integrity)},
		{"verified", `{"status":"verified","target":{"scope":"forecast-envelope/v3","canonicalization":"RFC8785","artifact_path":"target.json","digest":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"timestamps":[{"type":"rfc3161","request_path":"request.tsq","response_path":"response.tsr","tsa_url":"https://example.org/tsa","hash_algorithm":"sha256","state":"verified","gen_time":"2026-01-01T00:00:00Z","policy_oid":"1.2.3","serial_number":"1","ca_bundle_path":"ca.pem"}],"verified_at":"2026-01-01T00:00:01Z"}`, new(Integrity)},
		{"failed", `{"status":"failed","failure_reason":"bad token"}`, new(Integrity)},
		{"sealed", `{"scheme":"forecast-seal/v3","commitment_hash":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"encryption":{"algorithm":"chacha20-poly1305","nonce":"zMzMzMzMzMzMzMzM","ciphertext":"AAAAAAAAAAAAAAAAAAAAAAAA"},"key_hint":"secret"}`, new(Commitment)},
		{"revealed", `{"scheme":"forecast-seal/v3","commitment_hash":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"encryption":{"algorithm":"chacha20-poly1305","nonce":"zMzMzMzMzMzMzMzM","ciphertext":"AAAAAAAAAAAAAAAAAAAAAAAA"},"key_hint":"secret","revealed_at":"2026-01-01T00:00:00Z","revealed_key":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`, new(Commitment)},
	}
	for _, test := range tests {
		if err := json.Unmarshal([]byte(test.input), test.target); err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		encoded, err := json.Marshal(test.target)
		if err != nil {
			t.Fatalf("%s marshal: %v", test.name, err)
		}
		assertJSONEqual(t, []byte(test.input), encoded)
	}
}

func TestOptionalCollectionsPreserveAbsentEmptyAndPopulatedStates(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"schema_version":"2.2.0","ledger_id":"test","created_at":"2026-01-01T00:00:00Z","default_timezone":"UTC","forecaster":{"id":"me","kind":"individual","name":"Me"},"platforms":{},"questions":[]}`,
		`{"schema_version":"2.2.0","ledger_id":"test","created_at":"2026-01-01T00:00:00Z","default_timezone":"UTC","forecaster":{"id":"me","kind":"individual","name":"Me"},"platforms":{},"groups":[],"relationships":[],"questions":[]}`,
		`{"schema_version":"2.2.0","ledger_id":"test","created_at":"2026-01-01T00:00:00Z","default_timezone":"UTC","forecaster":{"id":"team","kind":"team","name":"Team","members":[{"id":"a","name":"A"},{"id":"b","name":"B"}]},"platforms":{},"groups":[{"id":"g","title":"Group"}],"questions":[]}`,
	} {
		var model Ledger
		if err := json.Unmarshal([]byte(input), &model); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, []byte(input), encoded)
	}
}

func TestClosedUnionsRejectUnknownMissingAndAmbiguousVariants(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{}`,
		`{"kind":"future"}`,
		`{"kind":"binary","extra":true}`,
	} {
		var domain Domain
		if err := json.Unmarshal([]byte(input), &domain); err == nil {
			t.Fatalf("domain accepted %s", input)
		}
	}
	for _, input := range []string{
		`{}`,
		`{"kind":"future"}`,
		`{"kind":"point","statistic":"median","value":"1","extra":true}`,
	} {
		var representation ForecastRepresentation
		if err := json.Unmarshal([]byte(input), &representation); err == nil {
			t.Fatalf("representation accepted %s", input)
		}
	}
	_, err := json.Marshal(Domain{Binary: &BinaryDomain{Kind: OutcomeBinary}, Numeric: &NumericDomain{Kind: OutcomeNumeric}})
	if err == nil {
		t.Fatal("ambiguous domain state was marshalled")
	}
	_, err = json.Marshal(ForecastRepresentation{Probability: &ProbabilityRepresentation{}, Point: &PointRepresentation{}})
	if err == nil {
		t.Fatal("ambiguous representation state was marshalled")
	}
	ambiguous := []struct {
		name  string
		value any
	}{
		{"values", ValuesPolicy[Decimal, Decimal]{Continuous: &ContinuousValues{}, Allowed: &AllowedValues[Decimal]{}}},
		{"relationship", Relationship{GroupMembership: &GroupMembership{}, Conditional: &ConditionalRelationship{}}},
		{"resolution", Resolution{Resolved: &ResolvedResolution{}, Unresolved: &UnresolvedResolution{}}},
		{"integrity", Integrity{Unanchored: &UnanchoredIntegrity{}, Retained: &RetainedIntegrity{}}},
		{"lifecycle integrity", LifecycleIntegrity{Retained: &RetainedLifecycleIntegrity{}, Failed: &FailedLifecycleIntegrity{}}},
		{"commitment", Commitment{Sealed: &SealedCommitment{}, Revealed: &RevealedCommitment{}}},
		{"scalar", ScalarValue{Boolean: new(bool), String: new(string)}},
	}
	for _, test := range ambiguous {
		if _, err := json.Marshal(test.value); err == nil {
			t.Fatalf("ambiguous %s state was marshalled", test.name)
		}
	}

	closedCases := []struct {
		name   string
		input  string
		target any
	}{
		{"values", `{"kind":"continuous","extra":true}`, new(ValuesPolicy[Decimal, Decimal])},
		{"relationship", `{"id":"r","kind":"group_membership","group_id":"g","question_id":"q","extra":true}`, new(Relationship)},
		{"resolution", `{"status":"void","reason":"reason","recorded_at":"2026-01-01T00:00:00Z","extra":true}`, new(Resolution)},
		{"integrity", `{"status":"unanchored","extra":true}`, new(Integrity)},
		{"retained integrity", `{"status":"retained","target":{"scope":"forecast-envelope/v3","canonicalization":"RFC8785","artifact_path":"proofs/targets/f.json","digest":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"extra":true}`, new(Integrity)},
		{"retained lifecycle integrity", `{"status":"retained","target":{"scope":"forecast-lifecycle/v2","canonicalization":"RFC8785","artifact_path":"proofs/targets/f.lifecycle.e.json","digest":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"extra":true}`, new(LifecycleIntegrity)},
		{"commitment", `{"scheme":"forecast-seal/v3","commitment_hash":{"algorithm":"sha-256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"encryption":{"algorithm":"chacha20-poly1305","nonce":"zMzMzMzMzMzMzMzM","ciphertext":"AAAAAAAAAAAAAAAAAAAAAAAA"},"key_hint":"secret","extra":true}`, new(Commitment)},
	}
	for _, test := range closedCases {
		if err := json.Unmarshal([]byte(test.input), test.target); err == nil {
			t.Fatalf("%s union accepted an unknown property", test.name)
		}
	}

	missingCases := []struct {
		name   string
		target any
	}{
		{"values", new(ValuesPolicy[Decimal, Decimal])},
		{"relationship", new(Relationship)},
		{"resolution", new(Resolution)},
		{"integrity", new(Integrity)},
	}
	for _, test := range missingCases {
		if err := json.Unmarshal([]byte(`{}`), test.target); err == nil {
			t.Fatalf("%s union accepted a missing discriminator", test.name)
		}
	}
	unknownCases := []struct {
		name   string
		input  string
		target any
	}{
		{"values", `{"kind":"future"}`, new(ValuesPolicy[Decimal, Decimal])},
		{"relationship", `{"kind":"future"}`, new(Relationship)},
		{"resolution", `{"status":"future"}`, new(Resolution)},
		{"integrity", `{"status":"future"}`, new(Integrity)},
	}
	for _, test := range unknownCases {
		if err := json.Unmarshal([]byte(test.input), test.target); err == nil {
			t.Fatalf("%s union accepted an unknown discriminator", test.name)
		}
	}
}

func TestRetainedIntegrityUnionsDecodeCloneAndStayClosed(t *testing.T) {
	t.Parallel()

	var forecastLedger Ledger
	if err := json.Unmarshal(fixtureAsJSON(t, "tests/conformance/valid/retained-forecast.json"), &forecastLedger); err != nil {
		t.Fatal(err)
	}
	forecast := &forecastLedger.Questions[0].Forecasts[0]
	if forecast.Integrity.Retained == nil || forecast.Integrity.Retained.Status != IntegrityRetained || forecast.Integrity.Retained.Target.Scope != "forecast-envelope/v3" {
		t.Fatalf("retained forecast integrity = %#v", forecast.Integrity)
	}
	forecastClone, err := Clone(&forecastLedger)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(&forecastLedger, forecastClone) || forecastClone.Questions[0].Forecasts[0].Integrity.Retained == nil {
		t.Fatal("forecast retained state did not survive clone")
	}

	var lifecycleLedger Ledger
	if err := json.Unmarshal(fixtureAsJSON(t, "tests/conformance/valid/lifecycle-checkpoints.json"), &lifecycleLedger); err != nil {
		t.Fatal(err)
	}
	checkpoints := lifecycleLedger.Questions[0].Forecasts[0].ActivityCheckpoints
	if checkpoints == nil || len(*checkpoints) == 0 || (*checkpoints)[0].Integrity.Retained == nil || (*checkpoints)[0].Integrity.Retained.Target.Scope != "forecast-lifecycle/v2" {
		t.Fatalf("retained lifecycle integrity = %#v", checkpoints)
	}
	lifecycleClone, err := Clone(&lifecycleLedger)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(&lifecycleLedger, lifecycleClone) || (*lifecycleClone.Questions[0].Forecasts[0].ActivityCheckpoints)[0].Integrity.Retained == nil {
		t.Fatal("lifecycle retained state did not survive clone")
	}
}

func TestScalarValuePreservesBooleanAndExactString(t *testing.T) {
	t.Parallel()

	for _, input := range []string{`true`, `false`, `"0.123456789012345678"`, `"2026-01-01T00:00:00+01:00"`} {
		var value ScalarValue
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("decode %s: %v", input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != input {
			t.Fatalf("round trip %s = %s, %v", input, encoded, err)
		}
	}
	for _, input := range []string{`null`, `1`, `1.5`, `""`, `{}`, `[]`} {
		var value ScalarValue
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Fatalf("scalar accepted %s", input)
		}
	}
}

func TestRootObjectRejectsUnknownProperty(t *testing.T) {
	t.Parallel()
	var value Ledger
	if err := json.Unmarshal([]byte(`{"schema_version":"2.2.0","unknown":true}`), &value); err == nil {
		t.Fatal("ledger accepted an unknown property")
	}
}

func TestCloneSelectorsSummaryAndEqualityCoverV2State(t *testing.T) {
	t.Parallel()

	sourceBytes := fixtureAsJSON(t, "examples/valid/individual-ledger.json")
	var source Ledger
	if err := json.Unmarshal(sourceBytes, &source); err != nil {
		t.Fatal(err)
	}
	clone, err := Clone(&source)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(&source, clone) {
		t.Fatal("fresh clone is not equal")
	}
	counts := SummaryCounts(clone)
	if counts.Groups != 1 || counts.Relationships != 1 || counts.Questions != 4 || counts.Revisions != 5 || counts.Forecasts != 5 || counts.Representations != 10 || counts.LifecycleEvents != 2 {
		t.Fatalf("summary counts = %#v", counts)
	}
	question := &clone.Questions[1]
	revision, ok := question.CurrentRevision()
	if !ok || revision.ID != "qr-election-coalition-2" {
		t.Fatalf("current revision = %#v, %v", revision, ok)
	}
	forecast, ok := question.Forecast("f-election-coalition-002")
	if !ok || forecast.QuestionRevisionID != revision.ID {
		t.Fatalf("forecast selector = %#v, %v", forecast, ok)
	}
	if kind, ok := revision.Domain.Kind(); !ok || kind != OutcomeCategorical {
		t.Fatalf("domain kind = %q, %v", kind, ok)
	}
	if kind, ok := (*forecast.Representations)[0].Kind(); !ok || kind != RepresentationPMF {
		t.Fatalf("representation kind = %q, %v", kind, ok)
	}

	(*forecast.Representations)[0].PMF.Entries[0].Probability = "0.420"
	if Equal(&source, clone) {
		t.Fatal("exact scalar string change was ignored")
	}
	if (*source.Questions[1].Forecasts[1].Representations)[0].PMF.Entries[0].Probability != "0.42" {
		t.Fatal("clone shares nested representation storage with source")
	}
}

func fixtureAsJSON(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(contractschema.Conformance(), name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".yaml") {
		return data
	}
	parsed, err := document.ParseYAML(bytes.NewReader(data), document.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(parsed.Root.Any())
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertJSONEqual(t *testing.T, left, right []byte) {
	t.Helper()
	var leftValue, rightValue any
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	leftDecoder.UseNumber()
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	rightDecoder.UseNumber()
	if err := leftDecoder.Decode(&leftValue); err != nil {
		t.Fatal(err)
	}
	if err := rightDecoder.Decode(&rightValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leftValue, rightValue) {
		t.Fatalf("JSON differs\nleft:  %s\nright: %s", left, right)
	}
}

func assertNoJSONFloats(t *testing.T, data []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(current any) {
		switch current := current.(type) {
		case json.Number:
			if strings.ContainsAny(current.String(), ".eE") {
				t.Errorf("JSON float introduced: %s", current)
			}
		case []any:
			for _, item := range current {
				walk(item)
			}
		case map[string]any:
			for _, item := range current {
				walk(item)
			}
		}
	}
	walk(value)
}
