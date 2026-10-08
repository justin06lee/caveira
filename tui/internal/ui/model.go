// Package ui is the terminal session: the skull and the prompt while things
// load and until the first message, then a scrolling transcript with the
// input at the bottom, all driven by events from the agent.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/tools"
)

// Options is everything the session needs once startup work is done.
type Options struct {
	Agent    *agent.Agent
	Settings config.Settings
	WorkDir  string
	// Branch is the git branch checked out in WorkDir, if any.
	Branch string
	// ListModels fills the /model picker. Nil asks the agent's endpoint.
	ListModels func(context.Context) ([]ModelChoice, error)
	Version    string
	// Initial, when set, is sent as the first message.
	Initial string
	// Resumed, when set, is the session that was picked up.
	Resumed *agent.Session
	// Fatal, when set, disables sending and explains why.
	Fatal error
}

type phase int

const (
	phaseSplash  phase = iota // the intro plays while boot runs
	phaseHero                 // empty session: skull, centred prompt
	phaseSlide                // first prompt sent: box sliding down
	phaseSession              // transcript and bottom prompt
)

type Model struct {
	phase   phase
	boot    func() (Options, error)
	booted  bool
	bootErr error
	splash  splash

	agent   *agent.Agent
	cfg     config.Settings
	workDir string
	branch  string
	version string

	width, height int
	vp            viewport.Model
	input         textarea.Model
	rend          renderer

	// what the terminal told us about itself
	dark    bool
	bg      color.Color
	profile colorprofile.Profile

	items    []*item
	targets  []copyTarget // the copy buttons in the transcript, by line
	dirty    bool
	follow   bool
	pendingR *item // in-progress reasoning block
	pendingA *item // in-progress assistant block

	running    bool
	compacting bool
	cancel     context.CancelFunc
	events     chan agent.Event
	approval   *agent.ApprovalEvent
	apSel      int
	picker     *modelPicker
	listModels func(context.Context) ([]ModelChoice, error)
	// queued is what you sent while the model was busy, oldest first; it
	// goes out together when the turn ends.
	queued     []string
	turnStart  time.Time
	turnCost   float64 // spend before this turn, to report what it cost
	turnTokens int     // tokens used before this turn
	turnTools  int
	turnFailed bool
	verb       string // what the status line says the model is up to
	streamed   int    // characters streamed this turn, for the token estimate

	frame   int
	ticking bool

	totals  agent.Totals
	context int
	notice  string
	fatal   error

	history []string
	histPos int
	draft   string
	palSel  int

	quitArmed time.Time
	initial   string

	slideT    float64
	slideText string
}

type eventMsg struct{ ev agent.Event }
type eventsClosedMsg struct{}
type bootMsg struct {
	opts Options
	err  error
}
type shellDoneMsg struct {
	it     *item
	result tools.Result
}
type clearNoticeMsg struct{}
type tickMsg struct{}

// Run shows the intro immediately and calls boot on another goroutine;
// the session starts when both are done. Errors from boot end the program.
func Run(version string, boot func() (Options, error)) error {
	m := New(version, boot)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return err
	}
	return m.bootErr
}

// New builds the screen. boot may be nil when the caller applies Options
// itself with Apply.
func New(version string, boot func() (Options, error)) *Model {
	ta := textarea.New()
	ta.Placeholder = "Ask caveira to build, fix, or explain…"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.DynamicHeight = true
	ta.MinHeight = 1
	// MaxHeight is how tall the box grows; past that the text scrolls
	// inside it. Without MaxContentHeight the textarea would refuse new
	// lines at MaxHeight instead.
	ta.MaxHeight = 8
	ta.MaxContentHeight = 10_000
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	ta.SetVirtualCursor(false)
	ta.Focus()

	vp := viewport.New()
	vp.SoftWrap = false
	vp.MouseWheelEnabled = true
	vp.KeyMap = viewport.KeyMap{}

	m := &Model{
		phase:   phaseSplash,
		boot:    boot,
		version: version,
		input:   ta,
		vp:      vp,
		follow:  true,
		dark:    true,
		profile: colorprofile.TrueColor,
	}
	m.applyTheme()
	if boot == nil {
		m.booted = true
	}
	return m
}

// applyTheme rebuilds the palette from what the terminal has reported and
// re-renders everything drawn with the old one.
func (m *Model) applyTheme() {
	th = newTheme(m.dark, m.profile, m.bg)
	st := textarea.DefaultDarkStyles()
	plain := lipgloss.NewStyle()
	st.Focused.Base = plain
	st.Focused.CursorLine = plain
	st.Focused.Text = th.Text
	st.Focused.Placeholder = th.Faint
	st.Focused.EndOfBuffer = plain
	st.Blurred = st.Focused
	st.Cursor.Color = th.accent
	m.input.SetStyles(st)
	m.rend.md = nil // rebuilt at the current width by layout
	for _, it := range m.items {
		it.invalidate()
	}
	m.dirty = true
	m.layout()
}

// Apply installs what boot produced.
func (m *Model) Apply(o Options) {
	m.agent = o.Agent
	m.cfg = o.Settings
	m.workDir = o.WorkDir
	m.branch = o.Branch
	m.listModels = o.ListModels
	if o.Version != "" {
		m.version = o.Version
	}
	m.fatal = o.Fatal
	m.initial = o.Initial
	if o.Agent != nil {
		m.totals = o.Agent.Totals
	}
	if o.Resumed != nil {
		m.pushHeader()
		m.replay(o.Resumed)
	}
	m.booted = true
}

// replay rebuilds the transcript from a stored session.
func (m *Model) replay(s *agent.Session) {
	byID := map[string]*item{}
	for _, msg := range s.Messages {
		switch msg.Role {
		case llm.RoleUser:
			if strings.HasPrefix(msg.Content, "This session's earlier conversation was compacted.") {
				m.push(&item{kind: itemDivider, text: "context compacted"})
				continue
			}
			if strings.HasPrefix(msg.Content, "I ran this command myself") {
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
				it.result = &tools.Result{Output: msg.Content, Summary: "from the previous session", IsError: strings.HasPrefix(msg.Content, "Error:")}
			}
		}
	}
	m.push(&item{kind: itemDivider, text: "resumed " + s.ID})
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{splashTicker(), tea.RequestBackgroundColor}
	if m.boot != nil {
		boot := m.boot
		cmds = append(cmds, func() tea.Msg {
			o, err := boot()
			return bootMsg{opts: o, err: err}
		})
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

	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.bg = msg.Color
		m.applyTheme()
		return m, nil

	case tea.ColorProfileMsg:
		m.profile = msg.Profile
		m.applyTheme()
		return m, nil

	case bootMsg:
		if msg.err != nil {
			m.bootErr = msg.err
			m.booted = true
		} else {
			m.Apply(msg.opts)
		}
		if m.phase == phaseSplash && m.splash.frame >= bootFrames {
			return m.leaveSplash()
		}
		return m, nil
	}

	switch m.phase {
	case phaseSplash:
		switch msg.(type) {
		case splashTickMsg, tea.KeyPressMsg:
			return m.updateSplash(msg)
		}
	case phaseSlide:
		switch msg.(type) {
		case slideTickMsg, tea.KeyPressMsg:
			return m.updateSlide(msg)
		}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		if m.phase != phaseSession {
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.follow = m.vp.AtBottom()
		return m, cmd

	case tea.MouseClickMsg:
		if m.phase != phaseSession {
			return m, nil
		}
		return m, m.click(msg.Mouse())

	case copiedDoneMsg:
		m.copiedDone(msg)
		return m, nil

	case tickMsg:
		if !m.running && !m.hasRunningTool() && (m.picker == nil || !m.picker.loading) {
			m.ticking = false
			return m, nil
		}
		m.frame++
		m.rend.frame = m.frame
		m.dirty = true
		return m, tick()

	case eventMsg:
		return m.handleEvent(msg.ev)

	case eventsClosedMsg:
		return m.turnEnded()

	case pickerModelsMsg:
		if m.picker == nil {
			return m, nil
		}
		m.picker.loading = false
		m.picker.err = msg.err
		if len(msg.choices) > 0 {
			m.picker.setChoices(msg.choices, m.agent.Model)
		}
		m.layout()
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

func tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// startTicking runs the animation clock (spinner, shimmer, timers) until
// nothing is running.
func (m *Model) startTicking() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return tick()
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
		case "up", "k":
			m.apSel = (m.apSel + len(approvalChoices) - 1) % len(approvalChoices)
		case "down", "j", "tab":
			m.apSel = (m.apSel + 1) % len(approvalChoices)
		case "enter":
			return m.decide([]agent.Decision{agent.Allow, agent.AllowAlways, agent.Deny}[m.apSel])
		case "1", "y", "Y":
			return m.decide(agent.Allow)
		case "2", "a", "A":
			return m.decide(agent.AllowAlways)
		case "3", "n", "N", "esc", "ctrl+c":
			return m.decide(agent.Deny)
		}
		return m, nil
	}

	if m.picker != nil {
		return m.pickerKey(k)
	}

	if matches := m.paletteMatches(); len(matches) > 0 {
		sel := matches[m.palSel%len(matches)]
		switch k {
		case "up":
			m.palSel = (m.palSel + len(matches) - 1) % len(matches)
			return m, nil
		case "down":
			m.palSel = (m.palSel + 1) % len(matches)
			return m, nil
		case "tab":
			v := sel.name
			if sel.args != "" {
				v += " "
			}
			m.input.SetValue(v)
			m.input.MoveToEnd()
			m.palSel = 0
			m.layout()
			return m, nil
		case "enter":
			if sel.args != "" && m.input.Value() != sel.name && !isAlias(sel, m.input.Value()) {
				// Commands that take arguments complete first, so the
				// argument can be typed.
				m.input.SetValue(sel.name + " ")
				m.input.MoveToEnd()
				m.palSel = 0
				m.layout()
				return m, nil
			}
			m.input.SetValue(sel.name)
		}
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
			return m.flash("showing thinking")
		}
		return m.flash("hiding thinking")

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
		if n := len(m.queued); n > 0 && m.input.Value() == "" {
			// Take the newest queued message back to change it.
			m.input.SetValue(m.queued[n-1])
			m.input.MoveToEnd()
			m.queued = m.queued[:n-1]
			m.dirty = true
			m.layout()
			return m, nil
		}
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
		m.palSel = 0
		m.layout()
		m.history = append(m.history, text)
		m.histPos = len(m.history)
		m.draft = ""
		return m, m.submit(text)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.palSel = 0
	m.layout()
	return m, cmd
}

func isAlias(c command, v string) bool {
	for _, a := range c.aliases {
		if a == v {
			return true
		}
	}
	return false
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
	m.apSel = 0
	m.layout()
	return m, nil
}

func (m *Model) interrupt() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.approval != nil {
		// The agent is waiting on us; a cancelled context releases it.
		m.approval = nil
		m.layout()
	}
	m.notice = "interrupting…"
}

// submit handles slash commands and shell escapes locally, and sends
// everything else to the agent. From the home screen, the first submission
// plays the slide and comes back here once the box has landed.
func (m *Model) submit(text string) tea.Cmd {
	if m.phase == phaseHero {
		return m.beginSlide(text)
	}
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
		m.queued = append(m.queued, text)
		m.follow = true
		m.dirty = true
		return nil
	}
	return m.startTurn(text)
}

// startTurn sends one or more messages as a single turn: each gets its own
// bubble and its own place in the history, and the model answers them
// together.
func (m *Model) startTurn(texts ...string) tea.Cmd {
	for _, t := range texts {
		m.push(&item{kind: itemUser, text: t})
	}
	m.follow = true
	m.running = true
	m.notice = ""
	m.pendingA, m.pendingR = nil, nil
	m.turnStart = time.Now()
	m.turnCost = m.totals.CostUSD
	m.turnTokens = m.totals.InputTokens + m.totals.OutputTokens
	m.turnTools = 0
	m.turnFailed = false
	m.streamed = 0
	m.verb = pickVerb(m.verb)

	if m.agent.Session == nil {
		m.agent.NewSession()
	}
	for _, t := range texts[:len(texts)-1] {
		m.agent.Note(t)
	}
	text := texts[len(texts)-1]

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
	return tea.Batch(waitEvent(ch), m.startTicking())
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

// endThinking closes the reasoning block when the model moves on, so it
// can say how long it thought.
func (m *Model) endThinking() {
	if r := m.pendingR; r != nil && r.running {
		r.running = false
		r.elapsed = time.Since(r.started)
		r.invalidate()
	}
}

func (m *Model) handleEvent(ev agent.Event) (tea.Model, tea.Cmd) {
	next := waitEvent(m.events)
	switch ev := ev.(type) {
	case agent.ReasoningEvent:
		if m.pendingR == nil {
			m.pendingR = &item{kind: itemReasoning, running: true, started: time.Now()}
			m.push(m.pendingR)
		}
		m.pendingR.text += ev.Delta
		m.streamed += len(ev.Delta)
		m.pendingR.invalidate()
		m.dirty = true

	case agent.TextEvent:
		m.endThinking()
		if m.pendingA == nil {
			m.pendingA = &item{kind: itemAssistant, running: true}
			m.push(m.pendingA)
		}
		m.pendingA.text += ev.Delta
		m.streamed += len(ev.Delta)
		m.pendingA.invalidate()
		m.dirty = true

	case agent.AssistantDoneEvent:
		m.endThinking()
		if m.pendingA != nil {
			m.pendingA.text = ev.Message.Content
			m.pendingA.running = false
			m.pendingA.invalidate()
		}
		m.pendingA, m.pendingR = nil, nil
		m.dirty = true

	case agent.ToolStartEvent:
		m.endThinking()
		m.turnTools++
		m.push(&item{
			kind: itemTool, toolID: ev.ID, toolName: ev.Name, preview: ev.Preview,
			toolKind: ev.Kind, running: true, started: time.Now(),
		})

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
		// The model goes back to work on the result: a new verb for it.
		m.verb = pickVerb(m.verb)
		m.dirty = true

	case agent.ApprovalEvent:
		e := ev
		m.approval = &e
		m.apSel = 0
		m.layout()

	case agent.UsageEvent:
		m.totals = ev.Totals
		m.context = ev.ContextTokens

	case agent.CompactEvent:
		m.push(&item{kind: itemDivider, text: fmt.Sprintf("context compacted · %d messages, %s tokens → a handoff note", ev.BeforeMessages, formatTokens(ev.BeforeTokens))})

	case agent.ErrorEvent:
		m.turnFailed = true
		m.push(&item{kind: itemError, text: ev.Err.Error()})

	case agent.DoneEvent:
		if ev.Interrupted {
			m.push(&item{kind: itemDivider, text: "interrupted · tell caveira what to do instead", warn: true})
		} else if !m.compacting && !m.turnFailed {
			m.push(&item{kind: itemDivider, quiet: true, text: m.turnSummary()})
		}
	}
	return m, next
}

// turnSummary is the line that closes a turn: how long it took, how many
// tools it ran, the tokens it used, and what it cost.
func (m *Model) turnSummary() string {
	parts := []string{"worked for " + formatDuration(max(time.Since(m.turnStart).Truncate(time.Second), time.Second))}
	switch m.turnTools {
	case 0:
	case 1:
		parts = append(parts, "1 tool call")
	default:
		parts = append(parts, fmt.Sprintf("%d tool calls", m.turnTools))
	}
	if d := m.totals.InputTokens + m.totals.OutputTokens - m.turnTokens; d > 0 {
		parts = append(parts, formatTokens(d)+" tokens")
	}
	if d := m.totals.CostUSD - m.turnCost; d > 0 {
		parts = append(parts, formatCost(d))
	}
	return strings.Join(parts, " · ")
}

func (m *Model) turnEnded() (tea.Model, tea.Cmd) {
	m.running = false
	m.compacting = false
	m.cancel = nil
	m.approval = nil
	m.notice = ""
	m.endThinking()
	for _, it := range m.items {
		if it.kind == itemTool && it.running {
			it.running = false
			it.result = &tools.Result{Summary: "cancelled", IsError: true}
			it.invalidate()
		}
	}
	if m.pendingA != nil {
		m.pendingA.running = false
		m.pendingA.invalidate()
	}
	m.pendingA, m.pendingR = nil, nil
	m.dirty = true
	if m.agent != nil {
		m.totals = m.agent.Totals
	}
	m.layout()
	if len(m.queued) > 0 {
		texts := m.queued
		m.queued = nil
		return m, m.startTurn(texts...)
	}
	return m, nil
}

// shell runs a `!command` typed by the user, shown like a tool call and
// recorded in the conversation so the model knows what happened.
func (m *Model) shell(command string) tea.Cmd {
	it := &item{kind: itemTool, toolName: "bash", preview: command, toolKind: tools.KindExecute, running: true, started: time.Now(), user: true}
	m.push(it)
	m.follow = true
	reg := m.agent.Tools
	return tea.Batch(m.startTicking(), func() tea.Msg {
		args := fmt.Sprintf(`{"command": %q}`, command)
		return shellDoneMsg{it: it, result: reg.Run(context.Background(), "bash", []byte(args))}
	})
}
