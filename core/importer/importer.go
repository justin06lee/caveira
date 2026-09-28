// Package importer brings chats from other coding agents on this machine
// into caveira: Claude Code, Codex, and OpenCode. Each one's own format is
// read into a caveira session, with the tool calls caveira also has turned
// into its own, so an imported chat reads like a native one and can be
// carried on.
//
// Only what the model saw is kept. A chat that was compacted starts from
// its summary, the way caveira's own do; text the other agent slipped into
// the user's messages (environment blocks, reminders, slash-command
// records) is left out.
package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
)

// App is an agent whose chats can be imported.
type App struct {
	ID   string // claude, codex, opencode
	Name string // what people call it
}

var (
	Claude   = App{ID: "claude", Name: "Claude Code"}
	Codex    = App{ID: "codex", Name: "Codex"}
	OpenCode = App{ID: "opencode", Name: "OpenCode"}
)

// Apps are the agents caveira knows how to read, in the order they are
// offered.
var Apps = []App{Claude, Codex, OpenCode}

// Chat is one conversation another agent left on this machine, as much
// as finding it took. Convert reads the rest.
type Chat struct {
	App     App
	ID      string // the app's own id for it
	Dir     string // the folder it ran in
	Updated time.Time
	// where is the file it is in, or OpenCode's database; title is the
	// name the app gave it, when it keeps names apart from the chat.
	where, title string
}

// SessionID is the id an imported chat gets in caveira. It is the same
// every time, so importing again skips what is already there.
func (c Chat) SessionID() string { return c.App.ID + "-" + c.ID }

// Scan finds the chats each app left under home, in folders that are
// still there and are not scratch space. An app that is not installed
// finds nothing; one that cannot be read is skipped.
func Scan(ctx context.Context, home string) []Chat {
	var out []Chat
	for _, find := range []func(context.Context, string) []Chat{scanClaude, scanCodex, scanOpenCode} {
		if ctx.Err() != nil {
			break
		}
		for _, c := range find(ctx, home) {
			if keep(c.Dir) {
				out = append(out, c)
			}
		}
	}
	return out
}

// Convert reads a chat in full. A chat with nothing the user said in it
// comes back as ErrEmpty.
func Convert(c Chat) (*agent.Session, error) {
	var (
		s   *agent.Session
		err error
	)
	switch c.App.ID {
	case Claude.ID:
		s, err = convertClaude(c)
	case Codex.ID:
		s, err = convertCodex(c)
	case OpenCode.ID:
		s, err = convertOpenCode(c)
	default:
		return nil, fmt.Errorf("no importer for %s", c.App.Name)
	}
	if err != nil {
		return nil, err
	}
	s.Messages = tidy(s.Messages)
	if !saidAnything(s.Messages) {
		return nil, ErrEmpty
	}
	s.ID = c.SessionID()
	s.Source = c.App.Name
	if s.WorkDir == "" {
		s.WorkDir = c.Dir
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = c.Updated
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = s.UpdatedAt
	}
	if s.Title == "" {
		for _, m := range s.Messages {
			if m.Role == llm.RoleUser && !strings.HasPrefix(m.Content, compactedPrefix) {
				s.Title = firstLine(m.Content, 80)
				break
			}
		}
	}
	return s, nil
}

// Each converts chats on a few cores at once, calling done with each as
// it finishes, one call at a time, in no particular order.
func Each(ctx context.Context, chats []Chat, done func(Chat, *agent.Session, error)) {
	work := make(chan Chat)
	var mu sync.Mutex
	var wg sync.WaitGroup
	// A few at a time: Claude Code chats run to tens of megabytes of JSON
	// each, and every one being read is in memory.
	for range min(4, runtime.NumCPU()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range work {
				s, err := Convert(c)
				mu.Lock()
				done(c, s, err)
				mu.Unlock()
			}
		}()
	}
	for _, c := range chats {
		if ctx.Err() != nil {
			break
		}
		work <- c
	}
	close(work)
	wg.Wait()
}

// ErrEmpty is a chat with nothing in it worth keeping.
var ErrEmpty = fmt.Errorf("nothing was said in this chat")

// keep says whether a folder is one someone works in: it is there, and it
// is not a temporary directory or inside an app bundle, where agents run
// for tests and one-off jobs.
func keep(dir string) bool {
	if dir == "" || !filepath.IsAbs(dir) {
		return false
	}
	for _, t := range Scratch {
		if dir == t || strings.HasPrefix(dir, t+"/") {
			return false
		}
	}
	if strings.Contains(dir, ".app/Contents/") {
		return false
	}
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// Scratch is where temporary directories live. Chats that ran under one
// were tests and one-off jobs, and are left where they are.
var Scratch = scratchDefaults()

func scratchDefaults() []string {
	out := []string{"/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}
	if t := strings.TrimRight(os.TempDir(), "/"); t != "" {
		out = append(out, t)
		if r, err := filepath.EvalSymlinks(t); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// The shapes caveira's own transcript reader looks for.
const (
	compactedPrefix   = "This session's earlier conversation was compacted."
	interruptedSuffix = "\n\n[interrupted by user]"
)

func compacted(summary string) llm.Message {
	return llm.Message{Role: llm.RoleUser, Content: compactedPrefix + " Here is the handoff note from the previous context:\n\n" + strings.TrimSpace(summary) + "\n\nContinue from here."}
}

// userWords is what the user typed, without the blocks agents wrap around
// it: a message that starts with <environment_context>…</environment_context>
// or ends with a <system-reminder> loses them. A message that was nothing
// but such blocks comes back empty.
func userWords(s string) string {
	s = strings.TrimSpace(s)
	for {
		name, ok := leadingTag(s)
		if !ok {
			break
		}
		end := strings.Index(s, "</"+name+">")
		if end < 0 {
			break
		}
		s = strings.TrimSpace(s[end+len(name)+3:])
	}
	for _, name := range []string{"system-reminder", "system_reminder"} {
		for {
			i := strings.Index(s, "<"+name+">")
			if i < 0 {
				break
			}
			j := strings.Index(s[i:], "</"+name+">")
			if j < 0 {
				break
			}
			s = strings.TrimSpace(s[:i] + s[i+j+len(name)+3:])
		}
	}
	if strings.HasPrefix(s, "# AGENTS.md instructions for ") || strings.HasPrefix(s, "Caveat: The messages below were generated by the user while running local commands") {
		return ""
	}
	return s
}

// leadingTag reads the name of an XML-ish tag s opens with: <name> or
// <name attr="…">.
func leadingTag(s string) (string, bool) {
	if !strings.HasPrefix(s, "<") {
		return "", false
	}
	end := strings.IndexByte(s, '>')
	if end < 2 {
		return "", false
	}
	name, _, _ := strings.Cut(s[1:end], " ")
	for i, r := range name {
		ok := r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9'
		if !ok {
			return "", false
		}
	}
	return name, true
}

// outputCap bounds one tool result. The other agent's model saw all of it;
// a model carrying the chat on needs the gist, and the sessions directory
// stays small enough to list.
const outputCap = 24 << 10

func bound(s string) string {
	if len(s) <= outputCap {
		return s
	}
	head, tail := s[:outputCap*3/4], s[len(s)-outputCap/4:]
	return fmt.Sprintf("%s\n\n[… %d bytes left out when this chat was imported …]\n\n%s", head, len(s)-len(head)-len(tail), tail)
}

// tidy makes the history one a chat-completions endpoint accepts: every
// tool call answered before the conversation moves on, no answer without
// a call, no empty assistant turns, and results cut to size.
func tidy(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	var open []llm.ToolCall // calls still waiting for a result
	answered := map[string]bool{}
	settle := func() {
		for _, c := range open {
			if !answered[c.ID] {
				out = append(out, llm.Message{Role: llm.RoleTool, ToolCallID: c.ID, Name: c.Function.Name, Content: "Error: no result was recorded for this call."})
			}
		}
		open, answered = nil, map[string]bool{}
	}
	seq := 0
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleTool:
			ok := false
			for _, c := range open {
				if c.ID == m.ToolCallID && !answered[c.ID] {
					ok = true
					m.Name = c.Function.Name
				}
			}
			if !ok {
				continue
			}
			answered[m.ToolCallID] = true
			m.Content = bound(m.Content)
			out = append(out, m)
		case llm.RoleAssistant:
			settle()
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Reasoning) == "" && len(m.ToolCalls) == 0 {
				continue
			}
			for i := range m.ToolCalls {
				if m.ToolCalls[i].ID == "" {
					seq++
					m.ToolCalls[i].ID = fmt.Sprintf("imported_%d", seq)
				}
				m.ToolCalls[i].Type = "function"
				if m.ToolCalls[i].Function.Arguments == "" {
					m.ToolCalls[i].Function.Arguments = "{}"
				}
			}
			out = append(out, m)
			open = m.ToolCalls
		default:
			settle()
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			out = append(out, m)
		}
	}
	settle()
	return out
}

func saidAnything(msgs []llm.Message) bool {
	for _, m := range msgs {
		if m.Role == llm.RoleUser && !strings.HasPrefix(m.Content, compactedPrefix) {
			return true
		}
	}
	return false
}

// call is a tool call in caveira's shape.
func call(id, name string, args any) llm.ToolCall {
	b, err := json.Marshal(args)
	if err != nil {
		b = []byte("{}")
	}
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: string(b)}}
}

// edited is an edit's result as caveira's own edit tool reports it, so
// the transcript draws the change.
func edited(path, diff string) string {
	return fmt.Sprintf("Edited %s.\n\n%s", path, strings.TrimRight(diff, "\n"))
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > n {
		s = string(r[:n]) + "…"
	}
	return s
}

// text reads a content field that is a string or a list of blocks, as
// both Claude Code and Codex write them. Images become a placeholder.
func text(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch {
		case b.Text != "":
			parts = append(parts, b.Text)
		case strings.Contains(b.Type, "image"):
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}

// envDir is the directory an app was told to use in env, or its usual one.
func envDir(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	return fallback
}

// rename copies the arguments a call was made with that caveira's tool of
// the same kind takes, under caveira's names: "file_path:path" renames,
// "pattern" keeps the name.
func rename(in map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		from, to, ok := strings.Cut(k, ":")
		if !ok {
			to = from
		}
		if v, ok := in[from]; ok && v != nil && v != "" {
			out[to] = v
		}
	}
	return out
}
