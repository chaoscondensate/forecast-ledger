package document

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chaoscondensate/forecast-ledger/internal/canonical"
	"go.yaml.in/yaml/v3"
)

var ErrUnsupportedPatch = errors.New("source-preserving patch is not supported")

type ScalarEdit struct {
	Pointer string
	Value   any
}

type byteEdit struct {
	start       int64
	end         int64
	replacement []byte
}

// ReplaceScalars replaces existing scalar values by RFC 6901 pointer. It
// splices only the selected source ranges, retaining all untouched bytes.
func ReplaceScalars(sourceDocument *Document, edits []ScalarEdit) ([]byte, error) {
	if sourceDocument == nil || sourceDocument.Root == nil {
		return nil, errors.New("document has no source tree")
	}
	seen := make(map[string]struct{}, len(edits))
	patches := make([]byteEdit, 0, len(edits))
	for _, edit := range edits {
		if _, exists := seen[edit.Pointer]; exists {
			return nil, fmt.Errorf("%w: pointer %q is edited more than once", ErrUnsupportedPatch, edit.Pointer)
		}
		seen[edit.Pointer] = struct{}{}
		semanticNode, err := lookupValue(sourceDocument.Root, edit.Pointer)
		if err != nil {
			return nil, err
		}
		if semanticNode.Kind == ValueArray || semanticNode.Kind == ValueObject {
			return nil, fmt.Errorf("%w: %q is not a scalar", ErrUnsupportedPatch, edit.Pointer)
		}
		var patch byteEdit
		switch sourceDocument.Format {
		case FormatJSON:
			if semanticNode.Source.End == nil {
				return nil, fmt.Errorf("%w: JSON source range is incomplete", ErrUnsupportedPatch)
			}
			replacement, err := canonical.Marshal(edit.Value)
			if err != nil {
				return nil, fmt.Errorf("encode JSON replacement: %w", err)
			}
			if !isScalarReplacement(edit.Value) {
				return nil, fmt.Errorf("%w: replacement for %q is not a scalar", ErrUnsupportedPatch, edit.Pointer)
			}
			patch = byteEdit{start: semanticNode.Source.Start.Offset, end: semanticNode.Source.End.Offset, replacement: replacement}
		case FormatYAML:
			yamlNode, err := lookupYAMLNode(sourceDocument.YAMLRoot, edit.Pointer)
			if err != nil {
				return nil, err
			}
			if yamlNode.Kind == yaml.AliasNode {
				return nil, fmt.Errorf("%w: alias nodes must be changed at their anchor", ErrUnsupportedPatch)
			}
			if yamlNode.Kind != yaml.ScalarNode || !isScalarReplacement(edit.Value) {
				return nil, fmt.Errorf("%w: replacement for %q is not a scalar", ErrUnsupportedPatch, edit.Pointer)
			}
			start := yamlOffset(sourceDocument.Raw, sourceDocument.lineIndex, yamlNode.Line, yamlNode.Column)
			end, err := yamlScalarEnd(sourceDocument.Raw, start, yamlNode)
			if err != nil {
				return nil, err
			}
			replacement, err := renderYAMLScalar(sourceDocument.Raw, start, yamlNode, edit.Value, sourceDocument.Newlines)
			if err != nil {
				return nil, err
			}
			patch = byteEdit{start: start, end: end, replacement: replacement}
		default:
			return nil, fmt.Errorf("%w: unknown document format", ErrUnsupportedPatch)
		}
		patches = append(patches, patch)
	}

	sort.Slice(patches, func(i, j int) bool { return patches[i].start > patches[j].start })
	output := bytes.Clone(sourceDocument.Raw)
	previousStart := int64(len(output))
	for _, patch := range patches {
		if patch.start < 0 || patch.end < patch.start || patch.end > int64(len(output)) || patch.end > previousStart {
			return nil, fmt.Errorf("%w: source ranges overlap or are invalid", ErrUnsupportedPatch)
		}
		output = append(output[:patch.start], append(patch.replacement, output[patch.end:]...)...)
		previousStart = patch.start
	}
	if err := verifyPatchedDocument(output, sourceDocument.Format); err != nil {
		return nil, fmt.Errorf("patched document is invalid: %w", err)
	}
	return output, nil
}

func lookupValue(root *Value, pointer string) (*Value, error) {
	current := root
	for _, token := range pointerTokens(pointer) {
		switch current.Kind {
		case ValueObject:
			found := false
			for _, member := range current.Object {
				if member.Key == token {
					current, found = member.Value, true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("source pointer %q does not exist", pointer)
			}
		case ValueArray:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(current.Array) {
				return nil, fmt.Errorf("source pointer %q does not exist", pointer)
			}
			current = current.Array[index]
		default:
			return nil, fmt.Errorf("source pointer %q does not exist", pointer)
		}
	}
	return current, nil
}

func lookupYAMLNode(documentNode *yaml.Node, pointer string) (*yaml.Node, error) {
	if documentNode == nil || documentNode.Kind != yaml.DocumentNode || len(documentNode.Content) != 1 {
		return nil, errors.New("YAML source tree has no root")
	}
	current := documentNode.Content[0]
	for _, token := range pointerTokens(pointer) {
		for current.Kind == yaml.AliasNode && current.Alias != nil {
			current = current.Alias
		}
		switch current.Kind {
		case yaml.MappingNode:
			found := false
			for index := 0; index < len(current.Content); index += 2 {
				if current.Content[index].Value == token {
					current, found = current.Content[index+1], true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("source pointer %q does not exist", pointer)
			}
		case yaml.SequenceNode:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(current.Content) {
				return nil, fmt.Errorf("source pointer %q does not exist", pointer)
			}
			current = current.Content[index]
		default:
			return nil, fmt.Errorf("source pointer %q does not exist", pointer)
		}
	}
	return current, nil
}

func pointerTokens(pointer string) []string {
	if pointer == "" {
		return nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return []string{"\x00invalid-pointer"}
	}
	tokens := strings.Split(pointer[1:], "/")
	for index, token := range tokens {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		tokens[index] = token
	}
	return tokens
}

func isScalarReplacement(value any) bool {
	_, ok := scalarReplacementValue(value)
	return ok
}

func scalarReplacementValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, string:
		return typed, true
	case OrderedValue:
		if typed.value == nil {
			return nil, true
		}
		switch typed.value.Kind {
		case ValueNull, ValueBool, ValueInt, ValueString:
			return typed.value.Any(), true
		}
	}
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() {
		switch reflected.Kind() {
		case reflect.Bool:
			return reflected.Bool(), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflected.Int(), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflected.Uint(), true
		case reflect.String:
			return reflected.String(), true
		}
	}
	return nil, false
}

func yamlScalarEnd(raw []byte, start int64, node *yaml.Node) (int64, error) {
	if start < 0 || start >= int64(len(raw)) {
		return 0, fmt.Errorf("%w: YAML scalar start is outside the source", ErrUnsupportedPatch)
	}
	switch node.Style {
	case yaml.SingleQuotedStyle:
		for offset := start + 1; offset < int64(len(raw)); offset++ {
			if raw[offset] == '\'' {
				if offset+1 < int64(len(raw)) && raw[offset+1] == '\'' {
					offset++
					continue
				}
				return offset + 1, nil
			}
		}
	case yaml.DoubleQuotedStyle:
		escaped := false
		for offset := start + 1; offset < int64(len(raw)); offset++ {
			if escaped {
				escaped = false
				continue
			}
			if raw[offset] == '\\' {
				escaped = true
				continue
			}
			if raw[offset] == '"' {
				return offset + 1, nil
			}
		}
	case yaml.LiteralStyle, yaml.FoldedStyle:
		return yamlBlockEnd(raw, start)
	default:
		if strings.Contains(node.Value, "\n") {
			return 0, fmt.Errorf("%w: multiline plain YAML scalar", ErrUnsupportedPatch)
		}
		for offset := start; offset < int64(len(raw)); offset++ {
			switch raw[offset] {
			case '\r', '\n', ',', ']', '}':
				return trimYAMLSuffix(raw, start, offset), nil
			case '#':
				if offset == start || raw[offset-1] == ' ' || raw[offset-1] == '\t' {
					return trimYAMLSuffix(raw, start, offset), nil
				}
			}
		}
		return trimYAMLSuffix(raw, start, int64(len(raw))), nil
	}
	return 0, fmt.Errorf("%w: unterminated YAML scalar", ErrUnsupportedPatch)
}

func trimYAMLSuffix(raw []byte, start, end int64) int64 {
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\t') {
		end--
	}
	return end
}

func yamlBlockEnd(raw []byte, start int64) (int64, error) {
	headerEnd := lineEnd(raw, start)
	lineStart := skipNewline(raw, headerEnd)
	contentIndent := -1
	for cursor := lineStart; cursor < int64(len(raw)); {
		end := lineEnd(raw, cursor)
		indent, blank := lineIndent(raw[cursor:end])
		if !blank {
			contentIndent = indent
			break
		}
		cursor = skipNewline(raw, end)
	}
	if contentIndent < 0 {
		return headerEnd, nil
	}
	cursor := lineStart
	for cursor < int64(len(raw)) {
		end := lineEnd(raw, cursor)
		indent, blank := lineIndent(raw[cursor:end])
		if !blank && indent < contentIndent {
			return cursor, nil
		}
		cursor = skipNewline(raw, end)
	}
	return int64(len(raw)), nil
}

func renderYAMLScalar(raw []byte, start int64, node *yaml.Node, value any, newlines NewlineInfo) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		return []byte(strconv.FormatBool(value)), nil
	case int:
		return []byte(strconv.FormatInt(int64(value), 10)), nil
	case int8:
		return []byte(strconv.FormatInt(int64(value), 10)), nil
	case int16:
		return []byte(strconv.FormatInt(int64(value), 10)), nil
	case int32:
		return []byte(strconv.FormatInt(int64(value), 10)), nil
	case int64:
		return []byte(strconv.FormatInt(value, 10)), nil
	case uint:
		return []byte(strconv.FormatUint(uint64(value), 10)), nil
	case uint8:
		return []byte(strconv.FormatUint(uint64(value), 10)), nil
	case uint16:
		return []byte(strconv.FormatUint(uint64(value), 10)), nil
	case uint32:
		return []byte(strconv.FormatUint(uint64(value), 10)), nil
	case uint64:
		return []byte(strconv.FormatUint(value, 10)), nil
	case string:
		if !utf8.ValidString(value) {
			return nil, fmt.Errorf("%w: replacement string is not valid UTF-8", ErrUnsupportedPatch)
		}
		switch node.Style {
		case yaml.SingleQuotedStyle:
			return []byte("'" + strings.ReplaceAll(value, "'", "''") + "'"), nil
		case yaml.LiteralStyle, yaml.FoldedStyle:
			return renderYAMLBlock(raw, start, value, newlines), nil
		case yaml.DoubleQuotedStyle:
			return jsontext.AppendQuote(nil, value)
		default:
			if safePlainYAML(value) {
				return []byte(value), nil
			}
			return jsontext.AppendQuote(nil, value)
		}
	default:
		return nil, fmt.Errorf("%w: replacement is not a scalar", ErrUnsupportedPatch)
	}
}

func renderYAMLBlock(raw []byte, start int64, value string, newlines NewlineInfo) []byte {
	newline := "\n"
	if newlines.CRLF > newlines.LF {
		newline = "\r\n"
	}
	lineStart := start
	for lineStart > 0 && raw[lineStart-1] != '\n' && raw[lineStart-1] != '\r' {
		lineStart--
	}
	indent := "  "
	for cursor := lineStart; cursor < start && (raw[cursor] == ' ' || raw[cursor] == '\t'); cursor++ {
		indent += string(raw[cursor])
	}
	chomp := "|-"
	if strings.HasSuffix(value, "\n") {
		chomp = "|"
		value = strings.TrimSuffix(value, "\n")
	}
	var output strings.Builder
	output.WriteString(chomp)
	output.WriteString(newline)
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		output.WriteString(indent)
		output.WriteString(line)
		if index < len(lines)-1 || value != "" {
			output.WriteString(newline)
		}
	}
	return []byte(output.String())
}

func safePlainYAML(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n:#[]{},&*!|>'\"%@`") {
		return false
	}
	lower := strings.ToLower(value)
	if lower == "null" || lower == "true" || lower == "false" || lower == "~" {
		return false
	}
	return true
}

func lineEnd(raw []byte, start int64) int64 {
	for start < int64(len(raw)) && raw[start] != '\r' && raw[start] != '\n' {
		start++
	}
	return start
}

func skipNewline(raw []byte, offset int64) int64 {
	if offset < int64(len(raw)) && raw[offset] == '\r' {
		offset++
	}
	if offset < int64(len(raw)) && raw[offset] == '\n' {
		offset++
	}
	return offset
}

func lineIndent(line []byte) (int, bool) {
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}
	return indent, indent == len(line)
}

func verifyPatchedDocument(raw []byte, format Format) error {
	var err error
	switch format {
	case FormatJSON:
		_, err = ParseJSON(bytes.NewReader(raw), DefaultLimits)
	case FormatYAML:
		_, err = ParseYAML(bytes.NewReader(raw), DefaultLimits)
	default:
		err = errors.New("unknown document format")
	}
	return err
}
