package importer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
)

// Codex keeps each chat as JSON lines in
// ~/.codex/sessions/<year>/<month>/<day>/rollout-<time>-<id>.jsonl, or
// under $CODEX_HOME. The first line describes the session; after it come
// response items in the Responses API's shape, with events and turn
// settings between them. Names given to chats are in session_index.jsonl.
// Codex imports Claude Code chats itself and lists the copies in
// external_agent_session_imports.json; those are left for the originals,
// and threads a chat spawned for its subagents are left with it.

func codexHome(home string) string { return envDir("CODEX_HOME", filepath.Join(home, ".codex")) }

type codexMeta struct {
	ID        string          `json:"id"`
	CWD       string          `json:"cwd"`
	Timestamp string          `json:"timestamp"`
	Source    json.RawMessage `json:"source"`
}

func scanCodex(ctx context.Context, home string) []Chat {
	root := codexHome(home)
	copies := codexCopies(root)
	titles := codexTitles(root)
	var out []Chat
	_ = filepath.WalkDir(filepath.Join(root, "sessions"), func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		m, ok := readCodexMeta(p)
		// A thread started by another one says so in its source, which is
		// then an object rather than the name of a client.
		if !ok || m.ID == "" || copies[m.ID] || strings.HasPrefix(string(m.Source), "{") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, Chat{App: Codex, ID: m.ID, Dir: m.CWD, Updated: info.ModTime(), where: p, title: titles[m.ID]})
		return nil
	})
	return out
}

func readCodexMeta(path string) (codexMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return codexMeta{}, false
	}
	defer f.Close()
	line, _ := bufio.NewReaderSize(f, 64<<10).ReadBytes('\n')
	var l struct {
		Type    string    `json:"type"`
		Payload codexMeta `json:"payload"`
	}
	if json.Unmarshal(line, &l) != nil || l.Type != "session_meta" {
		return codexMeta{}, false
	}
	return l.Payload, true
}

func codexCopies(root string) map[string]bool {
	out := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(root, "external_agent_session_imports.json"))
	if err != nil {
		return out
	}
	var f struct {
		Records []struct {
			ThreadID string `json:"imported_thread_id"`
		} `json:"records"`
	}
	if json.Unmarshal(b, &f) == nil {
		for _, r := range f.Records {
			out[r.ThreadID] = true
		}
	}
	return out
}

// codexTitles are the names chats were given, the latest for each.
func codexTitles(root string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(root, "session_index.jsonl"))
	if err != nil {
		return out
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		var e struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(line, &e) == nil && e.ID != "" && e.Name != "" {
			out[e.ID] = e.Name
		}
		if err != nil {
			return out
		}
	}
}

type codexItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Action    *struct {
		Command []string `json:"command"`
	} `json:"action"`
}

func convertCodex(c Chat) (*agent.Session, error) {
	f, err := os.Open(c.where)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := &agent.Session{WorkDir: c.Dir, Title: c.title}
	var msgs []llm.Message
	var reply *llm.Message
	flush := func() {
		if reply != nil {
			msgs = append(msgs, *reply)
			reply = nil
		}
	}
	replying := func() *llm.Message {
		if reply == nil {
			reply = &llm.Message{Role: llm.RoleAssistant}
		}
		return reply
	}

	r := bufio.NewReaderSize(f, 256<<10)
	for {
		raw, rerr := r.ReadBytes('\n')
		var l struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(raw, &l) == nil {
			if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
				if s.CreatedAt.IsZero() {
					s.CreatedAt = t
				}
				s.UpdatedAt = t
			}
			// Each kind of line is read as its own shape: the same field
			// name means different things on different lines.
			var it codexItem
			switch l.Type {
			case "turn_context":
				var tc struct {
					Model string `json:"model"`
				}
				if json.Unmarshal(l.Payload, &tc) == nil && tc.Model != "" {
					s.Model = tc.Model
				}
			case "compacted":
				var cp struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(l.Payload, &cp) == nil {
					reply = nil
					msgs = []llm.Message{compacted(cp.Message)}
				}
			case "response_item":
				if json.Unmarshal(l.Payload, &it) != nil {
					break
				}
				switch it.Type {
				case "message":
					switch it.Role {
					case "user":
						if words := userWords(text(it.Content)); words != "" {
							flush()
							msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: words})
						}
					case "assistant":
						if reply != nil && len(reply.ToolCalls) > 0 {
							flush()
						}
						replying().Content = join(replying().Content, text(it.Content))
					}
				case "reasoning":
					if reply != nil && len(reply.ToolCalls) > 0 {
						flush()
					}
					for _, sm := range it.Summary {
						replying().Reasoning = join(replying().Reasoning, sm.Text)
					}
				case "function_call":
					replying().ToolCalls = append(replying().ToolCalls, codexCall(it.CallID, it.Name, it.Arguments))
				case "custom_tool_call":
					replying().ToolCalls = append(replying().ToolCalls, codexCustomCall(it.CallID, it.Name, it.Input))
				case "local_shell_call":
					var cmd []string
					if it.Action != nil {
						cmd = it.Action.Command
					}
					replying().ToolCalls = append(replying().ToolCalls, call(it.CallID, "bash", map[string]any{"command": shellLine(cmd)}))
				case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
					flush()
					msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: it.CallID, Content: codexOutput(it.Output)})
				}
			}
		}
		if rerr != nil {
			break
		}
	}
	flush()
	s.Messages = msgs
	return s, nil
}

// codexCall is a Codex function call as caveira's, where caveira has one.
func codexCall(id, name, args string) llm.ToolCall {
	var in map[string]any
	_ = json.Unmarshal([]byte(args), &in)
	switch name {
	case "shell", "container.exec", "local_shell":
		var parts []string
		if list, ok := in["command"].([]any); ok {
			for _, p := range list {
				if s, ok := p.(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return call(id, "bash", map[string]any{"command": shellLine(parts)})
	case "shell_command":
		return call(id, "bash", rename(in, "command"))
	case "exec_command":
		return call(id, "bash", rename(in, "cmd:command"))
	}
	if in == nil {
		args = "{}"
	}
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: args}}
}

// codexCustomCall is a freeform tool call. The desktop app's code mode
// runs a script that calls tools; one that only runs a command is that
// command. Anything else keeps its name, with the input as an argument.
func codexCustomCall(id, name, input string) llm.ToolCall {
	if name == "exec" && strings.Count(input, "tools.") == 1 {
		if cmd, ok := scriptCommand(input); ok {
			return call(id, "bash", map[string]any{"command": cmd})
		}
	}
	return call(id, name, map[string]any{"input": input})
}

// scriptCommand reads cmd:"…" out of tools.exec_command({cmd:"…"}).
func scriptCommand(script string) (string, bool) {
	_, rest, ok := strings.Cut(script, "tools.exec_command(")
	if !ok {
		return "", false
	}
	_, rest, ok = strings.Cut(rest, "cmd:")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	for i := 1; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			i++
		case '"':
			var cmd string
			if json.Unmarshal([]byte(rest[:i+1]), &cmd) != nil {
				return "", false
			}
			return cmd, true
		}
	}
	return "", false
}

// codexOutput reads a tool result: plain text, content blocks, or, from
// older versions, JSON with the output and its exit code.
func codexOutput(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var obj struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &obj) == nil && len(obj.Content) > 0 {
			return text(obj.Content)
		}
		return text(raw)
	}
	var wrapped struct {
		Output   *string `json:"output"`
		Metadata struct {
			ExitCode int `json:"exit_code"`
		} `json:"metadata"`
	}
	if strings.HasPrefix(s, "{") && json.Unmarshal([]byte(s), &wrapped) == nil && wrapped.Output != nil {
		out := *wrapped.Output
		if wrapped.Metadata.ExitCode != 0 {
			// How caveira's own shell tool marks a command that failed.
			out = strings.TrimRight(out, "\n") + fmt.Sprintf("\n[exit code %d]", wrapped.Metadata.ExitCode)
		}
		return out
	}
	return s
}

// shellLine is an argv as the line typed: the script of bash -lc "…", or
// the words joined.
func shellLine(argv []string) string {
	if len(argv) == 3 && (argv[1] == "-lc" || argv[1] == "-c") {
		switch filepath.Base(argv[0]) {
		case "bash", "zsh", "sh":
			return argv[2]
		}
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\n'\"$`\\|&;<>()*?") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}
