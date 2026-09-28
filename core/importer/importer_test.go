package importer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin06lee/caveira/core/llm"
)

// fakeHome is an empty home with a project folder in it. Temporary
// directories are where Scan leaves chats alone, so that rule is off here.
func fakeHome(t *testing.T) (home, project string) {
	t.Helper()
	home = t.TempDir()
	project = filepath.Join(home, "work", "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_DATA_HOME"} {
		t.Setenv(env, "")
	}
	old := Scratch
	Scratch = nil
	t.Cleanup(func() { Scratch = old })
	return home, project
}

func writeLines(t *testing.T, path string, lines ...any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

type obj = map[string]any

func only(t *testing.T, chats []Chat, app App) Chat {
	t.Helper()
	var got []Chat
	for _, c := range chats {
		if c.App == app {
			got = append(got, c)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one %s chat, found %d: %+v", app.Name, len(got), got)
	}
	return got[0]
}

func args(t *testing.T, c llm.ToolCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(c.Function.Arguments), &m); err != nil {
		t.Fatalf("arguments of %s are not JSON: %v", c.Function.Name, err)
	}
	return m
}

func TestClaude(t *testing.T) {
	home, project := fakeHome(t)
	dir := filepath.Join(home, ".claude", "projects", "-work-proj")
	user := func(uuid, parent string, content any, extra obj) obj {
		l := obj{"type": "user", "uuid": uuid, "parentUuid": parent, "cwd": project, "timestamp": "2026-09-01T10:00:00Z",
			"message": obj{"role": "user", "content": content}}
		for k, v := range extra {
			l[k] = v
		}
		return l
	}
	assistant := func(uuid, parent, id string, block obj) obj {
		return obj{"type": "assistant", "uuid": uuid, "parentUuid": parent, "cwd": project, "timestamp": "2026-09-01T10:01:00Z",
			"message": obj{"id": id, "role": "assistant", "model": "claude-test", "content": []obj{block}}}
	}
	writeLines(t, filepath.Join(dir, "s1.jsonl"),
		obj{"type": "queue-operation", "operation": "enqueue"},
		user("u1", "", []obj{{"type": "text", "text": "fix the bug"}, {"type": "text", "text": "<system-reminder>be nice</system-reminder>"}}, nil),
		user("meta", "u1", "<command-name>/model</command-name>", obj{"isMeta": true}),
		assistant("a1", "u1", "m1", obj{"type": "thinking", "thinking": "hmm"}),
		assistant("a2", "a1", "m1", obj{"type": "text", "text": "Looking."}),
		assistant("a3", "a2", "m1", obj{"type": "tool_use", "id": "t1", "name": "Edit",
			"input": obj{"file_path": "/p/x.go", "old_string": "a", "new_string": "b"}}),
		obj{"type": "attachment", "uuid": "x1", "parentUuid": "a3"},
		user("r1", "x1", []obj{{"type": "tool_result", "tool_use_id": "t1", "content": "The file /p/x.go has been updated."}},
			obj{"toolUseResult": obj{"filePath": "/p/x.go", "structuredPatch": []obj{{"oldStart": 1, "oldLines": 1, "newStart": 1, "newLines": 1, "lines": []string{"-a", "+b"}}}}}),
		// A reply that was rewound: on the file, not on the chat.
		assistant("gone", "r1", "m9", obj{"type": "text", "text": "Abandoned."}),
		assistant("a4", "r1", "m2", obj{"type": "tool_use", "id": "t2", "name": "Task", "input": obj{"prompt": "look around"}}),
		obj{"type": "assistant", "uuid": "side", "parentUuid": "a4", "isSidechain": true,
			"message": obj{"id": "m5", "role": "assistant", "content": []obj{{"type": "text", "text": "subagent"}}}},
		user("r2", "a4", []obj{{"type": "tool_result", "tool_use_id": "t2", "is_error": true, "content": []obj{{"type": "text", "text": "it broke"}}}}, nil),
		assistant("a5", "r2", "m3", obj{"type": "text", "text": "Done."}),
		obj{"type": "ai-title", "aiTitle": "Fix the bug"},
		user("u2", "a5", []obj{{"type": "text", "text": "[Request interrupted by user]"}}, nil),
	)
	// Written by a subagent, one folder down: not a chat of its own.
	writeLines(t, filepath.Join(dir, "s1", "subagents", "agent-1.jsonl"), user("z", "", "hi", nil))

	c := only(t, Scan(context.Background(), home), Claude)
	if c.Dir != project || c.ID != "s1" || c.SessionID() != "claude-s1" {
		t.Fatalf("found %+v", c)
	}
	s, err := Convert(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Fix the bug" || s.Source != "Claude Code" || s.Model != "claude-test" || s.WorkDir != project || s.ID != "claude-s1" {
		t.Errorf("header: %+v", s)
	}
	m := s.Messages
	if len(m) != 6 {
		for _, x := range m {
			t.Logf("%s %q %d", x.Role, x.Content, len(x.ToolCalls))
		}
		t.Fatalf("want 6 messages, got %d", len(m))
	}
	if m[0].Role != llm.RoleUser || m[0].Content != "fix the bug" {
		t.Errorf("user message: %+v", m[0])
	}
	if m[1].Reasoning != "hmm" || m[1].Content != "Looking." || len(m[1].ToolCalls) != 1 {
		t.Errorf("one reply from three lines: %+v", m[1])
	}
	edit := m[1].ToolCalls[0]
	if a := args(t, edit); edit.Function.Name != "edit_file" || a["path"] != "/p/x.go" || a["old_string"] != "a" || a["new_string"] != "b" {
		t.Errorf("edit call: %s %s", edit.Function.Name, edit.Function.Arguments)
	}
	if m[2].Role != llm.RoleTool || m[2].Name != "edit_file" || m[2].Content != "Edited /p/x.go.\n\n@@ -1,1 +1,1 @@\n-a\n+b" {
		t.Errorf("edit result: %+v", m[2])
	}
	if m[3].ToolCalls[0].Function.Name != "Task" || args(t, m[3].ToolCalls[0])["prompt"] != "look around" {
		t.Errorf("a tool caveira lacks keeps its name and input: %+v", m[3].ToolCalls)
	}
	if m[4].Content != "Error: it broke" {
		t.Errorf("error result: %q", m[4].Content)
	}
	if m[5].Content != "Done."+interruptedSuffix {
		t.Errorf("interrupted reply: %q", m[5].Content)
	}
}

func TestClaudeCompactedChatStartsAtItsSummary(t *testing.T) {
	home, project := fakeHome(t)
	writeLines(t, filepath.Join(home, ".claude", "projects", "-work-proj", "s2.jsonl"),
		obj{"type": "user", "uuid": "old", "parentUuid": nil, "cwd": project, "message": obj{"role": "user", "content": "early days"}},
		obj{"type": "system", "subtype": "compact_boundary", "uuid": "b", "parentUuid": nil, "logicalParentUuid": "old"},
		obj{"type": "user", "uuid": "sum", "parentUuid": "b", "isCompactSummary": true, "cwd": project,
			"message": obj{"role": "user", "content": "We built a thing."}},
		obj{"type": "user", "uuid": "u", "parentUuid": "sum", "cwd": project, "message": obj{"role": "user", "content": "keep going"}},
	)
	s, err := Convert(only(t, Scan(context.Background(), home), Claude))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Messages) != 2 || !strings.HasPrefix(s.Messages[0].Content, compactedPrefix) ||
		!strings.Contains(s.Messages[0].Content, "We built a thing.") || s.Messages[1].Content != "keep going" {
		t.Errorf("messages: %+v", s.Messages)
	}
	if s.Title != "keep going" {
		t.Errorf("title from the first thing the user said, not the summary: %q", s.Title)
	}
}

func TestCodex(t *testing.T) {
	home, project := fakeHome(t)
	root := filepath.Join(home, ".codex")
	day := filepath.Join(root, "sessions", "2026", "09", "01")
	line := func(typ string, payload obj) obj {
		return obj{"timestamp": "2026-09-01T10:00:00.000Z", "type": typ, "payload": payload}
	}
	item := func(payload obj) obj { return line("response_item", payload) }
	msg := func(role, typ, text string) obj {
		return item(obj{"type": "message", "role": role, "content": []obj{{"type": typ, "text": text}}})
	}
	writeLines(t, filepath.Join(day, "rollout-2026-09-01T10-00-00-c1.jsonl"),
		line("session_meta", obj{"id": "c1", "cwd": project, "source": "vscode", "base_instructions": obj{"text": "…"}}),
		line("turn_context", obj{"model": "gpt-test", "summary": "auto", "cwd": project}),
		msg("developer", "input_text", "<permissions instructions>sandbox</permissions instructions>"),
		msg("user", "input_text", "<environment_context>\n  <cwd>"+project+"</cwd>\n</environment_context>"),
		msg("user", "input_text", "make it faster"),
		item(obj{"type": "reasoning", "summary": []obj{{"type": "summary_text", "text": "**Profiling**"}}, "encrypted_content": "x"}),
		msg("assistant", "output_text", "On it."),
		item(obj{"type": "function_call", "name": "shell", "call_id": "k1",
			"arguments": `{"command":["bash","-lc","go test ./..."],"workdir":"/p"}`}),
		item(obj{"type": "function_call_output", "call_id": "k1", "output": `{"output":"FAIL\n","metadata":{"exit_code":1}}`}),
		item(obj{"type": "custom_tool_call", "name": "exec", "call_id": "k2", "input": `text(await tools.exec_command({cmd:"ls -la \"x y\"",max_output_tokens:100}));`}),
		item(obj{"type": "custom_tool_call_output", "call_id": "k2", "output": []obj{{"type": "input_text", "text": "Script completed\nOutput:\nx y"}, {"type": "input_image", "image_url": "data:…"}}}),
		item(obj{"type": "custom_tool_call", "name": "apply_patch", "call_id": "k3", "input": "*** Begin Patch\n*** End Patch"}),
		msg("assistant", "output_text", "Fixed."),
	)
	// Codex's own copy of a Claude Code chat, and a thread a chat spawned.
	writeLines(t, filepath.Join(day, "rollout-2026-09-01T11-00-00-c2.jsonl"),
		line("session_meta", obj{"id": "c2", "cwd": project, "source": "vscode"}), msg("user", "input_text", "copied"))
	writeLines(t, filepath.Join(day, "rollout-2026-09-01T12-00-00-c3.jsonl"),
		line("session_meta", obj{"id": "c3", "cwd": project, "source": obj{"subagent": "review"}}), msg("user", "input_text", "spawned"))
	writeLines(t, filepath.Join(root, "session_index.jsonl"),
		obj{"id": "c1", "thread_name": "Old name"}, obj{"id": "c1", "thread_name": "Speed up tests"})
	b, _ := json.Marshal(obj{"records": []obj{{"imported_thread_id": "c2"}}})
	if err := os.WriteFile(filepath.Join(root, "external_agent_session_imports.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	c := only(t, Scan(context.Background(), home), Codex)
	s, err := Convert(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Speed up tests" || s.Model != "gpt-test" || s.ID != "codex-c1" || s.Source != "Codex" {
		t.Errorf("header: %+v", s)
	}
	m := s.Messages
	if len(m) != 8 {
		for _, x := range m {
			t.Logf("%s %q %+v", x.Role, x.Content, x.ToolCalls)
		}
		t.Fatalf("want 8 messages, got %d", len(m))
	}
	if m[0].Content != "make it faster" {
		t.Errorf("injected context kept: %q", m[0].Content)
	}
	if m[1].Reasoning != "**Profiling**" || m[1].Content != "On it." || args(t, m[1].ToolCalls[0])["command"] != "go test ./..." {
		t.Errorf("first reply: %+v", m[1])
	}
	if m[2].Content != "FAIL\n[exit code 1]" {
		t.Errorf("a failed command reads as one to caveira: %q", m[2].Content)
	}
	if c := m[3].ToolCalls; len(c) != 1 || c[0].Function.Name != "bash" || args(t, c[0])["command"] != `ls -la "x y"` {
		t.Errorf("a script that runs one command is that command: %+v", c)
	}
	if m[4].Content != "Script completed\nOutput:\nx y\n[image]" {
		t.Errorf("block output: %q", m[4].Content)
	}
	if c := m[5].ToolCalls; len(c) != 1 || c[0].Function.Name != "apply_patch" || args(t, c[0])["input"] != "*** Begin Patch\n*** End Patch" {
		t.Errorf("patch call: %+v", c)
	}
	if m[6].Role != llm.RoleTool || !strings.HasPrefix(m[6].Content, "Error: no result") {
		t.Errorf("a call left unanswered gets an answer: %+v", m[6])
	}
	if m[7].Content != "Fixed." {
		t.Errorf("last reply: %+v", m[7])
	}
}

func TestOpenCode(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("no sqlite3")
	}
	home, project := fakeHome(t)
	db := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	j := func(v any) string {
		b, _ := json.Marshal(v)
		return strings.ReplaceAll(string(b), "'", "''")
	}
	diff := "Index: /p/a.go\n===\n--- /p/a.go\n+++ /p/a.go\n@@ -1,1 +1,1 @@\n-x\n+y\n"
	sql := `
create table session (id text primary key, parent_id text, directory text, title text, time_updated integer, time_archived integer);
create table message (id text primary key, session_id text, time_created integer, data text);
create table part (id text primary key, message_id text, session_id text, time_created integer, data text);
insert into session values ('ses_1', null, '` + project + `', 'Rename things', 1790000000000, null);
insert into session values ('ses_2', 'ses_1', '` + project + `', 'subagent', 1790000000000, null);
insert into session values ('ses_3', null, '` + project + `', 'archived', 1790000000000, 1790000000001);
insert into message values ('msg_1', 'ses_1', 1, '` + j(obj{"role": "user", "summary": obj{"diffs": []obj{}}}) + `');
insert into message values ('msg_2', 'ses_1', 2, '` + j(obj{"role": "assistant", "modelID": "big-model"}) + `');
insert into part values ('prt_1', 'msg_1', 'ses_1', 1, '` + j(obj{"type": "text", "text": "rename x to y"}) + `');
insert into part values ('prt_2', 'msg_1', 'ses_1', 2, '` + j(obj{"type": "text", "text": "Called the Read tool", "synthetic": true}) + `');
insert into part values ('prt_3', 'msg_2', 'ses_1', 3, '` + j(obj{"type": "step-start"}) + `');
insert into part values ('prt_4', 'msg_2', 'ses_1', 4, '` + j(obj{"type": "reasoning", "text": "easy"}) + `');
insert into part values ('prt_5', 'msg_2', 'ses_1', 5, '` + j(obj{"type": "tool", "tool": "edit", "callID": "c1", "state": obj{"status": "completed",
		"input": obj{"filePath": "/p/a.go", "oldString": "x", "newString": "y"}, "output": "Edit applied successfully.", "metadata": obj{"diff": diff}}}) + `');
insert into part values ('prt_6', 'msg_2', 'ses_1', 6, '` + j(obj{"type": "tool", "tool": "bash", "callID": "c2", "state": obj{"status": "error",
		"input": obj{"command": "go build"}, "error": "it's broken"}}) + `');
insert into part values ('prt_7', 'msg_2', 'ses_1', 7, '` + j(obj{"type": "text", "text": "Renamed."}) + `');
`
	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	c := only(t, Scan(context.Background(), home), OpenCode)
	s, err := Convert(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Rename things" || s.Model != "big-model" || s.ID != "opencode-ses_1" {
		t.Errorf("header: %+v", s)
	}
	m := s.Messages
	if len(m) != 5 {
		for _, x := range m {
			t.Logf("%s %q %+v", x.Role, x.Content, x.ToolCalls)
		}
		t.Fatalf("want 5 messages, got %d", len(m))
	}
	if m[0].Content != "rename x to y" {
		t.Errorf("user message: %q", m[0].Content)
	}
	if m[1].Reasoning != "easy" || len(m[1].ToolCalls) != 2 || m[1].ToolCalls[0].Function.Name != "edit_file" ||
		args(t, m[1].ToolCalls[0])["old_string"] != "x" || args(t, m[1].ToolCalls[1])["command"] != "go build" {
		t.Errorf("reply with calls: %+v", m[1])
	}
	if m[2].Content != "Edited /p/a.go.\n\n@@ -1,1 +1,1 @@\n-x\n+y" {
		t.Errorf("edit result: %q", m[2].Content)
	}
	if m[3].Content != "Error: it's broken" {
		t.Errorf("error result: %q", m[3].Content)
	}
	if m[4].Content != "Renamed." {
		t.Errorf("text after the calls is the next step: %+v", m[4])
	}
}

func TestScanLeavesScratchAndMissingFolders(t *testing.T) {
	home, project := fakeHome(t)
	Scratch = []string{filepath.Join(home, "tmp")}
	for i, dir := range []string{project, filepath.Join(home, "tmp", "x"), filepath.Join(home, "gone"), "/Applications/Some.app/Contents/Resources"} {
		_ = os.MkdirAll(filepath.Join(home, "tmp", "x"), 0o755)
		writeLines(t, filepath.Join(home, ".claude", "projects", "p", string(rune('a'+i))+".jsonl"),
			obj{"type": "user", "uuid": "u", "cwd": dir, "message": obj{"role": "user", "content": "hi"}})
	}
	chats := Scan(context.Background(), home)
	if len(chats) != 1 || chats[0].Dir != project {
		t.Errorf("want only the project, got %+v", chats)
	}
}

func TestUserWords(t *testing.T) {
	for in, want := range map[string]string{
		"hello": "hello",
		"<environment_context>\n<cwd>/x</cwd>\n</environment_context>":                           "",
		"<compacted-history>notes</compacted-history>\n\n<relevant>more</relevant>\n\nnow do it": "now do it",
		"do it <system-reminder>psst</system-reminder>":                                          "do it",
		"<command-name>/model</command-name>\n<command-message>model</command-message>":          "",
		"<div>real html the user pasted":                                                         "<div>real html the user pasted",
		"# AGENTS.md instructions for /p\n\nstuff":                                               "",
	} {
		if got := userWords(in); got != want {
			t.Errorf("userWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTidy(t *testing.T) {
	calls := []llm.ToolCall{{ID: "a", Function: llm.FunctionCall{Name: "bash"}}, {ID: "b", Function: llm.FunctionCall{Name: "grep"}}}
	got := tidy([]llm.Message{
		{Role: llm.RoleUser, Content: "go"},
		{Role: llm.RoleAssistant},
		{Role: llm.RoleAssistant, ToolCalls: calls},
		{Role: llm.RoleTool, ToolCallID: "b", Content: strings.Repeat("x", outputCap*2)},
		{Role: llm.RoleTool, ToolCallID: "zzz", Content: "orphan"},
		{Role: llm.RoleUser, Content: "next"},
	})
	if len(got) != 5 {
		t.Fatalf("want 5, got %+v", got)
	}
	if got[1].ToolCalls[0].Function.Arguments != "{}" || got[1].ToolCalls[0].Type != "function" {
		t.Errorf("call not filled in: %+v", got[1].ToolCalls[0])
	}
	if got[2].ToolCallID != "b" || got[2].Name != "grep" || len(got[2].Content) > outputCap+200 {
		t.Errorf("result not named and cut: %s %d", got[2].Name, len(got[2].Content))
	}
	if got[3].ToolCallID != "a" || !strings.HasPrefix(got[3].Content, "Error:") {
		t.Errorf("unanswered call: %+v", got[3])
	}
	if got[4].Content != "next" {
		t.Errorf("order: %+v", got[4])
	}
}
