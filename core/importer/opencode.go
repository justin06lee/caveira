package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
)

// OpenCode keeps its chats in SQLite, at
// ~/.local/share/opencode/opencode.db or under $XDG_DATA_HOME: a session
// row per chat, a message row per turn, and a part row for each piece of
// a turn (text, reasoning, a tool call with its result). It is read with
// the sqlite3 that comes with macOS, so caveira carries no driver for it.
// Sessions with a parent are subagents' and stay with their chat.

func openCodeDB(home string) string {
	return filepath.Join(envDir("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "opencode", "opencode.db")
}

func scanOpenCode(ctx context.Context, home string) []Chat {
	db := openCodeDB(home)
	if _, err := os.Stat(db); err != nil {
		return nil
	}
	var found []struct {
		ID      string `json:"id"`
		Dir     string `json:"directory"`
		Title   string `json:"title"`
		Updated int64  `json:"time_updated"`
	}
	if rows(ctx, db, []string{"id", "directory", "title", "time_updated"}, "from session where parent_id is null and time_archived is null", &found) != nil {
		return nil
	}
	out := make([]Chat, 0, len(found))
	for _, r := range found {
		out = append(out, Chat{App: OpenCode, ID: r.ID, Dir: r.Dir, Updated: time.UnixMilli(r.Updated), where: db, title: r.Title})
	}
	return out
}

// rows runs "select <cols> <rest>" on db, read-only, and decodes the rows
// into out as objects keyed by column. SQLite writes the JSON itself:
// the sqlite3 shell's own JSON mode takes seconds over a few hundred
// kilobytes of text.
func rows(ctx context.Context, db string, cols []string, rest string, out any) error {
	bin := "/usr/bin/sqlite3"
	if _, err := os.Stat(bin); err != nil {
		if bin, err = exec.LookPath("sqlite3"); err != nil {
			return err
		}
	}
	pairs := make([]string, len(cols))
	for i, c := range cols {
		pairs[i] = sqlQuote(c) + ", " + c
	}
	query := fmt.Sprintf("select coalesce(json_group_array(json_object(%s)), '[]') from (select %s %s)",
		strings.Join(pairs, ", "), strings.Join(cols, ", "), rest)
	// Readers wait for each other, and for OpenCode if it is writing,
	// rather than failing as busy.
	b, err := exec.CommandContext(ctx, bin, "-readonly", "-cmd", ".timeout 10000", db, query).Output()
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes.TrimSpace(b), out)
}

func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

type openCodePart struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Synthetic bool   `json:"synthetic"`
	Ignored   bool   `json:"ignored"`
	Filename  string `json:"filename"`
	Tool      string `json:"tool"`
	CallID    string `json:"callID"`
	State     struct {
		Status   string         `json:"status"`
		Input    map[string]any `json:"input"`
		Output   string         `json:"output"`
		Error    string         `json:"error"`
		Metadata struct {
			Diff string `json:"diff"`
		} `json:"metadata"`
	} `json:"state"`
}

func convertOpenCode(c Chat) (*agent.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var messages []struct {
		ID      string `json:"id"`
		Created int64  `json:"time_created"`
		Data    string `json:"data"`
	}
	id := sqlQuote(c.ID)
	if err := rows(ctx, c.where, []string{"id", "time_created", "data"}, "from message where session_id = "+id+" order by time_created, id", &messages); err != nil {
		return nil, err
	}
	var partRows []struct {
		Message string `json:"message_id"`
		Data    string `json:"data"`
	}
	if err := rows(ctx, c.where, []string{"message_id", "data"}, "from part where session_id = "+id+" order by time_created, id", &partRows); err != nil {
		return nil, err
	}
	parts := map[string][]openCodePart{}
	for _, r := range partRows {
		var p openCodePart
		if json.Unmarshal([]byte(r.Data), &p) == nil {
			parts[r.Message] = append(parts[r.Message], p)
		}
	}

	s := &agent.Session{WorkDir: c.Dir, Title: c.title}
	var msgs []llm.Message
	for _, m := range messages {
		var data struct {
			Role    string `json:"role"`
			ModelID string `json:"modelID"`
			// Summary is true on a compaction's summary; on a user
			// message it is an object describing the turn's changes.
			Summary json.RawMessage `json:"summary"`
		}
		if json.Unmarshal([]byte(m.Data), &data) != nil {
			continue
		}
		t := time.UnixMilli(m.Created)
		if s.CreatedAt.IsZero() {
			s.CreatedAt = t
		}
		s.UpdatedAt = t

		if data.Role == "user" {
			var said []string
			for _, p := range parts[m.ID] {
				switch {
				case p.Type == "text" && !p.Synthetic && !p.Ignored:
					said = append(said, p.Text)
				case p.Type == "file" && p.Filename != "":
					said = append(said, "[attached "+p.Filename+"]")
				}
			}
			if words := userWords(strings.Join(said, "\n\n")); words != "" {
				msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: words})
			}
			continue
		}

		if data.ModelID != "" {
			s.Model = data.ModelID
		}
		if string(data.Summary) == "true" {
			// A compaction's summary: the model saw only this from here on.
			var sum []string
			for _, p := range parts[m.ID] {
				if p.Type == "text" {
					sum = append(sum, p.Text)
				}
			}
			msgs = []llm.Message{compacted(strings.Join(sum, "\n\n"))}
			continue
		}
		// One turn can be several steps: text, calls, their results, more
		// text. Each step is an assistant message followed by its results.
		reply := &llm.Message{Role: llm.RoleAssistant}
		var results []llm.Message
		step := func() {
			msgs = append(msgs, *reply)
			msgs = append(msgs, results...)
			reply, results = &llm.Message{Role: llm.RoleAssistant}, nil
		}
		for _, p := range parts[m.ID] {
			switch p.Type {
			case "text", "reasoning":
				if p.Synthetic || p.Ignored {
					continue
				}
				if len(reply.ToolCalls) > 0 {
					step()
				}
				if p.Type == "text" {
					reply.Content = join(reply.Content, p.Text)
				} else {
					reply.Reasoning = join(reply.Reasoning, p.Text)
				}
			case "tool":
				tc := openCodeCall(p.CallID, p.Tool, p.State.Input)
				reply.ToolCalls = append(reply.ToolCalls, tc)
				switch p.State.Status {
				case "completed":
					out := p.State.Output
					if tc.Function.Name == "edit_file" && p.State.Metadata.Diff != "" {
						if path, _ := p.State.Input["filePath"].(string); path != "" {
							out = edited(path, hunksOnly(p.State.Metadata.Diff))
						}
					}
					results = append(results, llm.Message{Role: llm.RoleTool, ToolCallID: p.CallID, Content: out})
				case "error":
					results = append(results, llm.Message{Role: llm.RoleTool, ToolCallID: p.CallID, Content: "Error: " + p.State.Error})
				}
			}
		}
		step()
	}
	s.Messages = msgs
	return s, nil
}

// openCodeCall is an OpenCode tool call as caveira's, where caveira has
// one.
func openCodeCall(id, name string, in map[string]any) llm.ToolCall {
	switch name {
	case "bash":
		return call(id, "bash", rename(in, "command", "description"))
	case "read":
		return call(id, "read_file", rename(in, "filePath:path", "offset", "limit"))
	case "write":
		return call(id, "write_file", rename(in, "filePath:path", "content"))
	case "edit":
		return call(id, "edit_file", rename(in, "filePath:path", "oldString:old_string", "newString:new_string", "replaceAll:replace_all"))
	case "glob":
		return call(id, "glob", rename(in, "pattern", "path"))
	case "grep":
		return call(id, "grep", rename(in, "pattern", "path", "include:glob"))
	case "list":
		return call(id, "list_dir", rename(in, "path"))
	}
	if in == nil {
		in = map[string]any{}
	}
	return call(id, name, in)
}

// hunksOnly drops the header a unified diff opens with (Index, ===, ---,
// +++) and keeps the hunks.
func hunksOnly(diff string) string {
	if i := strings.Index(diff, "\n@@"); i >= 0 {
		return diff[i+1:]
	}
	return diff
}
