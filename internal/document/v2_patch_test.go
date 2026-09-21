package document

import (
	"strings"
	"testing"
)

func TestApplyPatchAppendsNestedV2RevisionBeforeSibling(t *testing.T) {
	input := "questions:\n  - id: q-one\n    current_revision_id: qr-one\n    revisions:\n      - id: qr-one\n        domain:\n          kind: binary\n    forecasts: []\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	value, err := ParseJSON(strings.NewReader(`{"id":"qr-two","domain":{"kind":"binary"}}`), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(doc, []PatchOperation{{Kind: PatchAdd, Pointer: "/questions/0/revisions/-", Value: Ordered(value.Root)}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "      - id: qr-two\n") {
		t.Fatalf("revision indentation is wrong:\n%s", got)
	}
}
