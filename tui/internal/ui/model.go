// Package ui is the terminal session: a scrolling transcript, an input box,
// and a status line, driven by events from the agent.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
	"github.com/justin06lee/caveira/tui/internal/tools"
)

// Options configures a session screen.
type Options struct {
	Agent    *agent.Agent
	Settings config.Settings
	WorkDir  string
	Version  string
	// Initial, when set, is sent as the first message.
	Initial string
	// Resumed, when set, is the session that was picked up.
	Resumed *agent.Session
	// Fatal, when set, disables sending and explains why.
	Fatal error
}

type Model struct {
	agent   *agent.Agent
	cfg     config.Settings
	workDir string
	version string

	width, height int
	vp            viewport.Model
	input         textarea.Model
	spin          spinner.Model
	rend          renderer

	items    []*item
	dirty    bool
	follow   bool
	pendingR *item // in-progress reasoning block
	pendingA *item // in-progress assistant block

	running  bool
	cancel   context.CancelFunc
	events   chan agent.Event
	approval *agent.ApprovalEvent
	queued   string

	totals  agent.Totals
	context int
	notice  string
	fatal   error

	history []string
	histPos int
	draft   string

	quitArmed time.Time
	initial   string
}

type eventMsg struct{ ev agent.Event }
type eventsClosedMsg struct{}
type modelsMsg struct {
	models []llm.ModelInfo
	err    error
}
type shellDoneMsg struct {
	it     *item
	result tools.Result
}
type clearNoticeMsg struct{}

func New(o Options) *Model {
	ta := textarea.New()
	ta.Placeholder = "Ask caveira to build, fix, or explain…"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 8
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	ta.SetVirtualCursor(false)
	st := textarea.DefaultDarkStyles()
	plain := lipgloss.NewStyle()
	st.Focused.Base = plain
	st.Focused.CursorLine = plain
	st.Focused.Text = styleBody
	st.Focused.Placeholder = styleDim
	st.Focused.EndOfBuffer = plain
	st.Blurred = st.Focused
	st.Cursor.Color = brass
	ta.SetStyles(st)
	ta.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(styleBrass))

	vp := viewport.New()
	vp.SoftWrap = false
	vp.MouseWheelEnabled = true
	vp.KeyMap = viewport.KeyMap{}

	m := &Model{
		agent:   o.Agent,
		cfg:     o.Settings,
		workDir: o.WorkDir,
		version: o.Version,
		input:   ta,
		spin:    sp,
		vp:      vp,
		follow:  true,
		fatal:   o.Fatal,
		initial: o.Initial,
	}
	if o.Agent != nil {
		m.totals = o.Agent.Totals
	}
	if o.Resumed != nil {
		m.replay(o.Resumed)
	}
	if o.Fatal != nil {
		m.push(&item{kind: itemError, text: o.Fatal.Error()})
	}
	return m
}

// replay rebuilds the transcript from a stored session.
func (m *Model) replay(s *agent.Session) {
	byID := map[string]*item{}
	for _, msg := range s.Messages {
		switch msg.Role {
		case llm.RoleUser:
			if strings.HasPrefix(msg.Content, "This session's earlier conversation was compacted.") {
				m.push(&item{kind: itemNotice, text: "── context compacted ──"})
				continue
			}
			m.push(&item{kind: itemUser, text: msg.Content})
		case llm.RoleAssistant:
			if strings.HasPrefix(msg.Content, "Understood. I have the state from the handoff note") {
				continue
			}
			if msg.Reasoning != "" {
				m.push(&item{kind: itemReasoning, text: msg.Reasoning})
			}
			if strings.TrimSpace(msg.Content) != "" {
				m.push(&item{kind: itemAssistant, text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				it := &item{kind: itemTool, toolID: tc.ID, toolName: tc.Function.Name}
				if t, ok := m.agent.Tools.Get(tc.Function.Name); ok {
					it.preview = t.Preview([]byte(tc.Function.Arguments))
					it.toolKind = t.Kind()
				}
				byID[tc.ID] = it
				m.push(it)
			}
		case llm.RoleTool:
			if it, ok := byID[msg.ToolCallID]; ok {
				it.result = &tools.Result{Output: msg.Content, Summary: "from previous session", IsError: strings.HasPrefix(msg.Content, "Error:")}
			}
		}
	}
	m.push(&item{kind: itemNotice, text: fmt.Sprintf("── resumed session %s ──", s.ID)})
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.initial != "" && m.fatal == nil {
		text := m.initial
		m.initial = ""
		cmds = append(cmds, m.submit(text))
	}
	return tea.Batch(cmds...)
}

func (m *Model) push(it *item) {
	m.items = append(m.items, it)
	m.dirty = true
}

// ---- update ----

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.follow = m.vp.AtBottom()
		return m, cmd

	case spinner.TickMsg:
		if !m.running && !m.hasRunningTool() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.rend.spinner = m.spin.View()
		m.dirty = true
		return m, cmd

	case eventMsg:
		return m.handleEvent(msg.ev)

	case eventsClosedMsg:
		return m.turnEnded()

	case modelsMsg:
		if msg.err != nil {
			m.push(&item{kind: itemError, text: "could not list models: " + msg.err.Error()})
			return m, nil
		}
		var sb strings.Builder
		sb.WriteString("models available to this key:\n")
		for _, mi := range msg.models {
			mark := "  "
			if mi.ID == m.agent.Model {
				mark = "› "
			}
			line := mark + mi.ID
			if mi.ContextLength > 0 {
				line += fmt.Sprintf("  ·  %s context", formatTokens(mi.ContextLength))
			}
			if mi.Pricing != nil && mi.Pricing.Prompt != "" {
				line += fmt.Sprintf("  ·  $%s in / $%s out per token", mi.Pricing.Prompt, mi.Pricing.Completion)
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("switch with /model <id>")
		m.push(&item{kind: itemNotice, text: sb.String()})
		return m, nil

	case shellDoneMsg:
		msg.it.running = false
		msg.it.result = &msg.result
		msg.it.elapsed = time.Since(msg.it.started)
		msg.it.invalidate()
		m.dirty = true
		if m.agent != nil {
			m.agent.Note(fmt.Sprintf("I ran this command myself in the working directory:\n\n$ %s\n\n%s", msg.it.preview, msg.result.Output))
		}
		return m, nil

	case clearNoticeMsg:
		m.notice = ""
		return m, nil
	}

	// Anything else (paste, blink, cursor) belongs to the textarea.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.layout()
	return m, cmd
}

func (m *Model) hasRunningTool() bool {
	for i := len(m.items) - 1; i >= 0 && i >= len(m.items)-5; i-- {
		if m.items[i].kind == itemTool && m.items[i].running {
			return true
		}
	}
	return false
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	if m.approval != nil {
		switch k {
		case "y", "Y", "enter":
			return m.decide(agent.Allow)
		case "a", "A":
			return m.decide(agent.AllowAlways)
		case "n", "N", "esc", "ctrl+c":
			return m.decide(agent.Deny)
		}
		return m, nil
	}

	switch k {
	case "ctrl+c":
		if m.running {
			m.interrupt()
			return m, nil
		}
		if m.input.Value() != "" {
			m.input.Reset()
			m.layout()
			return m, nil
		}
		if time.Since(m.quitArmed) < 2*time.Second {
			return m, tea.Quit
		}
		m.quitArmed = time.Now()
		return m.flash("press ctrl+c again to quit")

	case "ctrl+d":
		if m.input.Value() == "" && !m.running {
			return m, tea.Quit
		}

	case "esc":
		if m.running {
			m.interrupt()
			return m, nil
		}
		if m.input.Value() != "" {
			m.input.Reset()
			m.layout()
		}
		return m, nil

	case "ctrl+t":
		m.rend.showReasoning = !m.rend.showReasoning
		m.dirty = true
		if m.rend.showReasoning {
			return m.flash("showing reasoning")
		}
		return m.flash("hiding reasoning")

	case "ctrl+o":
		m.rend.expandTools = !m.rend.expandTools
		m.dirty = true
		if m.rend.expandTools {
			return m.flash("tool output expanded")
		}
		return m.flash("tool output collapsed")

	case "pgup":
		m.vp.PageUp()
		m.follow = false
		return m, nil
	case "pgdown":
		m.vp.PageDown()
		m.follow = m.vp.AtBottom()
		return m, nil
	case "ctrl+home":
		m.vp.GotoTop()
		m.follow = false
		return m, nil
	case "ctrl+end":
		m.vp.GotoBottom()
		m.follow = true
		return m, nil

	case "up":
		if m.input.LineCount() <= 1 && len(m.history) > 0 && (m.input.Value() == "" || m.histPos < len(m.history)) {
			if m.histPos == len(m.history) {
				m.draft = m.input.Value()
			}
			if m.histPos > 0 {
				m.histPos--
			}
			m.input.SetValue(m.history[m.histPos])
			m.input.MoveToEnd()
			m.layout()
			return m, nil
		}
	case "down":
		if m.histPos < len(m.history) && m.input.LineCount() <= 1 {
			m.histPos++
			if m.histPos == len(m.history) {
				m.input.SetValue(m.draft)
			} else {
				m.input.SetValue(m.history[m.histPos])
			}
			m.input.MoveToEnd()
			m.layout()
			return m, nil
		}

	case "enter":
		text := m.input.Value()
		if strings.HasSuffix(text, "\\") {
			// A trailing backslash asks for a newline, like a shell.
			m.input.SetValue(strings.TrimSuffix(text, "\\") + "\n")
			m.input.MoveToEnd()
			m.layout()
			return m, nil
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return m, nil
		}
		m.input.Reset()
		m.layout()
		m.history = append(m.history, text)
		m.histPos = len(m.history)
		m.draft = ""
		return m, m.submit(text)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.layout()
	return m, cmd
}

func (m *Model) flash(s string) (tea.Model, tea.Cmd) {
	m.notice = s
	return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearNoticeMsg{} })
}

func (m *Model) decide(d agent.Decision) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		return m, nil
	}
	m.approval.Reply <- d
	m.approval = nil
	return m, nil
}

func (m *Model) interrupt() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.approval != nil {
		// The agent is waiting on us; a cancelled context releases it.
		m.approval = nil
	}
	m.notice = "interrupting…"
}

// submit handles slash commands and shell escapes locally, and sends
// everything else to the agent.
func (m *Model) submit(text string) tea.Cmd {
	if strings.HasPrefix(text, "/") {
		return m.slash(text)
	}
	if strings.HasPrefix(text, "!") && len(text) > 1 {
		return m.shell(strings.TrimSpace(text[1:]))
	}
	if m.fatal != nil {
		m.push(&item{kind: itemError, text: m.fatal.Error()})
		return nil
	}
	if m.running {
		m.queued = text
		m.notice = "queued; sends when the current turn finishes"
		return nil
	}
	return m.startTurn(text)
}

func (m *Model) startTurn(text string) tea.Cmd {
	m.push(&item{kind: itemUser, text: text})
	m.follow = true
	m.running = true
	m.notice = ""
	m.pendingA, m.pendingR = nil, nil

	if m.agent.Session == nil {
		m.agent.NewSession()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan agent.Event, 256)
	m.events = ch
	go func() {
		defer close(ch)
		m.agent.Run(ctx, text, func(ev agent.Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
				// Still deliver, but never block forever on a UI that gave up.
				select {
				case ch <- ev:
				case <-time.After(time.Second):
				}
			}
		})
	}()
	return tea.Batch(waitEvent(ch), m.spin.Tick)
}

func waitEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return eventsClosedMsg{}
		}
		return eventMsg{ev: ev}
	}
}

func (m *Model) handleEvent(ev agent.Event) (tea.Model, tea.Cmd) {
	next := waitEvent(m.events)
	switch ev := ev.(type) {
	case agent.ReasoningEvent:
		if m.pendingR == nil {
			m.pendingR = &item{kind: itemReasoning}
			m.push(m.pendingR)
		}
		m.pendingR.text += ev.Delta
		m.pendingR.invalidate()
		m.dirty = true

	case agent.TextEvent:
		if m.pendingA == nil {
			m.pendingA = &item{kind: itemAssistant}
			m.push(m.pendingA)
		}
		m.pendingA.text += ev.Delta
		m.pendingA.invalidate()
		m.dirty = true

	case agent.AssistantDoneEvent:
		if m.pendingA != nil {
			m.pendingA.text = ev.Message.Content
			m.pendingA.invalidate()
		}
		m.pendingA, m.pendingR = nil, nil
		m.dirty = true

	case agent.ToolStartEvent:
		m.push(&item{
			kind: itemTool, toolID: ev.ID, toolName: ev.Name, preview: ev.Preview,
			toolKind: ev.Kind, running: true, started: time.Now(),
		})
		m.rend.spinner = m.spin.View()

	case agent.ToolEndEvent:
		for i := len(m.items) - 1; i >= 0; i-- {
			it := m.items[i]
			if it.kind == itemTool && it.toolID == ev.ID {
				it.running = false
				res := ev.Result
				it.result = &res
				it.elapsed = ev.Duration
				it.invalidate()
				break
			}
		}
		m.dirty = true

	case agent.ApprovalEvent:
		e := ev
		m.approval = &e
		m.layout()

	case agent.UsageEvent:
		m.totals = ev.Totals
		m.context = ev.ContextTokens

	case agent.CompactEvent:
		m.push(&item{kind: itemNotice, text: fmt.Sprintf("── context compacted: %d messages, %s tokens → handoff note ──", ev.BeforeMessages, formatTokens(ev.BeforeTokens))})

	case agent.ErrorEvent:
		m.push(&item{kind: itemError, text: ev.Err.Error()})

	case agent.DoneEvent:
		if ev.Interrupted {
			m.push(&item{kind: itemNotice, text: "── interrupted ──"})
		}
	}
	return m, next
}

func (m *Model) turnEnded() (tea.Model, tea.Cmd) {
	m.running = false
	m.cancel = nil
	m.approval = nil
	m.notice = ""
	for _, it := range m.items {
		if it.kind == itemTool && it.running {
			it.running = false
			it.result = &tools.Result{Summary: "cancelled", IsError: true}
			it.invalidate()
		}
	}
	m.pendingA, m.pendingR = nil, nil
	m.dirty = true
	if m.agent != nil {
		m.totals = m.agent.Totals
	}
	m.layout()
	if m.queued != "" {
		text := m.queued
		m.queued = ""
		return m, m.startTurn(text)
	}
	return m, nil
}

// shell runs a `!command` typed by the user, shown like a tool call and
// recorded in the conversation so the model knows what happened.
func (m *Model) shell(command string) tea.Cmd {
	it := &item{kind: itemTool, toolName: "bash", preview: command, toolKind: tools.KindExecute, running: true, started: time.Now()}
	m.push(it)
	m.follow = true
	reg := m.agent.Tools
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		args := fmt.Sprintf(`{"command": %q}`, command)
		return shellDoneMsg{it: it, result: reg.Run(context.Background(), "bash", []byte(args))}
	})
}
