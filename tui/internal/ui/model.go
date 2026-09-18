// Package ui is the caveira terminal client: the sign-in gate, the plan
// picker, and (for now) a placeholder for the session itself.
package ui

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/api"
	"github.com/justin06lee/caveira/tui/internal/config"
)

type state int

const (
	stateChecking state = iota // asking the backend who this token belongs to
	stateWelcome               // no token: log in or sign up
	stateWaiting               // device code issued, waiting on the browser
	statePlans                 // signed in, no subscription: pick one
	stateReady                 // signed in and paid
	stateFatal                 // nothing useful left to do but read the error
)

type Model struct {
	state  state
	client *api.Client

	width, height int
	spinner       spinner.Model

	me       *api.Me
	device   *api.DeviceStart
	deadline time.Time

	plans       []api.Plan
	billingMode string
	cursor      int
	notice      string

	err      error
	menu     int
	quitting bool
}

func New(token, email string) Model {
	s := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(brass)))
	m := Model{
		client:  api.New(config.BaseURL(), token),
		spinner: s,
		state:   stateWelcome,
	}
	if token != "" {
		m.state = stateChecking
		m.me = &api.Me{Email: email}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.state == stateChecking {
		return tea.Batch(m.spinner.Tick, fetchMe(m.client))
	}
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case meMsg:
		return m.handleMe(msg)

	case deviceStartedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateFatal
			return m, nil
		}
		m.device = msg.device
		m.deadline = time.Now().Add(time.Duration(msg.device.ExpiresIn) * time.Second)
		m.state = stateWaiting
		_ = openBrowser(m.loginURL())
		return m, tick(time.Duration(max(msg.device.Interval, 1)) * time.Second)

	case pollTickMsg:
		if m.state != stateWaiting || m.device == nil {
			return m, nil
		}
		return m, pollDevice(m.client, m.device.DeviceCode)

	case devicePollMsg:
		return m.handlePoll(msg)

	case plansMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateFatal
			return m, nil
		}
		m.plans = msg.plans.Plans
		m.billingMode = msg.plans.Billing
		m.state = statePlans
		return m, nil

	case checkoutMsg:
		return m.handleCheckout(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "q":
		// Everywhere except the plan picker, where the user may still be
		// deciding and a stray q should not throw the work away.
		if m.state != statePlans {
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateWelcome:
		switch msg.String() {
		case "up", "k":
			m.menu = max(m.menu-1, 0)
		case "down", "j":
			m.menu = min(m.menu+1, 1)
		case "enter":
			return m, startDevice(m.client)
		}

	case stateWaiting:
		switch msg.String() {
		case "o":
			_ = openBrowser(m.loginURL())
		case "esc":
			m.state = stateWelcome
			m.device = nil
		}

	case statePlans:
		switch msg.String() {
		case "left", "h":
			m.cursor = max(m.cursor-1, 0)
		case "right", "l":
			m.cursor = min(m.cursor+1, len(m.plans)-1)
		case "enter":
			if len(m.plans) == 0 {
				return m, nil
			}
			m.notice = "Opening checkout…"
			return m, checkout(m.client, m.plans[m.cursor].ID)
		case "r":
			m.notice = "Checking your subscription…"
			return m, fetchMe(m.client)
		case "esc":
			m.quitting = true
			return m, tea.Quit
		}

	case stateReady:
		if msg.String() == "s" {
			_ = config.Clear()
			m.client = api.New(config.BaseURL(), "")
			m.me = nil
			m.state = stateWelcome
			m.notice = "Signed out."
		}

	case stateFatal:
		if msg.String() == "enter" {
			m.err = nil
			m.state = stateWelcome
		}
	}

	return m, nil
}

func (m Model) handleMe(msg meMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		var apiErr *api.Error
		// A rejected token is a stale token: forget it and start over rather
		// than stranding the user on an error they cannot act on.
		if errors.As(msg.err, &apiErr) && apiErr.Status == 401 {
			_ = config.Clear()
			m.client = api.New(config.BaseURL(), "")
			m.me = nil
			m.state = stateWelcome
			m.notice = "Your session expired. Sign in again."
			return m, nil
		}
		m.err = msg.err
		m.state = stateFatal
		return m, nil
	}

	m.me = msg.me
	m.notice = ""
	if msg.me.HasAccess {
		m.state = stateReady
		return m, nil
	}
	m.state = stateChecking
	return m, fetchPlans(m.client)
}

func (m Model) handlePoll(msg devicePollMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.state = stateFatal
		return m, nil
	}

	switch msg.poll.Status {
	case "approved":
		if err := config.Save(config.Auth{Token: msg.poll.Token, Email: emailOf(msg.poll.User)}); err != nil {
			m.err = fmt.Errorf("signed in, but could not write %s: %w", config.Path(), err)
			m.state = stateFatal
			return m, nil
		}
		m.client = api.New(config.BaseURL(), msg.poll.Token)
		m.me = msg.poll.User
		m.device = nil
		if msg.poll.HasAccess {
			m.state = stateReady
			return m, nil
		}
		m.state = stateChecking
		return m, fetchPlans(m.client)

	case "expired":
		m.state = stateWelcome
		m.device = nil
		m.notice = "That code expired before it was approved. Try again."
		return m, nil

	default:
		return m, tick(2 * time.Second)
	}
}

func (m Model) handleCheckout(msg checkoutMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = ""
		m.err = msg.err
		m.state = stateFatal
		return m, nil
	}

	if msg.checkout.Mode == "dev" {
		m.notice = "Dev billing: plan activated without payment."
		return m, fetchMe(m.client)
	}

	if err := openBrowser(msg.checkout.URL); err != nil {
		m.notice = "Open this to pay: " + msg.checkout.URL
		return m, nil
	}
	// Stripe redirects the browser, not the terminal, so the CLI finds out by
	// asking again once the user is likely to be done.
	m.notice = "Finish checkout in your browser, then press r to refresh."
	return m, nil
}

// loginURL sends people to signup or login first, then on to the approval
// page, so a brand-new user never hits a login wall they have no account for.
func (m Model) loginURL() string {
	if m.device == nil {
		return config.BaseURL()
	}
	next := "/cli?code=" + url.QueryEscape(m.device.UserCode)
	page := "/login"
	if m.menu == 1 {
		page = "/signup"
	}
	return fmt.Sprintf("%s%s?next=%s", strings.TrimRight(config.BaseURL(), "/"), page, url.QueryEscape(next))
}

func emailOf(me *api.Me) string {
	if me == nil {
		return ""
	}
	return me.Email
}
