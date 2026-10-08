package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/prompt"
)

// Item is one entry in a transcript as the window draws it.
type Item struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // user, assistant, tool, notice, paywall
	// Text is the message, or on a paywall the model the plan lacks.
	Text string `json:"text,omitempty"`
	// Reasoning is the model's thinking before an assistant reply.
	Reasoning string    `json:"reasoning,omitempty"`
	Streaming bool      `json:"streaming,omitempty"`
	Tool      *ToolView `json:"tool,omitempty"`
	// Tone colours a notice: "info" or "error".
	Tone string `json:"tone,omitempty"`
}

// ToolView is one tool call.
type ToolView struct {
	// Call is the model's ID for the call, which approvals answer to.
	Call    string `json:"call"`
	Name    string `json:"name"`
	Label   string `json:"label"` // Read, Edit, Run…
	Preview string `json:"preview"`
	Kind    string `json:"kind"`   // read, write, execute
	Status  string `json:"status"` // running, approval, done, error
	Summary string `json:"summary,omitempty"`
	Output  string `json:"output,omitempty"`
	Diff    string `json:"diff,omitempty"`
	Ms      int64  `json:"ms,omitempty"`
	// Proposed is what a write or edit is about to do, as diff lines, for
	// its approval card.
	Proposed string `json:"proposed,omitempty"`
}

// ChatView is a whole chat, for opening it.
type ChatView struct {
	ID      string  `json:"id"`
	Dir     string  `json:"dir"`
	Title   string  `json:"title"`
	Model   string  `json:"model"`
	Effort  string  `json:"effort"`
	Window  int     `json:"window"`
	Context int     `json:"context"`
	Cost    float64 `json:"cost"`
	Running bool    `json:"running"`
	// Ask says writes and commands wait for approval; otherwise every
	// permission is bypassed.
	Ask   bool   `json:"ask"`
	Items []Item `json:"items"`
	// Problem is why this chat cannot run right now; NeedsKey says the
	// problem is a missing API key.
	Problem  string `json:"problem,omitempty"`
	NeedsKey bool   `json:"needsKey,omitempty"`
}

// ChatHeader is a chat in the sidebar, under the project folder Dir.
type ChatHeader struct {
	ID      string `json:"id"`
	Dir     string `json:"dir"`
	Title   string `json:"title"`
	Updated string `json:"updated"`
	Running bool   `json:"running"`
}

// ChatEvent tells the window what changed in a chat:
//
//	item    Item was added or replaced (matched by ID)
//	delta   Text or Reasoning goes on the end of item ID
//	remove  item ID is gone
//	usage   Context and Cost are new
//	start   a turn began
//	done    the turn ended; Stats says how it went
type ChatEvent struct {
	Chat        string     `json:"chat"`
	Type        string     `json:"type"`
	Item        *Item      `json:"item,omitempty"`
	ID          string     `json:"id,omitempty"`
	Text        string     `json:"text,omitempty"`
	Reasoning   string     `json:"reasoning,omitempty"`
	Context     int        `json:"context,omitempty"`
	Cost        float64    `json:"cost,omitempty"`
	Title       string     `json:"title,omitempty"`
	Interrupted bool       `json:"interrupted,omitempty"`
	Stats       *TurnStats `json:"stats,omitempty"`
}

// TurnStats closes a turn: how long, how many tools, what it cost.
type TurnStats struct {
	Ms    int64   `json:"ms"`
	Tools int     `json:"tools"`
	Cost  float64 `json:"cost"`
}

// chat is one conversation the window has open. Its items are the
// transcript as drawn; the turn goroutine keeps them current under App.mu
// and tells the window what changed. Nothing outside that goroutine reads
// the agent while a turn runs.
type chat struct {
	id      string
	dir     string
	title   string
	ag      *agent.Agent
	gen     int
	problem string

	items   []Item
	seq     int
	byCall  map[string]string // tool call ID → item ID
	stream  string            // the assistant item being streamed, "" if none
	context int
	cost    float64

	cancel  context.CancelFunc
	replies map[string]chan<- agent.Decision
	// held is what was sent while the plan could not run this chat, kept
	// under the plans until one can.
	held []string
}

const noKeyProblem = "needs-key"

// outputCap bounds tool output kept for the window; the model has seen
// all of it already.
const outputCap = 64 << 10

func newChat(id, dir string, ag *agent.Agent) *chat {
	return &chat{id: id, dir: dir, ag: ag, byCall: map[string]string{}}
}

// build makes an agent for dir from the current settings. problem, when
// set, is why it cannot run; the agent can still show an old chat.
func (a *App) build(dir string) (ag *agent.Agent, gen int, problem string) {
	waitShellEnv()
	cfg, err := config.Load(dir)
	a.mu.Lock()
	gen = a.gen
	a.mu.Unlock()
	if err != nil {
		problem = err.Error()
	}
	if cfg.APIKey == "" && !isLocal(cfg.BaseURL) {
		problem = noKeyProblem
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		problem = fmt.Sprintf("%s is not there any more", dir)
	}
	system := prompt.Build(prompt.Options{WorkDir: dir, Model: cfg.Model, Desktop: true})
	ag = agent.New(llm.New(cfg.BaseURL, cfg.APIKey), cfg, dir, system)
	return ag, gen, problem
}

// NewChat starts a chat in dir. An empty chat already open there is
// reused, so pressing ⌘N twice does not pile them up.
func (a *App) NewChat(dir string) (ChatView, error) {
	if _, err := a.OpenProject(dir); err != nil {
		return ChatView{}, err
	}
	a.mu.Lock()
	for _, c := range a.chats {
		if c.dir == dir && len(c.items) == 0 && c.cancel == nil && c.gen == a.gen {
			v := c.view()
			a.mu.Unlock()
			return v, nil
		}
	}
	a.mu.Unlock()

	ag, gen, problem := a.build(dir)
	s := ag.NewSession()
	ag.Client.Headers["x-abliteration-session-id"] = s.ID
	c := newChat(s.ID, dir, ag)
	c.gen, c.problem = gen, problem
	c.cost = ag.Totals.CostUSD

	a.mu.Lock()
	defer a.mu.Unlock()
	a.chats[c.id] = c
	return c.view(), nil
}

// OpenChat opens a saved chat, or returns one already open. An open chat
// left idle across a settings change is brought up to date first, so the
// window shows whether it can run now.
func (a *App) OpenChat(id string) (ChatView, error) {
	a.mu.Lock()
	if c, ok := a.chats[id]; ok {
		stale := c.cancel == nil && c.gen != a.gen
		a.mu.Unlock()
		if stale {
			a.rebuild(c)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		return c.view(), nil
	}
	a.mu.Unlock()

	s, err := agent.LoadSession(id)
	if err != nil {
		return ChatView{}, fmt.Errorf("cannot open that chat: %w", err)
	}
	ag, gen, problem := a.build(s.WorkDir)
	ag.Attach(s)
	ag.Client.Headers["x-abliteration-session-id"] = s.ID
	c := newChat(s.ID, s.WorkDir, ag)
	c.title, c.gen, c.problem = s.Title, gen, problem
	c.cost = s.Totals.CostUSD
	if s.Source != "" {
		c.push(Item{Kind: "notice", Tone: "info", Text: "Imported from " + s.Source + "."})
	}
	c.load(s.Messages, ag)

	a.mu.Lock()
	defer a.mu.Unlock()
	if open, ok := a.chats[id]; ok {
		return open.view(), nil
	}
	a.chats[id] = c
	return c.view(), nil
}

// Chats lists the saved chats for dir, newest first; every project's
// when dir is empty, which the sidebar sorts into its folders. Only each
// file's header is read, so all of them is cheap.
func (a *App) Chats(dir string) ([]ChatHeader, error) {
	sessions, err := agent.ListSessions(dir)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ChatHeader, 0, len(sessions))
	for _, s := range sessions {
		h := ChatHeader{ID: s.ID, Dir: s.WorkDir, Title: s.Title, Updated: s.UpdatedAt.Format(time.RFC3339)}
		if c, ok := a.chats[s.ID]; ok {
			h.Running = c.cancel != nil
		}
		out = append(out, h)
	}
	return out, nil
}

func (a *App) DeleteChat(id string) error {
	a.mu.Lock()
	if c, ok := a.chats[id]; ok {
		if c.cancel != nil {
			a.mu.Unlock()
			return errors.New("stop this chat before deleting it")
		}
		delete(a.chats, id)
	}
	a.mu.Unlock()
	if err := agent.DeleteSession(id); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Send starts a turn, and says whether it did: a message the plan cannot
// run is held under the plans instead. It returns at once; the turn
// reports through events.
func (a *App) Send(id, text string) (bool, error) {
	if text == "" {
		return false, nil
	}
	a.mu.Lock()
	c, ok := a.chats[id]
	if !ok {
		a.mu.Unlock()
		return false, errors.New("that chat is not open")
	}
	stale := c.cancel == nil && (c.gen != a.gen || c.problem != "")
	a.mu.Unlock()
	if stale {
		a.rebuild(c)
	}

	a.mu.Lock()
	if c.cancel != nil {
		a.mu.Unlock()
		return false, errors.New("caveira is still working on the last message")
	}
	if c.problem != "" {
		p := c.problem
		a.mu.Unlock()
		if p == noKeyProblem {
			return false, errors.New("caveira cannot reach abliteration.ai: there is no key for it on this machine")
		}
		return false, errors.New(p)
	}
	model, _ := c.ag.ModelInfo()
	if ok, locked := allows(a.prefs.Plan, model); !ok {
		out := c.hold(text, locked)
		a.mu.Unlock()
		for _, e := range out {
			a.emit("chat", e)
		}
		return false, nil
	} else if len(c.held) > 0 {
		// A plan came along since: what was held goes with this.
		c.held = append(c.held, text)
		ev := c.push(Item{Kind: "user", Text: text})
		a.mu.Unlock()
		a.emit("chat", ev)
		a.release(id)
		return true, nil
	}
	ctx := c.begin()
	ev := c.push(Item{Kind: "user", Text: text})
	ag := c.ag
	a.mu.Unlock()

	a.emit("chat", ev)
	go a.turn(ctx, c, ag, text)
	return true, nil
}

// begin marks a turn as running and gives it the context Stop cancels.
// Caller holds App.mu.
func (c *chat) begin() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.replies = map[string]chan<- agent.Decision{}
	return ctx
}

// rebuild gives an idle chat an agent made from the current settings,
// keeping its conversation.
func (a *App) rebuild(c *chat) {
	ag, gen, problem := a.build(c.dir)
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.cancel != nil {
		return
	}
	old := c.ag
	ag.Session, ag.Messages, ag.Totals = old.Session, old.Messages, old.Totals
	if ag.Session != nil {
		ag.Client.Headers["x-abliteration-session-id"] = ag.Session.ID
	}
	c.ag, c.gen, c.problem = ag, gen, problem
}

func (a *App) turn(ctx context.Context, c *chat, ag *agent.Agent, text string) {
	a.emit("chat", ChatEvent{Chat: c.id, Type: "start"})
	start := time.Now()
	cost := ag.Totals.CostUSD
	tools := 0
	interrupted := false
	ag.Run(ctx, text, func(ev agent.Event) {
		switch ev := ev.(type) {
		case agent.ToolStartEvent:
			tools++
		case agent.DoneEvent:
			interrupted = ev.Interrupted
		}
		a.mu.Lock()
		out := c.apply(ev)
		a.mu.Unlock()
		for _, e := range out {
			a.emit("chat", e)
		}
	})

	a.mu.Lock()
	c.cancel, c.replies = nil, nil
	c.endStream()
	if ag.Session != nil {
		c.title = ag.Session.Title
	}
	done := ChatEvent{
		Chat: c.id, Type: "done", Title: c.title, Interrupted: interrupted,
		Stats: &TurnStats{Ms: time.Since(start).Milliseconds(), Tools: tools, Cost: ag.Totals.CostUSD - cost},
	}
	a.mu.Unlock()
	a.emit("chat", done)
}

// Stop interrupts the turn running in a chat.
func (a *App) Stop(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.chats[id]; ok && c.cancel != nil {
		c.cancel()
	}
}

func (a *App) stopAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.chats {
		if c.cancel != nil {
			c.cancel()
		}
	}
}

// Answer replies to an approval: "allow", "always", or "deny".
func (a *App) Answer(id, callID, decision string) {
	d := agent.Deny
	switch decision {
	case "allow":
		d = agent.Allow
	case "always":
		d = agent.AllowAlways
	}
	a.mu.Lock()
	c, ok := a.chats[id]
	if !ok || c.replies[callID] == nil {
		a.mu.Unlock()
		return
	}
	reply := c.replies[callID]
	delete(c.replies, callID)
	var out []ChatEvent
	if d != agent.Deny {
		if it := c.find(c.byCall[callID]); it != nil && it.Tool != nil {
			it.Tool.Status = "running"
			out = append(out, c.itemEvent(it))
		}
	}
	a.mu.Unlock()
	reply <- d
	for _, e := range out {
		a.emit("chat", e)
	}
}

// SetModel switches a chat's model and makes it the default for new ones.
// A message held for a model the plan lacks goes once the plan runs the
// new one.
func (a *App) SetModel(id, model string, window int, effort string) (ChatView, error) {
	if effort != "" && !config.ValidEffort(effort) {
		return ChatView{}, fmt.Errorf("unknown reasoning effort %q", effort)
	}
	a.mu.Lock()
	c, ok := a.chats[id]
	if !ok {
		a.mu.Unlock()
		return ChatView{}, errors.New("that chat is not open")
	}
	if c.cancel != nil {
		v := c.view()
		a.mu.Unlock()
		return v, errors.New("wait for this turn to finish before switching models")
	}
	c.ag.SetModel(model, window)
	c.ag.Effort = effort
	c.ag.System = prompt.Build(prompt.Options{WorkDir: c.dir, Model: model, Desktop: true})
	a.mu.Unlock()

	err := saveModel(model, effort)
	a.release(id)
	a.mu.Lock()
	defer a.mu.Unlock()
	return c.view(), err
}

// SetPermissions has a chat ask before writes and commands, or bypass
// every permission, and makes that the default for new chats. It works
// mid-turn: the next tool call goes by it, and bypassing answers any
// approval already waiting with Allow.
func (a *App) SetPermissions(id string, ask bool) (ChatView, error) {
	a.mu.Lock()
	c, ok := a.chats[id]
	if !ok {
		a.mu.Unlock()
		return ChatView{}, errors.New("that chat is not open")
	}
	c.ag.SetConfirm(ask)
	var waiting []string
	if !ask {
		for call := range c.replies {
			waiting = append(waiting, call)
		}
	}
	a.mu.Unlock()
	for _, call := range waiting {
		a.Answer(id, call, "allow")
	}

	err := saveConfirm(ask)
	a.mu.Lock()
	defer a.mu.Unlock()
	return c.view(), err
}

// saveConfirm makes asking first, or not, the default in config.json.
func saveConfirm(ask bool) error {
	file, err := config.ReadFile()
	if err != nil {
		return err
	}
	file.Confirm = ask
	return config.Save(file)
}

// saveModel makes model and effort the defaults in config.json.
func saveModel(model, effort string) error {
	file, err := config.ReadFile()
	if err != nil {
		return err
	}
	file.Model = unlessDefault(model, config.DefaultModel)
	file.ReasoningEffort = effort
	return config.Save(file)
}

// view is a copy of the chat for the window. Caller holds App.mu.
func (c *chat) view() ChatView {
	items := make([]Item, len(c.items))
	for i, it := range c.items {
		items[i] = copyItem(it)
	}
	model, window := c.ag.ModelInfo()
	v := ChatView{
		ID: c.id, Dir: c.dir, Title: c.title,
		Model: model, Effort: c.ag.Effort, Window: window,
		Context: c.context, Cost: c.cost, Running: c.cancel != nil, Ask: c.ag.Confirming(), Items: items,
	}
	if c.problem == noKeyProblem {
		v.NeedsKey = true
	} else {
		v.Problem = c.problem
	}
	return v
}

func copyItem(it Item) Item {
	if it.Tool != nil {
		t := *it.Tool
		it.Tool = &t
	}
	return it
}

func (c *chat) push(it Item) ChatEvent {
	c.seq++
	it.ID = fmt.Sprintf("i%d", c.seq)
	c.items = append(c.items, it)
	return c.itemEvent(&c.items[len(c.items)-1])
}

func (c *chat) itemEvent(it *Item) ChatEvent {
	cp := copyItem(*it)
	return ChatEvent{Chat: c.id, Type: "item", Item: &cp}
}

func (c *chat) find(id string) *Item {
	if id == "" {
		return nil
	}
	for i := len(c.items) - 1; i >= 0; i-- {
		if c.items[i].ID == id {
			return &c.items[i]
		}
	}
	return nil
}

func (c *chat) remove(id string) {
	for i := len(c.items) - 1; i >= 0; i-- {
		if c.items[i].ID == id {
			c.items = append(c.items[:i], c.items[i+1:]...)
			return
		}
	}
}

// streaming is the assistant item text is streaming into, made if there
// is none; fresh says it was just made.
func (c *chat) streaming() (it *Item, fresh bool) {
	if it := c.find(c.stream); it != nil {
		return it, false
	}
	c.push(Item{Kind: "assistant", Streaming: true})
	it = &c.items[len(c.items)-1]
	c.stream = it.ID
	return it, true
}

func (c *chat) endStream() {
	if it := c.find(c.stream); it != nil {
		it.Streaming = false
	}
	c.stream = ""
}

// apply folds one agent event into the transcript and says what the
// window needs to hear about it. Caller holds App.mu.
func (c *chat) apply(ev agent.Event) []ChatEvent {
	switch ev := ev.(type) {
	case agent.TextEvent:
		it, fresh := c.streaming()
		it.Text += ev.Delta
		if fresh {
			return []ChatEvent{c.itemEvent(it)}
		}
		return []ChatEvent{{Chat: c.id, Type: "delta", ID: it.ID, Text: ev.Delta}}

	case agent.ReasoningEvent:
		it, fresh := c.streaming()
		it.Reasoning += ev.Delta
		if fresh {
			return []ChatEvent{c.itemEvent(it)}
		}
		return []ChatEvent{{Chat: c.id, Type: "delta", ID: it.ID, Reasoning: ev.Delta}}

	case agent.AssistantDoneEvent:
		m := ev.Message
		it := c.find(c.stream)
		c.stream = ""
		if it == nil {
			if m.Content == "" && m.Reasoning == "" {
				return nil
			}
			return []ChatEvent{c.push(Item{Kind: "assistant", Text: m.Content, Reasoning: m.Reasoning})}
		}
		// What streamed is replaced by what the message says: a reply
		// that was a tool call written as text is gone from it.
		it.Text = m.Content
		if m.Reasoning != "" {
			it.Reasoning = m.Reasoning
		}
		it.Streaming = false
		if it.Text == "" && it.Reasoning == "" {
			id := it.ID
			c.remove(id)
			return []ChatEvent{{Chat: c.id, Type: "remove", ID: id}}
		}
		return []ChatEvent{c.itemEvent(it)}

	case agent.ToolStartEvent:
		e := c.push(Item{Kind: "tool", Tool: &ToolView{
			Call: ev.ID, Name: ev.Name, Label: toolLabel(ev.Name), Preview: ev.Preview,
			Kind: ev.Kind.String(), Status: "running", Proposed: proposed(ev.Name, ev.Args),
		}})
		c.byCall[ev.ID] = e.Item.ID
		return []ChatEvent{e}

	case agent.ApprovalEvent:
		if c.replies != nil {
			c.replies[ev.ID] = ev.Reply
		}
		it := c.find(c.byCall[ev.ID])
		if it == nil || it.Tool == nil {
			return nil
		}
		it.Tool.Status = "approval"
		return []ChatEvent{c.itemEvent(it)}

	case agent.ToolEndEvent:
		it := c.find(c.byCall[ev.ID])
		if it == nil || it.Tool == nil {
			return nil
		}
		t := it.Tool
		t.Status = "done"
		if ev.Result.IsError {
			t.Status = "error"
		}
		t.Summary = ev.Result.Summary
		t.Output = bound(ev.Result.Output)
		t.Diff = bound(ev.Result.Diff)
		t.Ms = ev.Duration.Milliseconds()
		return []ChatEvent{c.itemEvent(it)}

	case agent.UsageEvent:
		if ev.ContextTokens > 0 {
			c.context = ev.ContextTokens
		}
		c.cost = ev.Totals.CostUSD
		return []ChatEvent{{Chat: c.id, Type: "usage", Context: c.context, Cost: c.cost}}

	case agent.CompactEvent:
		c.context = 0
		return []ChatEvent{c.push(Item{Kind: "notice", Tone: "info", Text: "Earlier messages were summarized to make room."})}

	case agent.ErrorEvent:
		return []ChatEvent{c.push(Item{Kind: "notice", Tone: "error", Text: ev.Err.Error()})}

	case agent.DoneEvent:
		c.endStream()
		if ev.Interrupted {
			return []ChatEvent{c.push(Item{Kind: "notice", Tone: "info", Text: "Stopped."})}
		}
	}
	return nil
}

// proposed shows an edit as its old lines taken out and new ones put in,
// and a write as the lines it writes.
func proposed(name string, args json.RawMessage) string {
	var a struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Content   string `json:"content"`
	}
	if json.Unmarshal(args, &a) != nil {
		return ""
	}
	lines := func(s, sign string) string {
		if s == "" {
			return ""
		}
		var sb strings.Builder
		for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
			sb.WriteString(sign + l + "\n")
		}
		return sb.String()
	}
	switch name {
	case "edit_file":
		return bound(strings.TrimSuffix(lines(a.OldString, "-")+lines(a.NewString, "+"), "\n"))
	case "write_file":
		return bound(strings.TrimSuffix(lines(a.Content, "+"), "\n"))
	}
	return ""
}

func bound(s string) string {
	if len(s) <= outputCap {
		return s
	}
	return s[:outputCap] + "\n…"
}

var toolLabels = map[string]string{
	"read_file":  "Read",
	"write_file": "Write",
	"edit_file":  "Edit",
	"bash":       "Run",
	"glob":       "Find",
	"grep":       "Search",
	"list_dir":   "List",

	// Tools other agents have, in chats imported from them.
	"apply_patch": "Patch",
	"exec":        "Script",
	"Task":        "Agent",
	"Agent":       "Agent",
	"task":        "Agent",
	"WebFetch":    "Fetch",
	"webfetch":    "Fetch",
	"WebSearch":   "Web search",
	"websearch":   "Web search",
	"web_search":  "Web search",
	"TodoWrite":   "Plan",
	"todowrite":   "Plan",
	"update_plan": "Plan",
	"MultiEdit":   "Edit",
	"view_image":  "View",
}

func toolLabel(name string) string {
	if l, ok := toolLabels[name]; ok {
		return l
	}
	return name
}
