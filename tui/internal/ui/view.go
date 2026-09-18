package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/config"
)

const skull = `  ▄███████▄
 ███████████
 ██ ▀█ █▀ ██
 ███  █  ███
 ▀██ ███ ██▀
  ▀███████▀`

func (m Model) View() tea.View {
	var body string
	switch m.state {
	case stateChecking:
		body = m.viewChecking()
	case stateWelcome:
		body = m.viewWelcome()
	case stateWaiting:
		body = m.viewWaiting()
	case statePlans:
		body = m.viewPlans()
	case stateReady:
		body = m.viewReady()
	case stateFatal:
		body = m.viewFatal()
	}

	v := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body))
	v.AltScreen = true
	return v
}

func (m Model) header() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(bone).Render(skull),
		"",
		titleStyle.Render("caveira"),
		subtitleStyle.Render("your model, your machine, no refusals"),
	)
}

func (m Model) viewChecking() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		m.header(),
		"",
		fmt.Sprintf("%s %s", m.spinner.View(), hintStyle.Render("checking your account…")),
	)
}

func (m Model) viewWelcome() string {
	items := []string{"Log in", "Sign up"}
	var rendered []string
	for i, item := range items {
		if i == m.menu {
			rendered = append(rendered, accentStyle.Render("› "+item))
			continue
		}
		rendered = append(rendered, hintStyle.Render("  "+item))
	}

	parts := []string{m.header(), ""}
	if m.notice != "" {
		parts = append(parts, subtitleStyle.Render(m.notice), "")
	}
	parts = append(parts,
		lipgloss.JoinVertical(lipgloss.Left, rendered...),
		"",
		hintStyle.Render("↑/↓ choose · enter continue in browser · q quit"),
	)
	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

func (m Model) viewWaiting() string {
	if m.device == nil {
		return m.viewChecking()
	}

	left := time.Until(m.deadline).Round(time.Second)
	if left < 0 {
		left = 0
	}

	return lipgloss.JoinVertical(lipgloss.Center,
		m.header(),
		"",
		bodyStyle.Render("Approve this code in your browser:"),
		"",
		codeStyle.Render(m.device.UserCode),
		"",
		subtitleStyle.Render(m.device.VerificationURL),
		"",
		fmt.Sprintf("%s %s", m.spinner.View(), hintStyle.Render(fmt.Sprintf("waiting… expires in %s", left))),
		"",
		hintStyle.Render("o open the browser again · esc cancel · q quit"),
	)
}

func (m Model) viewPlans() string {
	if len(m.plans) == 0 {
		return m.viewChecking()
	}

	// Wrap to the card's inner width ourselves, so lipgloss never re-wraps a
	// line we already broke and bullets keep their hanging indent.
	inner := cardWidth - cardStyle.GetHorizontalFrameSize()
	bodies := make([]string, 0, len(m.plans))
	for _, plan := range m.plans {
		lines := []string{
			titleStyle.Render(plan.Name),
			accentStyle.Render(fmt.Sprintf("$%d", plan.PriceUSD)) + hintStyle.Render("/"+plan.Interval),
			"",
			subtitleStyle.Render(wrap(plan.Tagline, inner)),
			"",
		}
		for _, f := range plan.Features {
			item := strings.ReplaceAll(wrap(f, inner-2), "\n", "\n  ")
			lines = append(lines, bodyStyle.Render("• "+item))
		}
		bodies = append(bodies, lipgloss.JoinVertical(lipgloss.Left, lines...))
	}

	// Equal heights, so the row reads as a set of options rather than a
	// ragged skyline, and the selection border does not jump around.
	tallest := 0
	for _, b := range bodies {
		tallest = max(tallest, lipgloss.Height(b))
	}
	cards := make([]string, 0, len(bodies))
	for i, b := range bodies {
		style := cardStyle
		if i == m.cursor {
			style = cardSelectedStyle
		}
		cards = append(cards, style.Height(tallest+cardStyle.GetVerticalFrameSize()).Render(b))
	}

	greeting := "Pick a plan to get started."
	if m.me != nil && m.me.Email != "" {
		greeting = fmt.Sprintf("Signed in as %s. Pick a plan to get started.", m.me.Email)
	}

	parts := []string{
		titleStyle.Render("caveira"),
		subtitleStyle.Render(greeting),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, cards...),
		"",
	}
	if m.billingMode == "dev" {
		parts = append(parts, hintStyle.Render("dev billing: no card required, the plan activates immediately"))
	}
	if m.notice != "" {
		parts = append(parts, accentStyle.Render(m.notice))
	}
	parts = append(parts, "", hintStyle.Render("←/→ choose · enter subscribe · r refresh · esc quit"))

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

func (m Model) viewReady() string {
	plan := "no plan"
	if m.me != nil && m.me.Subscription != nil {
		plan = fmt.Sprintf("%s · %s", m.me.Subscription.PlanID, m.me.Subscription.Status)
	}
	email := ""
	if m.me != nil {
		email = m.me.Email
	}

	return lipgloss.JoinVertical(lipgloss.Center,
		m.header(),
		"",
		bodyStyle.Render("You're in."),
		subtitleStyle.Render(email+"  ·  "+plan),
		"",
		hintStyle.Render("The agent session lands here next."),
		"",
		hintStyle.Render("s sign out · q quit"),
	)
}

func (m Model) viewFatal() string {
	msg := "Something went wrong."
	if m.err != nil {
		msg = m.err.Error()
	}
	return lipgloss.JoinVertical(lipgloss.Center,
		m.header(),
		"",
		errorStyle.Render(wrap(msg, 60)),
		"",
		hintStyle.Render(fmt.Sprintf("backend: %s", config.BaseURL())),
		"",
		hintStyle.Render("enter start over · q quit"),
	)
}

// wrap is a plain greedy word wrap: the strings here are short, and pulling in
// a reflow dependency to break three lines of marketing copy is not worth it.
func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	return strings.Join(append(lines, line), "\n")
}
