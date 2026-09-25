package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/justin06lee/caveira/tui/internal/llm"
)

// Some models write a tool call out as the text of their reply instead of
// making it: small local ones especially, and any model behind a server
// whose chat template parser missed a malformed call. llama3.2 on Ollama,
// for one, sends {"name": "read_file", "parameters": {...}} as its reply,
// sometimes with slips like "parameters={" that Ollama then gives up on.
// textToolCalls turns a reply that is nothing but such calls into real
// ones. Anything else, prose that mentions a tool included, is left alone.

var (
	// A key followed by = instead of ":, or by = after its closing quote.
	argsKeySlip = regexp.MustCompile(`"(parameters|arguments)"?\s*=\s*`)
	// Wrappers models put around calls: Llama's python tag, Hermes-style
	// tags, and code fences.
	callWrappers = regexp.MustCompile("(?s)^(?:<\\|python_tag\\|>|<tool_call>|```(?:json)?)\\s*|\\s*(?:</tool_call>|<\\|eom_id\\|>|<\\|eot_id\\|>|```)$")
)

// textToolCalls returns the calls written out in content, with known
// saying which tool names exist. It returns nil unless every part of the
// text is a well-formed call to a known tool.
func textToolCalls(content string, known func(string) bool) []llm.ToolCall {
	s := strings.TrimSpace(content)
	for {
		t := strings.TrimSpace(callWrappers.ReplaceAllString(s, ""))
		if t == s {
			break
		}
		s = t
	}
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") || !strings.Contains(s, `"name"`) {
		return nil
	}
	s = closeBraces(argsKeySlip.ReplaceAllString(s, `"$1": `))

	var raws []json.RawMessage
	dec := json.NewDecoder(strings.NewReader(s))
	for dec.More() {
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil
		}
		if bytes.HasPrefix(bytes.TrimSpace(v), []byte("[")) {
			var list []json.RawMessage
			if json.Unmarshal(v, &list) != nil {
				return nil
			}
			raws = append(raws, list...)
		} else {
			raws = append(raws, v)
		}
	}

	var calls []llm.ToolCall
	for i, raw := range raws {
		name, args, ok := parseTextCall(raw)
		if !ok || !known(name) {
			return nil
		}
		calls = append(calls, llm.ToolCall{
			ID:       fmt.Sprintf("call_text_%d", i),
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		})
	}
	return calls
}

// parseTextCall reads one call in any of the shapes models write:
// {"name", "parameters"}, {"name", "arguments"} with the arguments as an
// object or as JSON in a string, or OpenAI's {"function": {...}}.
func parseTextCall(raw json.RawMessage) (name, args string, ok bool) {
	var c struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
		Arguments  json.RawMessage `json:"arguments"`
		Function   *struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return "", "", false
	}
	a := c.Parameters
	if a == nil {
		a = c.Arguments
	}
	if c.Function != nil {
		c.Name, a = c.Function.Name, c.Function.Arguments
	}
	if c.Name == "" {
		return "", "", false
	}
	a = bytes.TrimSpace(a)
	if len(a) == 0 || string(a) == "null" {
		a = []byte("{}")
	}
	if a[0] == '"' {
		var inner string
		if json.Unmarshal(a, &inner) != nil {
			return "", "", false
		}
		a = []byte(inner)
	}
	var obj map[string]any
	if json.Unmarshal(a, &obj) != nil {
		return "", "", false
	}
	return c.Name, string(a), true
}

// closeBraces adds the closing braces and brackets a truncated or sloppy
// call left off, ignoring any inside strings.
func closeBraces(s string) string {
	var open []byte
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case esc:
			esc = false
		case inStr && ch == '\\':
			esc = true
		case ch == '"':
			inStr = !inStr
		case inStr:
		case ch == '{' || ch == '[':
			open = append(open, ch)
		case (ch == '}' || ch == ']') && len(open) > 0:
			open = open[:len(open)-1]
		}
	}
	if inStr {
		return s
	}
	for i := len(open) - 1; i >= 0; i-- {
		if open[i] == '{' {
			s += "}"
		} else {
			s += "]"
		}
	}
	return s
}
