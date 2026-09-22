package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
)

const helpText = `commands
  /model [id]      show or switch the model (abliterated-model, abliterated-model-large-v2, …)
  /models          list the models this key can use
  /effort [level]  reasoning effort: none minimal low medium high xhigh max
  /compact         summarize the conversation to free context
  /cost            tokens and cost for this session
  /clear           start a new session
  /session         where this session is saved
  /help            this
  /quit            leave
  !<command>       run a shell command yourself; the output goes to the model too

keys
  enter            send            alt+enter, ctrl+j   newline (or end a line with \)
  esc              interrupt       ctrl+c              interrupt, clear, or quit
  ctrl+t           show thinking   ctrl+o              expand tool output
  pgup / pgdn      scroll          up / down           recall earlier prompts`

func (m *Model) slash(text string) tea.Cmd {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "/help", "/?":
		m.push(&item{kind: itemNotice, text: helpText})

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
		m.push(&item{kind: itemNotice, text: "new session"})

	case "/model", "/m":
		if len(args) == 0 {
			m.push(&item{kind: itemNotice, text: fmt.Sprintf("model: %s  ·  context window %s  ·  endpoint %s", m.agent.Model, formatTokens(m.agent.ContextWindow), m.cfg.BaseURL)})
			return nil
		}
		if m.running {
			m.push(&item{kind: itemError, text: "cannot switch models mid-turn"})
			return nil
		}
		m.agent.SetModel(args[0], m.cfg.ContextWindow)
		m.cfg.Model = args[0]
		m.push(&item{kind: itemNotice, text: fmt.Sprintf("model → %s  ·  context window %s", args[0], formatTokens(m.agent.ContextWindow))})

	case "/models":
		client := m.agent.Client
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			models, err := client.Models(ctx)
			return modelsMsg{models: models, err: err}
		}

	case "/effort", "/e":
		if len(args) == 0 {
			cur := m.agent.Effort
			if cur == "" {
				cur = "model default"
			}
			m.push(&item{kind: itemNotice, text: "reasoning effort: " + cur + "  ·  levels: " + strings.Join(config.Efforts, " ")})
			return nil
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
		m.push(&item{kind: itemNotice, text: m.costText()})

	case "/session":
		if m.agent.Session == nil {
			m.push(&item{kind: itemNotice, text: "no session saved yet; one is created on the first message.\nresume the latest session for this directory with: caveira -c"})
		} else {
			m.push(&item{kind: itemNotice, text: fmt.Sprintf("session %s\n%s\nresume with: caveira --resume %s", m.agent.Session.ID, m.agent.Session.Path(), m.agent.Session.ID)})
		}

	default:
		m.push(&item{kind: itemError, text: "unknown command " + cmd + "  ·  /help lists them"})
	}
	return nil
}

func (m *Model) costText() string {
	t := m.totals
	spec := config.Spec(m.agent.Model)
	var sb strings.Builder
	fmt.Fprintf(&sb, "requests %d  ·  input %s (%s cached)  ·  output %s  ·  %s",
		t.Requests, formatTokens(t.InputTokens), formatTokens(t.CachedTokens), formatTokens(t.OutputTokens), formatCost(t.CostUSD))
	if spec.InputPerM == 0 {
		sb.WriteString("\n(no price known for this model; cost not tracked)")
	}
	if m.context > 0 && m.agent.ContextWindow > 0 {
		fmt.Fprintf(&sb, "\ncontext %s of %s (%.0f%%)", formatTokens(m.context), formatTokens(m.agent.ContextWindow), 100*float64(m.context)/float64(m.agent.ContextWindow))
	}
	return sb.String()
}

// runCompact runs compaction as its own turn so the UI treats it like work.
func (m *Model) runCompact() tea.Cmd {
	m.running = true
	m.notice = "compacting…"
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
	return tea.Batch(waitEvent(ch), m.spin.Tick)
}
