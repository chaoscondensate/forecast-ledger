package document

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestParseYAMLRetainsSourceTreeAndPresentation(t *testing.T) {
	input := "# ledger\r\nname: 'example' # inline\r\nitems: [one, two]\r\ntext: |\r\n  line one\r\n  line two\r\n"
	doc, err := ParseYAML(strings.NewReader(input), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if string(doc.Raw) != input {
		t.Fatal("raw YAML was not retained exactly")
	}
	if doc.YAMLRoot == nil || doc.YAMLRoot.HeadComment != "# ledger" {
		t.Fatalf("head comment not retained: %#v", doc.YAMLRoot)
	}
	root := doc.YAMLRoot.Content[0]
	if root.Content[1].Style != yaml.SingleQuotedStyle {
		t.Fatalf("single-quoted style not retained: %v", root.Content[1].Style)
	}
	if root.Content[3].Style != yaml.FlowStyle {
		t.Fatalf("flow style not retained: %v", root.Content[3].Style)
	}
	if root.Content[5].Style != yaml.LiteralStyle {
		t.Fatalf("literal style not retained: %v", root.Content[5].Style)
	}
	if doc.Newlines.CRLF != 6 || !doc.Newlines.FinalNewline {
		t.Fatalf("unexpected newlines: %#v", doc.Newlines)
	}
}

func TestParseYAMLRejectsDuplicateAndNonStringKeys(t *testing.T) {
	_, err := ParseYAML(strings.NewReader("a: 1\n\"a\": 2\n"), DefaultLimits)
	parseErr := requireParseCode(t, err, "document.duplicate_key")
	if parseErr.Diagnostic.Location.Pointer != "/a" || len(parseErr.Diagnostic.Related) != 1 {
		t.Fatalf("unexpected duplicate diagnostic: %#v", parseErr.Diagnostic)
	}

	_, err = ParseYAML(strings.NewReader("1: value\n"), DefaultLimits)
	requireParseCode(t, err, "document.non_string_key")
}

func TestParseYAMLRejectsFloatsUnsafeIntegersAndTags(t *testing.T) {
	for _, input := range []string{"value: 1.0\n", "value: .nan\n", "value: .inf\n"} {
		_, err := ParseYAML(strings.NewReader(input), DefaultLimits)
		requireParseCode(t, err, "document.float_not_allowed")
	}
	if _, err := ParseYAML(strings.NewReader("value: !!str 1.0\n"), DefaultLimits); err != nil {
		t.Fatalf("explicit string rejected: %v", err)
	}
	for _, input := range []string{
		"value: 9007199254740992\n",
		"value: -9007199254740992\n",
		"value: 0x20000000000000\n",
	} {
		_, err := ParseYAML(strings.NewReader(input), DefaultLimits)
		requireParseCode(t, err, "document.unsafe_integer")
	}
	if _, err := ParseYAML(strings.NewReader("value: 0x10\n"), DefaultLimits); err != nil {
		t.Fatalf("safe hexadecimal integer rejected: %v", err)
	}

	for _, input := range []string{
		"value: 2026-08-25\n",
		"value: !!binary SGVsbG8=\n",
		"value: !private secret\n",
	} {
		_, err := ParseYAML(strings.NewReader(input), DefaultLimits)
		requireParseCode(t, err, "document.unsupported_tag")
	}
}

func TestParseYAMLAliasLimitsCyclesAndMerge(t *testing.T) {
	valid := "source: &answer 42\ncopy: *answer\n"
	doc, err := ParseYAML(strings.NewReader(valid), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Root.Object[1].Value.Int; got != 42 {
		t.Fatalf("alias value = %d, want 42", got)
	}

	limits := DefaultLimits
	limits.MaxAliases = 1
	_, err = ParseYAML(strings.NewReader("source: &a 1\none: *a\ntwo: *a\n"), limits)
	requireParseCode(t, err, "document.alias_limit")

	_, err = ParseYAML(strings.NewReader("cycle: &a [*a]\n"), DefaultLimits)
	requireParseCode(t, err, "document.alias_cycle")

	_, err = ParseYAML(strings.NewReader("base: &b {a: 1}\nvalue:\n  <<: *b\n"), DefaultLimits)
	requireParseCode(t, err, "document.merge_key_not_allowed")
}

func TestParseYAMLRejectsMultipleDocumentsAndHonorsLimits(t *testing.T) {
	_, err := ParseYAML(strings.NewReader("a: 1\n---\nb: 2\n"), DefaultLimits)
	requireParseCode(t, err, "document.multiple_documents")

	limits := DefaultLimits
	limits.MaxDepth = 2
	if _, err := ParseYAML(strings.NewReader("a: 1\n"), limits); err != nil {
		t.Fatal(err)
	}
	_, err = ParseYAML(strings.NewReader("a:\n  b: 1\n"), limits)
	requireParseCode(t, err, "document.too_deep")

	limits = DefaultLimits
	limits.MaxExpandedNodes = 3
	_, err = ParseYAML(strings.NewReader("source: &a [1, 2]\ncopy: *a\n"), limits)
	requireParseCode(t, err, "document.alias_limit")
}

func TestParseYAMLUpstreamFixtureAndJSONSemanticParity(t *testing.T) {
	yamlData, err := os.ReadFile(filepath.Join("..", "schema", "upstream", "forecast-ledger", "v2.0.1", "examples", "valid", "team-ledger.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYAML(strings.NewReader(string(yamlData)), DefaultLimits); err != nil {
		t.Fatalf("parse upstream YAML fixture: %v", err)
	}

	jsonDoc, err := ParseJSON(strings.NewReader(`{"a":[1,true,null,"x"]}`), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	yamlDoc, err := ParseYAML(strings.NewReader("a: [1, true, null, x]\n"), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonDoc.Root.Any(), yamlDoc.Root.Any()) {
		t.Fatalf("semantic trees differ:\nJSON: %#v\nYAML: %#v", jsonDoc.Root.Any(), yamlDoc.Root.Any())
	}
}

func TestYAMLLocationAfterMultibyteText(t *testing.T) {
	doc, err := ParseYAML(strings.NewReader("прогноз: ok\nnext: value\n"), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	location := doc.Locations["/next"][0]
	if location.Start.Line != 2 || location.Start.Column != 7 {
		t.Fatalf("location = %#v, want line 2 column 7", location.Start)
	}
}
