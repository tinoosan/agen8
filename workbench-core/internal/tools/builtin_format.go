package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinFormatManifest = []byte(`{"id":"builtin.format","version":"0.1.0","kind":"builtin","displayName":"Builtin Format","description":"Formats text payloads deterministically so agents can write readable code/config/markup.","actions":[{"id":"json.pretty","displayName":"Pretty JSON","description":"Pretty-print JSON text with stable indentation; optional key sorting.","inputSchema":{"type":"object","properties":{"text":{"type":"string"},"indent":{"type":"integer"},"sortKeys":{"type":"boolean"}},"required":["text"]},"outputSchema":{"type":"object","properties":{"text":{"type":"string"},"changed":{"type":"boolean"}},"required":["text","changed"]}},{"id":"html.pretty","displayName":"Pretty HTML","description":"Best-effort HTML formatter for readability (indentation + line breaks).","inputSchema":{"type":"object","properties":{"text":{"type":"string"},"indent":{"type":"integer"}},"required":["text"]},"outputSchema":{"type":"object","properties":{"text":{"type":"string"},"changed":{"type":"boolean"},"warning":{"type":"string"}},"required":["text","changed"]}}]}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.format"),
		Manifest: builtinFormatManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			_ = cfg
			return BuiltinFormatInvoker{}
		},
	})
}

// BuiltinFormatInvoker implements builtin.format.
//
// Why this exists:
//   - Agents often generate minified/one-line JSON or HTML (especially when content
//     comes from tools like curl).
//   - Readability matters for humans reviewing /workspace outputs and for agents
//     that may need to re-open files later.
//   - Formatting should be deterministic and not depend on optional external binaries.
//
// This tool is pure computation: it does not access the filesystem and does not
// create artifacts.
type BuiltinFormatInvoker struct{}

type formatJSONInput struct {
	Text     string `json:"text"`
	Indent   int    `json:"indent,omitempty"`
	SortKeys bool   `json:"sortKeys,omitempty"`
}

type formatJSONOutput struct {
	Text    string `json:"text"`
	Changed bool   `json:"changed"`
}

type formatHTMLInput struct {
	Text   string `json:"text"`
	Indent int    `json:"indent,omitempty"`
}

type formatHTMLOutput struct {
	Text    string `json:"text"`
	Changed bool   `json:"changed"`
	Warning string `json:"warning,omitempty"`
}

func (BuiltinFormatInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	_ = ctx

	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	switch req.ActionID {
	case "json.pretty":
		return formatJSON(req)
	case "html.pretty":
		return formatHTML(req)
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: json.pretty, html.pretty)", req.ActionID)}
	}
}

func formatJSON(req types.ToolRequest) (ToolCallResult, error) {
	var in formatJSONInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	raw := []byte(in.Text)
	if !json.Valid(raw) {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "text must be valid JSON"}
	}

	indent := in.Indent
	if indent == 0 {
		indent = 2
	}
	if indent < 0 || indent > 8 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "indent must be between 0 and 8"}
	}
	indentStr := strings.Repeat(" ", indent)

	var outBytes []byte
	if in.SortKeys {
		sorted, err := sortJSONKeys(raw)
		if err != nil {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error(), Err: err}
		}
		raw = sorted
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", indentStr); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("json indent failed: %v", err), Err: err}
	}
	outBytes = buf.Bytes()

	// Ensure a trailing newline for editor friendliness.
	outText := string(outBytes)
	if !strings.HasSuffix(outText, "\n") {
		outText += "\n"
	}

	resp := formatJSONOutput{
		Text:    outText,
		Changed: outText != in.Text,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: b}, nil
}

// sortJSONKeys canonicalizes a JSON document by sorting object keys recursively.
//
// This is best-effort and intentionally limited to standard JSON types.
func sortJSONKeys(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	ordered := toOrdered(v)
	return marshalOrdered(ordered)
}

type orderedObject struct {
	Pairs []orderedPair
}

type orderedPair struct {
	Key   string
	Value any
}

func toOrdered(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]orderedPair, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, orderedPair{Key: k, Value: toOrdered(x[k])})
		}
		return orderedObject{Pairs: pairs}
	case []any:
		out := make([]any, 0, len(x))
		for _, it := range x {
			out = append(out, toOrdered(it))
		}
		return out
	default:
		return v
	}
}

func marshalOrdered(v any) ([]byte, error) {
	switch x := v.(type) {
	case orderedObject:
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, p := range x.Pairs {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(p.Key)
			buf.Write(kb)
			buf.WriteByte(':')
			vb, err := marshalOrdered(p.Value)
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, it := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			vb, err := marshalOrdered(it)
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case json.Number:
		return []byte(x.String()), nil
	default:
		return json.Marshal(x)
	}
}

func formatHTML(req types.ToolRequest) (ToolCallResult, error) {
	var in formatHTMLInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	indent := in.Indent
	if indent == 0 {
		indent = 2
	}
	if indent < 0 || indent > 8 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "indent must be between 0 and 8"}
	}

	outText := prettyHTML(in.Text, indent)
	resp := formatHTMLOutput{
		Text:    outText,
		Changed: outText != in.Text,
		Warning: "best-effort formatter; may not preserve all whitespace-sensitive semantics (avoid for <pre>/<textarea>/<script>/<style> if exact whitespace matters)",
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: b}, nil
}

// prettyHTML is a conservative, best-effort HTML formatter.
//
// It is NOT a full HTML parser. It is designed for readability of typical minified HTML
// (e.g. responses from curl), and tries to avoid breaking raw-text tags by keeping
// their contents untouched.
func prettyHTML(s string, indent int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	indentStr := strings.Repeat(" ", indent)
	var out strings.Builder
	depth := 0
	lastNewline := false

	// Raw-text elements where we should not reformat inner content.
	rawTag := ""

	writeLine := func(d int, line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if out.Len() > 0 && !lastNewline {
			out.WriteString("\n")
			lastNewline = true
		}
		out.WriteString(strings.Repeat(indentStr, d))
		out.WriteString(line)
		out.WriteString("\n")
		lastNewline = true
	}

	i := 0
	for i < len(s) {
		if rawTag != "" {
			// Find closing tag for rawTag.
			lower := strings.ToLower(s[i:])
			needle := "</" + rawTag
			j := strings.Index(lower, needle)
			if j == -1 {
				// No closing tag; emit remainder and stop.
				writeLine(depth, s[i:])
				break
			}
			// Emit raw content verbatim (no trimming) as its own block.
			raw := s[i : i+j]
			if strings.TrimSpace(raw) != "" {
				out.WriteString(raw)
				lastNewline = strings.HasSuffix(raw, "\n")
				if !lastNewline {
					out.WriteString("\n")
					lastNewline = true
				}
			}
			i = i + j
			rawTag = ""
			continue
		}

		lt := strings.IndexByte(s[i:], '<')
		if lt == -1 {
			// Remaining text.
			text := s[i:]
			writeLine(depth, text)
			break
		}
		lt = i + lt

		// Text before the tag.
		if lt > i {
			text := s[i:lt]
			// Avoid outputting pure whitespace between tags.
			if strings.TrimFunc(text, unicode.IsSpace) != "" {
				writeLine(depth, text)
			}
		}

		gt := strings.IndexByte(s[lt:], '>')
		if gt == -1 {
			writeLine(depth, s[lt:])
			break
		}
		gt = lt + gt

		tag := strings.TrimSpace(s[lt : gt+1])
		name, isClose, isSelf := classifyHTMLTag(tag)

		if isClose {
			if depth > 0 {
				depth--
			}
			writeLine(depth, tag)
		} else {
			writeLine(depth, tag)
			if name != "" && isRawTextTag(name) {
				rawTag = name
			}
			if !isSelf && name != "" && !isVoidElement(name) {
				depth++
			}
		}

		i = gt + 1
	}

	return out.String()
}

func classifyHTMLTag(tag string) (name string, isClose bool, isSelfClosing bool) {
	// Strip < and >
	t := strings.TrimSpace(tag)
	if strings.HasPrefix(t, "<!--") {
		return "", false, true
	}
	if strings.HasPrefix(t, "<!") || strings.HasPrefix(t, "<?") {
		return "", false, true
	}
	if strings.HasPrefix(t, "</") {
		isClose = true
		t = strings.TrimSpace(strings.TrimPrefix(t, "</"))
	} else if strings.HasPrefix(t, "<") {
		t = strings.TrimSpace(strings.TrimPrefix(t, "<"))
	}
	t = strings.TrimSuffix(t, ">")
	t = strings.TrimSpace(t)
	if strings.HasSuffix(t, "/") {
		isSelfClosing = true
		t = strings.TrimSpace(strings.TrimSuffix(t, "/"))
	}
	// Name ends at whitespace or /.
	name = ""
	for i := 0; i < len(t); i++ {
		if unicode.IsSpace(rune(t[i])) || t[i] == '/' {
			name = t[:i]
			break
		}
	}
	if name == "" {
		name = t
	}
	name = strings.ToLower(name)
	return name, isClose, isSelfClosing
}

func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func isRawTextTag(name string) bool {
	switch name {
	case "script", "style", "pre", "textarea":
		return true
	default:
		return false
	}
}
