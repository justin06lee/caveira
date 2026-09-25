package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
)

// command is one slash command, as the palette and /help list it.
type command struct {
	name, args, desc string
	aliases          []string
}

var commands = []command{
	{name: "/model", args: "[id]", desc: "choose the model and reasoning effort", aliases: []string{"/m", "/models"}},
	{name: "/effort", args: "[level]", desc: "reasoning effort, none to max", aliases: []string{"/e"}},
	{name: "/compact", desc: "summarize the conversation to free context"},
	{name: "/cost", desc: "tokens and spend for this session", aliases: []string{"/usage"}},
	{name: "/clear", desc: "start a new session", aliases: []string{"/new"}},
	{name: "/session", desc: "where this session is saved"},
	{name: "/help", desc: "commands and keys", aliases: []string{"/?"}},
	{name: "/quit", desc: "leave caveira", aliases: []string{"/exit", "/q"}},
}

var keyHelp = [][2]string{
	{"enter", "send"},
	{"shift+enter", "newline (also alt+enter or ctrl+j, or end a line with \\)"},
	{"esc", "interrupt the model"},
	{"ctrl+c", "interrupt, clear the input, or quit"},
	{"ctrl+t", "show or hide thinking"},
	{"ctrl+o", "expand or collapse tool output"},
	{"pgup pgdn", "scroll (or the mouse wheel)"},
	{"ctrl+end", "jump back to the bottom"},
	{"↑ ↓", "earlier prompts"},
	{"!cmd", "run a shell command yourself; the model sees the output"},
}

// ---- palette ----

// paletteMatches is what the palette shows for the current input: every
// command whose name starts with what has been typed, or nil when the
// input is not a bare command.
func (m *Model) paletteMatches() []command {
	v := m.input.Value()
	if !strings.HasPrefix(v, "/") || strings.ContainsAny(v, " \n") || m.approval != nil {
		return nil
	}
	var out []command
	for _, c := range commands {
		if strings.HasPrefix(c.name, v) {
			out = append(out, c)
			continue
		}
		for _, a := range c.aliases {
			if a == v {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func (m *Model) paletteOpen() bool { return len(m.paletteMatches()) > 0 }

// renderPalette lists matching commands under the input, the selected one
// marked.
func (m *Model) renderPalette(width int) string {
	matches := m.paletteMatches()
	if len(matches) == 0 {
		return ""
	}
	sel := m.palSel % len(matches)
	nameW := 0
	for _, c := range matches {
		nameW = max(nameW, lipgloss.Width(c.name+" "+c.args))
	}
	// Short terminals see a window of the list that follows the selection.
	rows := min(len(matches), max(3, m.height-14), 9)
	first := min(max(sel-rows/2, 0), len(matches)-rows)
	var out []string
	for i := first; i < first+rows; i++ {
		c := matches[i]
		name := c.name
		if c.args != "" {
			name += " " + c.args
		}
		name = fmt.Sprintf("%-*s", nameW, name)
		desc := oneLine(c.desc, max(width-nameW-8, 8))
		if i == sel {
			out = append(out, "  "+th.Accent.Render("›")+" "+th.Title.Render(name)+"   "+th.Text.Render(desc))
		} else {
			out = append(out, "    "+th.Muted.Render(name)+"   "+th.Faint.Render(desc))
		}
	}
	return strings.Join(out, "\n")
}

// ---- running commands ----

func (m *Model) slash(text string) tea.Cmd {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	m.push(&item{kind: itemCommand, text: text})

	switch cmd {
	case "/help", "/?":
		rows := [][2]string{}
		for _, c := range commands {
			name := c.name
			if c.args != "" {
				name += " " + c.args
			}
			rows = append(rows, [2]string{name, c.desc})
		}
		rows = append(rows, [2]string{"#Keys", ""})
		rows = append(rows, keyHelp...)
		m.push(&item{kind: itemPanel, title: "Commands", rows: rows})

	case "/quit", "/exit", "/q":
		return tea.Quit

	case "/clear", "/new":
		if m.running {
			m.push(&item{kind: itemError, text: "wait for the current turn to finish, or interrupt it first"})
			return nil
		}
		m.agent.Reset()
		m.items = nil
		m.totals = m.agent.Totals
		m.context = 0
		m.dirty = true
		m.pushHeader()

	case "/model", "/m", "/models":
		if m.running {
			m.push(&item{kind: itemError, text: "cannot switch models mid-turn"})
			return nil
		}
		if len(args) == 0 {
			return m.openPicker()
		}
		m.agent.SetModel(args[0], m.cfg.ContextWindow)
		m.cfg.Model = args[0]
		m.push(&item{kind: itemNotice, text: fmt.Sprintf("switched to %s · %s context", args[0], formatTokens(m.agent.ContextWindow))})

	case "/effort", "/e":
		if m.running {
			m.push(&item{kind: itemError, text: "cannot change the effort mid-turn"})
			return nil
		}
		if len(args) == 0 {
			return m.openPicker()
		}
		level := strings.ToLower(args[0])
		if level == "default" || level == "auto" {
			m.agent.Effort = ""
			m.push(&item{kind: itemNotice, text: "reasoning effort → model default"})
			return nil
		}
		if !config.ValidEffort(level) {
			m.push(&item{kind: itemError, text: "effort must be one of: " + strings.Join(config.Efforts, " ")})
			return nil
		}
		m.agent.Effort = level
		m.push(&item{kind: itemNotice, text: "reasoning effort → " + level})

	case "/compact":
		if m.running {
			m.push(&item{kind: itemError, text: "wait for the current turn to finish first"})
			return nil
		}
		if len(m.agent.Messages) < 2 {
			m.push(&item{kind: itemNotice, text: "nothing to compact yet"})
			return nil
		}
		return m.runCompact()

	case "/cost", "/usage":
		m.push(m.costPanel())

	case "/session":
		if m.agent.Session == nil {
			m.push(&item{kind: itemNotice, text: "no session saved yet; one is created on the first message.\nresume the latest session for this directory with: caveira -c"})
		} else {
			m.push(&item{kind: itemPanel, title: "Session", rows: [][2]string{
				{"id", m.agent.Session.ID},
				{"file", shortPath(m.agent.Session.Path())},
				{"resume", "caveira --resume " + m.agent.Session.ID},
			}})
		}

	default:
		m.push(&item{kind: itemError, text: "unknown command " + cmd + " · /help lists them"})
	}
	return nil
}

func (m *Model) costPanel() *item {
	t := m.totals
	spec := config.Spec(m.agent.Model)
	cost := formatCost(t.CostUSD)
	if spec.InputPerM == 0 {
		cost = "not tracked (no price known for this model)"
	}
	rows := [][2]string{
		{"requests", fmt.Sprintf("%d", t.Requests)},
		{"input", fmt.Sprintf("%s tokens (%s cached)", formatTokens(t.InputTokens), formatTokens(t.CachedTokens))},
		{"output", formatTokens(t.OutputTokens) + " tokens"},
		{"spend", cost},
	}
	if m.context > 0 && m.agent.ContextWindow > 0 {
		rows = append(rows, [2]string{"context", fmt.Sprintf("%s of %s (%.0f%%)", formatTokens(m.context), formatTokens(m.agent.ContextWindow), 100*float64(m.context)/float64(m.agent.ContextWindow))})
	}
	return &item{kind: itemPanel, title: "Usage", rows: rows}
}

// runCompact runs compaction as its own turn so the UI treats it like work.
func (m *Model) runCompact() tea.Cmd {
	m.running = true
	m.compacting = true
	m.turnStart = time.Now()
	m.streamed = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan agent.Event, 16)
	m.events = ch
	ag := m.agent
	go func() {
		defer close(ch)
		emit := func(ev agent.Event) { ch <- ev }
		if err := ag.Compact(ctx, emit); err != nil {
			emit(agent.ErrorEvent{Err: err})
		}
		emit(agent.UsageEvent{Totals: ag.Totals, ContextTokens: ag.LastPromptTokens})
	}()
	return tea.Batch(waitEvent(ch), m.startTicking())
}
