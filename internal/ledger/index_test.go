package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestBuildIndexCreatesV2StableLookups(t *testing.T) {
	first := Slug("f-first")
	groups := []Group{{ID: "group-one"}}
	relationships := []Relationship{{GroupMembership: &GroupMembership{ID: "membership-one", Kind: RelationshipGroupMembership}}}
	model := &Ledger{
		Platforms:     map[Slug]Platform{"zeta": {}, "alpha": {}},
		Groups:        &groups,
		Relationships: &relationships,
		Questions: []Question{
			{
				ID: "q-two", CurrentRevisionID: "qr-two", Revisions: []QuestionRevision{{ID: "qr-two", Provenance: &Provenance{Platform: "zeta"}}},
				Forecasts: []Forecast{{ID: first, Provenance: &Provenance{Platform: "alpha"}}, {ID: "f-second", SupersedesForecastID: &first}},
			},
			{ID: "q-one", CurrentRevisionID: "qr-one", Revisions: []QuestionRevision{{ID: "qr-one"}}, Forecasts: []Forecast{{ID: "f-third"}}},
		},
	}
	index, err := BuildIndex(model)
	if err != nil {
		t.Fatal(err)
	}
	if got := index.PlatformQuestionIDs["zeta"]; !reflect.DeepEqual(got, []Slug{"q-two"}) {
		t.Fatalf("platform question references = %#v", got)
	}
	if got := index.PlatformForecastIDs["alpha"]; !reflect.DeepEqual(got, []Slug{"f-first"}) {
		t.Fatalf("platform forecast references = %#v", got)
	}
	if got := index.QuestionForecastIDs["q-two"]; !reflect.DeepEqual(got, []Slug{"f-first", "f-second"}) {
		t.Fatalf("question order = %#v", got)
	}
	if revision, ok := index.Revision("q-two", "qr-two"); !ok || revision.RevisionIndex != 0 {
		t.Fatalf("revision location = %#v, %v", revision, ok)
	}
	location, ok := index.Forecast("f-third")
	if !ok || location.QuestionID != "q-one" || location.ForecastIndex != 0 {
		t.Fatalf("forecast location = %#v, %v", location, ok)
	}
	if index.Supersedes["f-second"] != "f-first" {
		t.Fatalf("supersession index = %#v", index.Supersedes)
	}
}

func TestBuildIndexRejectsInvalidV2IdentityAndLinks(t *testing.T) {
	earlier := Slug("f-earlier")
	question := func(id Slug, forecasts ...Forecast) Question {
		revisionID := Slug(string(id) + "-revision")
		return Question{ID: id, CurrentRevisionID: revisionID, Revisions: []QuestionRevision{{ID: revisionID}}, Forecasts: forecasts}
	}
	tests := []struct {
		name  string
		model *Ledger
		code  IndexErrorCode
	}{
		{name: "duplicate question", model: &Ledger{Questions: []Question{question("q"), question("q")}}, code: IndexDuplicateQuestion},
		{name: "duplicate revision", model: &Ledger{Questions: []Question{{ID: "q", CurrentRevisionID: "r", Revisions: []QuestionRevision{{ID: "r"}, {ID: "r"}}}}}, code: IndexDuplicateRevision},
		{name: "unknown current revision", model: &Ledger{Questions: []Question{{ID: "q", CurrentRevisionID: "missing", Revisions: []QuestionRevision{{ID: "r"}}}}}, code: IndexUnknownCurrentRevision},
		{name: "global duplicate forecast", model: &Ledger{Questions: []Question{question("q-a", Forecast{ID: "f"}), question("q-b", Forecast{ID: "f"})}}, code: IndexDuplicateForecast},
		{name: "unknown superseded", model: &Ledger{Questions: []Question{question("q", Forecast{ID: "f", SupersedesForecastID: &earlier})}}, code: IndexUnknownSuperseded},
		{name: "forward superseded", model: &Ledger{Questions: []Question{question("q", Forecast{ID: "f", SupersedesForecastID: &earlier}, Forecast{ID: earlier})}}, code: IndexForwardLink},
		{name: "cross question", model: &Ledger{Questions: []Question{question("q-a", Forecast{ID: earlier}), question("q-b", Forecast{ID: "f", SupersedesForecastID: &earlier})}}, code: IndexCrossQuestionLink},
		{name: "duplicate group", model: &Ledger{Groups: &[]Group{{ID: "g"}, {ID: "g"}}}, code: IndexDuplicateGroup},
		{name: "duplicate relationship", model: &Ledger{Relationships: &[]Relationship{{GroupMembership: &GroupMembership{ID: "r"}}, {Conditional: &ConditionalRelationship{ID: "r"}}}}, code: IndexDuplicateRelationship},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildIndex(test.model)
			var indexErr *IndexError
			if !errors.As(err, &indexErr) || indexErr.Code != test.code {
				t.Fatalf("error = %#v, want %q", err, test.code)
			}
		})
	}
}

func FuzzBuildIndexSelectors(f *testing.F) {
	f.Add("q-one", "r-one", "f-one", "f-two")
	f.Fuzz(func(t *testing.T, questionText, revisionText, firstText, secondText string) {
		questionID := Slug(questionText)
		revisionID := Slug(revisionText)
		firstID := Slug(firstText)
		secondID := Slug(secondText)
		model := &Ledger{Questions: []Question{{
			ID: questionID, CurrentRevisionID: revisionID, Revisions: []QuestionRevision{{ID: revisionID}},
			Forecasts: []Forecast{{ID: firstID}, {ID: secondID}},
		}}}
		index, err := BuildIndex(model)
		if firstID == secondID {
			if err == nil {
				t.Fatal("duplicate forecast IDs were accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if position, ok := index.Question(questionID); !ok || position != 0 {
			t.Fatalf("question lookup = %d, %v", position, ok)
		}
		if _, ok := index.Revision(questionID, revisionID); !ok {
			t.Fatal("revision lookup failed")
		}
		if location, ok := index.Forecast(secondID); !ok || location.ForecastIndex != 1 {
			t.Fatalf("forecast lookup = %#v, %v", location, ok)
		}
	})
}
