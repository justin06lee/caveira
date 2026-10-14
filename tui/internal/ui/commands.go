package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/cloud"
	"github.com/justin06lee/caveira/core/config"
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
	{name: "/copy", desc: "copy the last code block, or the last reply if it has none"},
	{name: "/cost", desc: "tokens and spend for this session", aliases: []string{"/usage"}},
	{name: "/clear", desc: "start a new session", aliases: []string{"/new"}},
	{name: "/session", desc: "where this session is saved"},
	{name: "/login", desc: "sign in to your caveira account in the browser"},
	{name: "/logout", desc: "sign out of your caveira account"},
	{name: "/help", desc: "commands and keys", aliases: []string{"/?"}},
	{name: "/quit", desc: "leave caveira", aliases: []string{"/exit", "/q"}},
}

var accountHelp = [][2]string{
	{"/login", "opens caveira in the browser; once you connect it, requests go through your account"},
	{"API key", "for development, ABLITERATION_API_KEY or CAVEIRA_API_KEY works instead"},
}

var keyHelp = [][2]string{
	{"enter", "send"},
	{"shift+enter", "newline (also alt+enter or ctrl+j, or end a line with \\)"},
	{"esc", "interrupt the model"},
	{"ctrl+c", "interrupt, clear the input, or quit"},
	{"ctrl+t", "show or hide thinking"},
	{"ctrl+o", "expand or collapse tool output"},
	{"pgup pgdn", "scroll (or the mouse wheel)"},
	{"click copy", "copy a code block"},
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
		rows = append(rows, [2]string{"#Signing in", ""})
		rows = append(rows, accountHelp...)
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
		_, window := m.agent.ModelInfo()
		m.push(&item{kind: itemNotice, text: fmt.Sprintf("switched to %s · %s context", args[0], formatTokens(window))})

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

	case "/copy":
		return m.copyLast()

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

	case "/login":
		return m.startLogin()

	case "/logout":
		if m.running {
			m.push(&item{kind: itemError, text: "wait for the current turn to finish, or interrupt it first"})
			return nil
		}
		return m.logout()

	default:
		m.push(&item{kind: itemError, text: "unknown command " + cmd + " · /help lists them"})
	}
	return nil
}

func (m *Model) costPanel() *item {
	t := m.totals
	model, window := m.agent.ModelInfo()
	spec := config.Spec(model)
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
	if m.context > 0 && window > 0 {
		rows = append(rows, [2]string{"context", fmt.Sprintf("%s of %s (%.0f%%)", formatTokens(m.context), formatTokens(window), 100*float64(m.context)/float64(window))})
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

// ---- account ----

// signInHint is what a session without a caveira account says.
const signInHint = "not signed in · /login to sign in to your caveira account"

// KeyMissing says s has nothing to send model requests with.
// abliteration.ai's models come through a caveira account only, so a key
// of one's own for them does not count. A server set on purpose with
// --base-url or CAVEIRA_BASE_URL needs only its own key, and one on this
// machine none.
func KeyMissing(s config.Settings) bool {
	if s.OnAccount() {
		return false
	}
	if strings.TrimRight(s.BaseURL, "/") == config.DefaultBaseURL {
		return true
	}
	if s.APIKey != "" {
		return false
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return true
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return false
	}
	return true
}

// signedOut says messages wait for /login.
func (m *Model) signedOut() bool { return m.agent != nil && KeyMissing(m.cfg) }

// openBrowser opens the sign-in page; tests replace it.
var openBrowser = cloud.OpenBrowser

// pendingLogin is a /login waiting on the caveira site and the browser.
type pendingLogin struct {
	ctx    context.Context
	cancel context.CancelFunc
	// code is the one the site handed out, once it has.
	code cloud.Code
}

type loginCodeMsg struct {
	login *pendingLogin
	site  string
	code  cloud.Code
	err   error
}

type loginDoneMsg struct {
	login *pendingLogin
	auth  cloud.Auth
	err   error
}

type logoutDoneMsg struct{ err error }

// startLogin signs in to a caveira account the way Claude Code and Codex
// do: the site hands out a code, the browser opens its page, and the
// session waits in the background for the person to connect it there.
// Esc or a second /login calls a waiting one off.
func (m *Model) startLogin() tea.Cmd {
	if m.login != nil {
		m.cancelLogin()
		return nil
	}
	if a, ok := cloud.Load(); ok && !m.tokenRejected {
		m.push(&item{kind: itemNotice, text: m.signedIn(a.Email) + "\n/logout first to use another account"})
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &pendingLogin{ctx: ctx, cancel: cancel}
	m.login = l
	site := cloud.URL()
	return tea.Batch(m.startTicking(), func() tea.Msg {
		c, err := cloud.Start(ctx, site, "tui", cloud.Label())
		return loginCodeMsg{login: l, site: site, code: c, err: err}
	})
}

// loginCode opens the sign-in page once the site has handed out a code,
// says where it is in case the browser did not open, and starts the wait.
func (m *Model) loginCode(msg loginCodeMsg) tea.Cmd {
	l := msg.login
	if l != m.login {
		return nil // called off meanwhile
	}
	if msg.err != nil {
		m.login = nil
		m.push(&item{kind: itemError, text: "cannot start the login: " + msg.err.Error()})
		return nil
	}
	l.code = msg.code
	lead := "Opening your browser to sign in to caveira. If it didn't open, visit"
	if openBrowser(msg.code.VerifyURL) != nil {
		lead = "To sign in to caveira, visit"
	}
	// The address gets a line of its own so the terminal sees all of it
	// as one link.
	m.push(&item{kind: itemNotice, text: lead + "\n" + msg.code.VerifyURL +
		"\nand check the page shows the code " + msg.code.UserCode + " · esc cancels"})
	m.follow = true
	return func() tea.Msg {
		a, err := cloud.Wait(l.ctx, msg.site, msg.code)
		return loginDoneMsg{login: l, auth: a, err: err}
	}
}

// loginDone switches the session to the account once the browser has
// connected it, or says why it was not.
func (m *Model) loginDone(msg loginDoneMsg) {
	if msg.login != m.login {
		return
	}
	m.login = nil
	switch {
	case msg.err == nil:
		m.useSettings()
		m.push(&item{kind: itemNotice, text: m.signedIn(msg.auth.Email)})
	case errors.Is(msg.err, cloud.ErrDenied), errors.Is(msg.err, cloud.ErrExpired):
		m.push(&item{kind: itemError, text: msg.err.Error() + " · /login to try again"})
	default:
		m.push(&item{kind: itemError, text: "login failed: " + msg.err.Error()})
	}
	m.follow = true
}

func (m *Model) cancelLogin() {
	m.login.cancel()
	m.login = nil
	m.push(&item{kind: itemNotice, text: "login cancelled"})
}

// signedIn says who is signed in and, when this session's requests go
// somewhere else on purpose, where.
func (m *Model) signedIn(email string) string {
	if email == "" {
		email = "your caveira account"
	}
	s := "Signed in as " + email
	if !m.cfg.OnAccount() {
		s += "\nthis session keeps sending its requests to " + m.cfg.BaseURL + ", as configured"
	}
	return s
}

// logout ends the account's session on the site and forgets it here; the
// session falls back on whatever key the configuration has, if any.
func (m *Model) logout() tea.Cmd {
	if m.login != nil {
		m.cancelLogin()
	}
	if _, ok := cloud.Load(); !ok {
		m.push(&item{kind: itemNotice, text: "not signed in"})
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return logoutDoneMsg{err: cloud.Logout(ctx)}
	}
}

func (m *Model) logoutDone(msg logoutDoneMsg) {
	m.useSettings()
	if _, ok := cloud.Load(); ok {
		m.push(&item{kind: itemError, text: fmt.Sprintf("cannot sign out: %v", msg.err)})
		return
	}
	text := "Signed out"
	if msg.err != nil {
		// The token is gone from here; the site may still take it.
		text += "\nthe caveira site could not be told: " + msg.err.Error()
	}
	switch {
	case m.signedOut():
		text += "\n/login to sign in again"
	case m.cfg.KeySource != "":
		text += "\nrequests now use the API key from " + keySourceName(m.cfg.KeySource)
	default:
		text += "\nrequests now go to " + m.cfg.BaseURL
	}
	m.push(&item{kind: itemNotice, text: text})
}

// keySourceName is where a key came from, the way a sentence says it.
func keySourceName(src string) string {
	switch src {
	case "environment":
		return "the environment"
	case "flag":
		return "--api-key"
	}
	return src
}
