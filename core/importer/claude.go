package importer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
)

// Claude Code keeps each chat as JSON lines in
// ~/.claude/projects/<folder, dashed>/<session id>.jsonl, or under
// $CLAUDE_CONFIG_DIR. Every line carries its own uuid and the one before
// it, so a chat that was rewound is a tree: the chat as it stands is the
// path from the newest message back to the start, or to the summary a
// compaction left. One reply is several lines, a block each, sharing the
// API's message id. Subagents write to their own files, one folder down,
// and are not chats of their own.

func claudeProjects(home string) string {
	return filepath.Join(envDir("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")), "projects")
}

func scanClaude(ctx context.Context, home string) []Chat {
	files, _ := filepath.Glob(filepath.Join(claudeProjects(home), "*", "*.jsonl"))
	var out []Chat
	for _, f := range files {
		if ctx.Err() != nil {
			break
		}
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if dir := claudeDir(f); dir != "" {
			out = append(out, Chat{App: Claude, ID: strings.TrimSuffix(filepath.Base(f), ".jsonl"), Dir: dir, Updated: st.ModTime(), where: f})
		}
	}
	return out
}

// claudeDir is the folder a chat ran in, from the first line that says.
func claudeDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for range 64 {
		line, err := r.ReadBytes('\n')
		var l struct {
			CWD string `json:"cwd"`
		}
		if json.Unmarshal(line, &l) == nil && l.CWD != "" {
			return l.CWD
		}
		if err != nil {
			break
		}
	}
	return ""
}

type claudeLine struct {
	Type             string          `json:"type"`
	UUID             string          `json:"uuid"`
	ParentUUID       string          `json:"parentUuid"`
	IsSidechain      bool            `json:"isSidechain"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	CWD              string          `json:"cwd"`
	Timestamp        string          `json:"timestamp"`
	Message          *claudeMessage  `json:"message"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"`
	AITitle          string          `json:"aiTitle"`
	CustomTitle      string          `json:"customTitle"`
	Summary          string          `json:"summary"`
}

type claudeMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func convertClaude(c Chat) (*agent.Session, error) {
	f, err := os.Open(c.where)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	byUUID := map[string]*claudeLine{}
	var last *claudeLine // the newest message: the end of the chat as it stands
	var customTitle, aiTitle, summary string
	r := bufio.NewReaderSize(f, 256<<10)
	for {
		raw, err := r.ReadBytes('\n')
		if len(raw) > 0 {
			l := new(claudeLine)
			if json.Unmarshal(raw, l) == nil {
				switch {
				case l.CustomTitle != "":
					customTitle = l.CustomTitle
				case l.AITitle != "":
					aiTitle = l.AITitle
				case l.Type == "summary" && l.Summary != "":
					summary = l.Summary
				}
				message := (l.Type == "user" || l.Type == "assistant") && !l.IsSidechain && l.Message != nil
				if !message {
					// Only its place in the chain matters. Chats run to
					// tens of megabytes, mostly attachments and file
					// snapshots.
					l = &claudeLine{Type: l.Type, UUID: l.UUID, ParentUUID: l.ParentUUID, IsCompactSummary: l.IsCompactSummary}
				} else if !bytes.Contains(l.ToolUseResult, []byte(`"structuredPatch"`)) {
					l.ToolUseResult = nil
				}
				if l.UUID != "" {
					byUUID[l.UUID] = l
				}
				if message {
					last = l
				}
			}
		}
		if err != nil {
			break
		}
	}
	if last == nil {
		return nil, ErrEmpty
	}

	// Walk back from the newest message. A compaction's summary has no
	// parent in the chain; nor does the first message.
	var chain []*claudeLine
	seen := map[string]bool{}
	for l := last; l != nil && !seen[l.UUID]; l = byUUID[l.ParentUUID] {
		seen[l.UUID] = true
		if (l.Type == "user" || l.Type == "assistant") && l.Message != nil && !l.IsSidechain {
			chain = append(chain, l)
		}
		if l.IsCompactSummary || l.ParentUUID == "" {
			break
		}
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	s := &agent.Session{WorkDir: c.Dir, Title: firstNonEmpty(customTitle, aiTitle, summary)}
	var msgs []llm.Message
	var reply *llm.Message
	replyID := ""
	names := map[string]string{} // tool call id → the tool's name in caveira
	flush := func() {
		if reply != nil {
			msgs = append(msgs, *reply)
			reply, replyID = nil, ""
		}
	}

	for _, l := range chain {
		if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
			if s.CreatedAt.IsZero() {
				s.CreatedAt = t
			}
			s.UpdatedAt = t
		}
		if l.CWD != "" && s.WorkDir == "" {
			s.WorkDir = l.CWD
		}
		m := l.Message
		var blocks []claudeBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			var str string
			if json.Unmarshal(m.Content, &str) == nil {
				blocks = []claudeBlock{{Type: "text", Text: str}}
			}
		}

		if l.Type == "assistant" {
			if reply == nil || m.ID == "" || m.ID != replyID {
				flush()
				reply, replyID = &llm.Message{Role: llm.RoleAssistant}, m.ID
			}
			if m.Model != "" && !strings.HasPrefix(m.Model, "<") {
				s.Model = m.Model
			}
			for _, b := range blocks {
				switch b.Type {
				case "text":
					reply.Content = join(reply.Content, b.Text)
				case "thinking":
					reply.Reasoning = join(reply.Reasoning, b.Thinking)
				case "tool_use":
					tc := claudeCall(b.ID, b.Name, b.Input)
					names[b.ID] = tc.Function.Name
					reply.ToolCalls = append(reply.ToolCalls, tc)
				}
			}
			continue
		}

		if l.IsMeta {
			continue
		}
		if l.IsCompactSummary {
			flush()
			msgs = append(msgs, compacted(text(m.Content)))
			continue
		}
		var said []string
		for _, b := range blocks {
			switch b.Type {
			case "tool_result":
				flush()
				out := text(b.Content)
				if b.IsError && !strings.HasPrefix(out, "Error") {
					out = "Error: " + out
				}
				if names[b.ToolUseID] == "edit_file" && !b.IsError {
					if path, diff := claudePatch(l.ToolUseResult); diff != "" {
						out = edited(path, diff)
					}
				}
				msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: b.ToolUseID, Name: names[b.ToolUseID], Content: out})
			case "text":
				said = append(said, b.Text)
			}
		}
		words := userWords(strings.Join(said, "\n\n"))
		if strings.HasPrefix(words, "[Request interrupted by user") {
			flush()
			if n := len(msgs); n > 0 && msgs[n-1].Role == llm.RoleAssistant && len(msgs[n-1].ToolCalls) == 0 && !strings.HasSuffix(msgs[n-1].Content, interruptedSuffix) {
				msgs[n-1].Content += interruptedSuffix
			}
			continue
		}
		if words != "" {
			flush()
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: words})
		}
	}
	flush()
	s.Messages = msgs
	return s, nil
}

// claudeCall is a Claude Code tool call as the caveira tool that does the
// same thing. Tools caveira has no match for keep their name and input.
func claudeCall(id, name string, input json.RawMessage) llm.ToolCall {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	switch name {
	case "Bash":
		return call(id, "bash", rename(in, "command", "description"))
	case "Read":
		return call(id, "read_file", rename(in, "file_path:path", "offset", "limit"))
	case "Write":
		return call(id, "write_file", rename(in, "file_path:path", "content"))
	case "Edit":
		return call(id, "edit_file", rename(in, "file_path:path", "old_string", "new_string", "replace_all"))
	case "Glob":
		return call(id, "glob", rename(in, "pattern", "path"))
	case "Grep":
		return call(id, "grep", rename(in, "pattern", "path", "glob", "output_mode", "-i:case_insensitive", "-C:context"))
	case "LS":
		return call(id, "list_dir", rename(in, "path"))
	}
	args := string(input)
	if in == nil {
		args = "{}"
	}
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: args}}
}

// claudePatch is the diff Claude Code kept beside an edit's result.
func claudePatch(raw json.RawMessage) (path, diff string) {
	var r struct {
		FilePath        string `json:"filePath"`
		StructuredPatch []struct {
			OldStart int      `json:"oldStart"`
			OldLines int      `json:"oldLines"`
			NewStart int      `json:"newStart"`
			NewLines int      `json:"newLines"`
			Lines    []string `json:"lines"`
		} `json:"structuredPatch"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.StructuredPatch) == 0 {
		return "", ""
	}
	var sb strings.Builder
	for _, h := range r.StructuredPatch {
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return r.FilePath, sb.String()
}

func join(a, b string) string {
	if strings.TrimSpace(b) == "" {
		return a
	}
	if a == "" {
		return b
	}
	return a + "\n\n" + b
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
