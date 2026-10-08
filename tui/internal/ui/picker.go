package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// The model picker is /model: a card in the input's place listing the
// models the endpoint serves, with the reasoning effort under them. ↑↓
// choose a model, ←→ set the effort, enter switches, esc leaves things as
// they were. The current model is there at once; the rest arrive when the
// endpoint answers.

// ModelChoice is one model the picker offers.
type ModelChoice struct {
	ID string
	// Context is its window in tokens; 0 when unknown.
	Context int
	// Note is shown beside it: prices, size.
	Note string
	// Unusable, when set, is why caveira cannot run on it.
	Unusable string
	// NoEffort marks a model that rejects reasoning levels.
	NoEffort bool
}

type modelPicker struct {
	choices []ModelChoice
	sel     int
	effort  int // index into effortLevels
	loading bool
	err     error
}

// effortLevels are the picker's steps: the model's own default, then the
// levels from none to max.
var effortLevels = append([]string{""}, config.Efforts...)

type pickerModelsMsg struct {
	choices []ModelChoice
	err     error
}

func effortIndex(e string) int {
	for i, l := range effortLevels {
		if l == e {
			return i
		}
	}
	return 0
}

// effortName is how a level reads on screen; unset leaves it to the model.
func effortName(e string) string {
	if e == "" {
		return "default"
	}
	return e
}

// openPicker shows the card with the current model and asks the endpoint
// for the rest.
func (m *Model) openPicker() tea.Cmd {
	cur := ModelChoice{ID: m.agent.Model, Context: m.agent.ContextWindow}
	m.picker = &modelPicker{choices: []ModelChoice{cur}, effort: effortIndex(m.agent.Effort), loading: true}
	m.layout()
	list := m.listModels
	if list == nil {
		list = apiModels(m.agent.Client)
	}
	return tea.Batch(m.startTicking(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		choices, err := list(ctx)
		return pickerModelsMsg{choices: choices, err: err}
	})
}

// apiModels lists what an OpenAI-compatible endpoint serves, with prices
// where caveira knows them. When the list cannot be fetched from
// abliteration.ai, the models caveira knows about are offered anyway.
func apiModels(client *llm.Client) func(context.Context) ([]ModelChoice, error) {
	return func(ctx context.Context) ([]ModelChoice, error) {
		models, err := client.Models(ctx)
		if err != nil && strings.TrimRight(client.BaseURL, "/") == config.DefaultBaseURL {
			for _, id := range config.KnownModels() {
				models = append(models, llm.ModelInfo{ID: id})
			}
		}
		out := make([]ModelChoice, 0, len(models))
		for _, mi := range models {
			c := ModelChoice{ID: mi.ID, Context: mi.ContextLength}
			if config.Known(mi.ID) {
				spec := config.Spec(mi.ID)
				if c.Context == 0 {
					c.Context = spec.ContextWindow
				}
				c.Note = fmt.Sprintf("$%s in · $%s out per M", price(spec.InputPerM), price(spec.OutputPerM))
			}
			out = append(out, c)
		}
		return out, err
	}
}

func price(usd float64) string { return strconv.FormatFloat(usd, 'f', -1, 64) }

// setChoices installs what the endpoint listed, keeping the selection on
// the model it was on and the current model in the list even if the
// endpoint left it out.
func (p *modelPicker) setChoices(choices []ModelChoice, current string) {
	selID := p.choices[p.sel].ID
	have := false
	for _, c := range choices {
		if c.ID == current {
			have = true
		}
	}
	if !have {
		for _, c := range p.choices {
			if c.ID == current {
				choices = append([]ModelChoice{c}, choices...)
			}
		}
	}
	p.choices = choices
	p.sel = 0
	for i, c := range choices {
		if c.ID == selID {
			p.sel = i
		}
	}
}

func (m *Model) pickerKey(k string) (tea.Model, tea.Cmd) {
	p := m.picker
	n := len(p.choices)
	switch k {
	case "up", "k", "shift+tab":
		p.sel = (p.sel + n - 1) % n
	case "down", "j", "tab":
		p.sel = (p.sel + 1) % n
	case "left", "h":
		p.effort = max(p.effort-1, 0)
	case "right", "l":
		p.effort = min(p.effort+1, len(effortLevels)-1)
	case "enter":
		return m.applyPicker()
	case "esc", "ctrl+c", "q":
		m.picker = nil
		m.layout()
	}
	return m, nil
}

// applyPicker switches to what the card shows and says so in the
// transcript.
func (m *Model) applyPicker() (tea.Model, tea.Cmd) {
	p := m.picker
	c := p.choices[p.sel]
	if c.Unusable != "" {
		return m.flash(c.ID + " " + c.Unusable)
	}
	effort := effortLevels[p.effort]
	if c.NoEffort {
		effort = ""
	}
	m.picker = nil
	m.layout()
	if c.ID == m.agent.Model && effort == m.agent.Effort {
		return m, nil
	}
	if c.ID != m.agent.Model {
		window := c.Context
		if m.cfg.ContextWindow > 0 {
			window = m.cfg.ContextWindow
		}
		m.agent.SetModel(c.ID, window)
		m.cfg.Model = c.ID
	}
	m.agent.Effort = effort
	m.push(&item{kind: itemNotice, text: fmt.Sprintf("now on %s · %s context · %s effort", m.agent.Model, formatTokens(m.agent.ContextWindow), effortName(effort))})
	return m, nil
}

// renderPicker draws the card.
func (m *Model) renderPicker() string {
	p := m.picker
	width := m.inner()
	room := width - 4

	nameW := 0
	for _, c := range p.choices {
		nameW = max(nameW, lipgloss.Width(c.ID))
	}
	// Short terminals see a window of the list that follows the selection.
	rows := min(len(p.choices), max(3, m.height-18), 8)
	first := min(max(p.sel-rows/2, 0), len(p.choices)-rows)

	var lines []string
	for i := first; i < first+rows; i++ {
		c := p.choices[i]
		mark, name := "  ", th.Muted
		if i == p.sel {
			mark, name = th.Accent.Render("›")+" ", th.Title
		}
		var info []string
		if c.Context > 0 {
			info = append(info, formatTokens(c.Context)+" context")
		}
		if c.Note != "" {
			info = append(info, c.Note)
		}
		detail := th.Faint.Render(strings.Join(info, " · "))
		if c.Unusable != "" {
			name = th.Faint
			detail = th.Warn.Render(c.Unusable)
		}
		line := mark + name.Render(fmt.Sprintf("%-*s", nameW, c.ID)) + "   " + detail
		if c.ID == m.agent.Model {
			line += th.OK.Render("  ✓")
		}
		lines = append(lines, oneLineANSI(line, room))
	}
	switch {
	case p.loading:
		lines = append(lines, th.Faint.Render("  "+m.rend.spinner()+" asking "+hostOf(m.cfg.BaseURL)+" for its models…"))
	case p.err != nil:
		lines = append(lines, oneLineANSI(th.Faint.Render("  could not list models: "+p.err.Error()), room))
	}

	lines = append(lines, "", oneLineANSI(m.effortRow(), room))
	return titledBox("Model", strings.Join(lines, "\n"), width, mix(th.line, th.muted, 0.35))
}

// effortRow is the effort setting: the level between arrows and a bar of
// how far along none…max it is.
func (m *Model) effortRow() string {
	p := m.picker
	label := th.Faint.Render("  effort   ")
	if c := p.choices[p.sel]; c.NoEffort {
		return label + th.Faint.Render(c.ID+" has no reasoning levels")
	}
	arrow := func(s string, live bool) string {
		if live {
			return th.Muted.Render(s)
		}
		return th.Line.Render(s)
	}
	level := effortLevels[p.effort]
	name := effortName(level)
	// The bar starts in the same column whichever level is showing.
	row := label + arrow("‹ ", p.effort > 0) + th.Title.Render(name) + arrow(" ›", p.effort < len(effortLevels)-1) +
		strings.Repeat(" ", len("minimal")-len(name)+3)
	if level == "" {
		return row + th.Faint.Render("the model decides")
	}
	steps := len(config.Efforts) - 1
	filled := p.effort - 1
	return row + th.Accent.Render(strings.Repeat("▰", filled)) + th.Line.Render(strings.Repeat("▱", steps-filled))
}

func hostOf(baseURL string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}
