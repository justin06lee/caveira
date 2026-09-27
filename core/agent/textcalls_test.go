package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin06lee/caveira/core/llm"
)

func knownTools(name string) bool { return name == "read_file" || name == "list_dir" || name == "bash" }

func TestTextToolCallsAreRecovered(t *testing.T) {
	cases := []struct {
		name, text, tool, args string
	}{
		// What llama3.2 on Ollama sent, "parameters=" slip and all.
		{"ollama slip", `{"name":"read_file","parameters={"path":"/Users/me/caveira"}}`, "read_file", `{"path":"/Users/me/caveira"}`},
		{"llama json", `{"name": "list_dir", "parameters": {"path": "."}}`, "list_dir", `{"path": "."}`},
		{"arguments as a string", `{"name":"bash","arguments":"{\"command\":\"ls\"}"}`, "bash", `{"command":"ls"}`},
		{"openai shape", `{"type":"function","function":{"name":"bash","arguments":{"command":"pwd"}}}`, "bash", `{"command":"pwd"}`},
		{"hermes tags", "<tool_call>\n{\"name\": \"list_dir\", \"arguments\": {\"path\": \"src\"}}\n</tool_call>", "list_dir", `{"path": "src"}`},
		{"python tag", `<|python_tag|>{"name": "read_file", "parameters": {"path": "go.mod"}}`, "read_file", `{"path": "go.mod"}`},
		{"code fence", "```json\n{\"name\": \"read_file\", \"parameters\": {\"path\": \"a.go\"}}\n```", "read_file", `{"path": "a.go"}`},
		{"missing brace", `{"name": "read_file", "parameters": {"path": "a.go"}`, "read_file", `{"path": "a.go"}`},
		{"no arguments", `{"name": "list_dir"}`, "list_dir", `{}`},
	}
	for _, c := range cases {
		calls := textToolCalls(c.text, knownTools)
		if len(calls) != 1 {
			t.Errorf("%s: got %d calls from %q", c.name, len(calls), c.text)
			continue
		}
		var got, want any
		json.Unmarshal([]byte(calls[0].Function.Arguments), &got)
		json.Unmarshal([]byte(c.args), &want)
		if calls[0].Function.Name != c.tool || !jsonEqual(got, want) {
			t.Errorf("%s: got %s(%s), want %s(%s)", c.name, calls[0].Function.Name, calls[0].Function.Arguments, c.tool, c.args)
		}
	}

	two := textToolCalls(`{"name":"list_dir","parameters":{"path":"."}} {"name":"read_file","parameters":{"path":"go.mod"}}`, knownTools)
	if len(two) != 2 || two[0].ID == two[1].ID {
		t.Errorf("two calls in a row: %+v", two)
	}

	for _, text := range []string{
		"Hello! How can I help?",
		`To read a file I would call {"name": "read_file", "parameters": {"path": "x"}} for you.`,
		`{"name": "run_command", "parameters": {"cmd": "build"}}`, // no such tool
		`{"answer": 42}`,
		`{"name": "read_file", "parameters": "not json"}`,
		`[1, 2, 3]`,
	} {
		if calls := textToolCalls(text, knownTools); calls != nil {
			t.Errorf("%q should stay text, got %+v", text, calls)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// A model that writes its tool call as its reply gets the call run, and
// the history holds a real tool call, not the text.
func TestRunExecutesToolCallWrittenAsText(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("buy milk\n"), 0o644)
	s := &scripted{responses: []func(w http.ResponseWriter){
		writeSSE(sse(`{"name":"read_file","parameters={"path":"notes.txt"}}`, nil)),
		writeSSE(sse("It says to buy milk.", nil)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()
	ag := newAgent(t, srv.URL, dir, false)

	events := collect(ag, context.Background(), "what's in notes.txt?", nil)
	var started, ended bool
	var doneTexts []string
	for _, ev := range events {
		switch ev := ev.(type) {
		case ToolStartEvent:
			started = ev.Name == "read_file"
		case ToolEndEvent:
			ended = !ev.Result.IsError
		case AssistantDoneEvent:
			doneTexts = append(doneTexts, ev.Message.Content)
		}
	}
	if !started || !ended {
		t.Fatalf("the written-out call did not run: %+v", events)
	}
	if len(doneTexts) != 2 || doneTexts[0] != "" || doneTexts[1] != "It says to buy milk." {
		t.Fatalf("assistant messages %q: the call's text should be dropped", doneTexts)
	}
	first := ag.Messages[1]
	if first.Role != llm.RoleAssistant || first.Content != "" || len(first.ToolCalls) != 1 {
		t.Fatalf("history holds %+v, want a real tool call", first)
	}
	if ag.Messages[2].Role != llm.RoleTool || ag.Messages[2].Content == "" {
		t.Fatalf("no tool result in the history: %+v", ag.Messages[2])
	}
}
