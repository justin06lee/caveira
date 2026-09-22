package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const skull = `  ▄███████▄
 ███████████
 ██ ▀█ █▀ ██
 ███  █  ███
 ▀██ ███ ██▀
  ▀███████▀`

const (
	headerLines = 2
	statusLines = 1
	sideMargin  = 1
)

// layout recomputes the sizes of the moving parts. Called on resize, on
// every input change (the box grows with its content), and when an approval
// box replaces the input.
func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	inner := m.width - 2*sideMargin
	if inner < 20 {
		inner = 20
	}
	m.input.SetWidth(inner - styleInputBox.GetHorizontalFrameSize())
	bottom := m.bottomHeight()
	vh := m.height - headerLines - statusLines - bottom
	if vh < 3 {
		vh = 3
	}
	m.vp.SetWidth(inner)
	m.vp.SetHeight(vh)
	if m.rend.width != inner {
		m.rend.width = inner
		m.rend.md = newMarkdown(inner - 4)
		for _, it := range m.items {
			it.invalidate()
		}
		m.dirty = true
	}
}

func (m *Model) bottomHeight() int {
	if m.approval != nil {
		return lipgloss.Height(m.renderApproval())
	}
	return m.input.Height() + styleInputBox.GetVerticalFrameSize()
}

func (m *Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	m.refreshTranscript()

	header := m.renderHeader()
	transcript := m.vp.View()
	status := m.renderStatus()
	var bottom string
	if m.approval != nil {
		bottom = m.renderApproval()
	} else {
		bottom = m.renderInput()
	}

	margin := strings.Repeat(" ", sideMargin)
	indent := func(s string) string {
		return margin + strings.ReplaceAll(s, "\n", "\n"+margin)
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		indent(header),
		indent(transcript),
		indent(status),
		indent(bottom),
	)

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "caveira"
	if m.approval == nil && m.input.Focused() {
		if cur := m.input.Cursor(); cur != nil {
			cur.Position.X += sideMargin + 2 + 1 // margin, border+padding, prompt mark
			cur.Position.Y += headerLines + m.vp.Height() + statusLines + 1
			v.Cursor = cur
		}
	}
	return v
}

// refreshTranscript re-renders changed items into the viewport.
func (m *Model) refreshTranscript() {
	if !m.dirty {
		return
	}
	m.dirty = false
	if len(m.items) == 0 {
		m.vp.SetContent(m.renderWelcome())
		m.vp.GotoTop()
		return
	}
	parts := make([]string, 0, len(m.items))
	for _, it := range m.items {
		s := m.rend.render(it)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	m.vp.SetContent(strings.Join(parts, "\n\n"))
	if m.follow {
		m.vp.GotoBottom()
	}
}

func (m *Model) renderHeader() string {
	left := styleTitle.Render("caveira") + styleDim.Render("  ·  ") + styleSlate.Render(m.agent.Model)
	if m.agent.Effort != "" {
		left += styleDim.Render("  ·  effort ") + styleSlate.Render(m.agent.Effort)
	}
	if m.cfg.Confirm {
		left += styleDim.Render("  ·  ") + styleBrass.Render("confirm")
	}
	right := styleDim.Render(shortPath(m.workDir))
	inner := m.width - 2*sideMargin
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		right = ""
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right + "\n"
}

func (m *Model) renderStatus() string {
	inner := m.width - 2*sideMargin
	var left string
	switch {
	case m.approval != nil:
		left = styleBrass.Render("waiting for your decision")
	case m.running:
		what := "thinking"
		for i := len(m.items) - 1; i >= 0; i-- {
			it := m.items[i]
			if it.kind == itemTool && it.running {
				what = it.toolName
				break
			}
			if it.kind == itemAssistant && it.text != "" {
				what = "writing"
				break
			}
		}
		left = styleBrass.Render(m.spin.View()) + " " + styleDim.Render(what+"…") + styleSlate.Render("  esc to interrupt")
		if m.queued != "" {
			left += styleSlate.Render("  ·  1 message queued")
		}
	case m.notice != "":
		left = styleBrass.Render(m.notice)
	default:
		left = styleSlate.Render("enter send · alt+enter newline · / commands · ! shell · ctrl+c quit")
	}
	if m.notice != "" && m.running {
		left = styleBrass.Render(m.notice)
	}

	var right string
	if m.totals.Requests > 0 {
		right = fmt.Sprintf("%s in · %s out · %s", formatTokens(m.totals.InputTokens), formatTokens(m.totals.OutputTokens), formatCost(m.totals.CostUSD))
		if m.context > 0 && m.agent.ContextWindow > 0 {
			pct := 100 * float64(m.context) / float64(m.agent.ContextWindow)
			ctx := fmt.Sprintf("ctx %.0f%%", pct)
			if pct >= 70 {
				ctx = styleBrass.Render(ctx)
			}
			right = ctx + " · " + right
		}
		right = styleDim.Render(right)
	}
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		right = ""
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) renderInput() string {
	box := styleInputBox
	if m.running {
		box = styleInputBoxBusy
	}
	mark := styleUserMark.Render("❯")
	if m.running {
		mark = styleSlate.Render("❯")
	}
	lines := m.input.Height()
	marks := mark
	for i := 1; i < lines; i++ {
		marks += "\n" + " "
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, marks+" ", m.input.View())
	return box.Width(m.width - 2*sideMargin).Render(body)
}

func (m *Model) renderApproval() string {
	a := m.approval
	if a == nil {
		return ""
	}
	verb := "wants to run"
	if a.Kind.String() == "write" {
		verb = "wants to change"
	}
	head := styleEmber.Render("⚠ ") + styleToolName.Render(a.Name) + styleBody.Render(" "+verb+":")
	preview := a.Preview
	if preview == "" {
		preview = "(see the transcript above)"
	}
	inner := m.width - 2*sideMargin - styleApprovalBox.GetHorizontalFrameSize()
	body := lipgloss.Wrap(styleBrass.Render(preview), inner-2, "")
	body = "  " + strings.ReplaceAll(body, "\n", "\n  ")
	keys := styleKey.Render("[y]") + styleDim.Render(" run once   ") +
		styleKey.Render("[a]") + styleDim.Render(" always allow "+a.Name+"   ") +
		styleKey.Render("[n]") + styleDim.Render(" deny")
	return styleApprovalBox.Width(m.width - 2*sideMargin).Render(head + "\n" + body + "\n" + keys)
}

func (m *Model) renderWelcome() string {
	tips := []string{
		"Describe what you want built, fixed, or explained.",
		"caveira reads and edits files, runs commands, and checks its work.",
		"",
		styleSlate.Render("/help for commands  ·  /model to switch models  ·  esc interrupts"),
	}
	block := lipgloss.JoinVertical(lipgloss.Center,
		styleBody.Render(skull),
		"",
		styleTitle.Render("caveira")+styleDim.Render("  "+m.version),
		styleSlate.Render("your model, your machine, no refusals"),
		"",
		styleDim.Render(strings.Join(tips, "\n")),
	)
	if m.fatal != nil {
		block = lipgloss.JoinVertical(lipgloss.Center, block, "", styleEmber.Render(lipgloss.Wrap(m.fatal.Error(), min(m.vp.Width()-4, 80), "")))
	}
	return lipgloss.Place(m.vp.Width(), m.vp.Height(), lipgloss.Center, lipgloss.Center, block)
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	if len(p) > 48 {
		parts := strings.Split(p, string(filepath.Separator))
		if len(parts) > 3 {
			p = "…" + string(filepath.Separator) + filepath.Join(parts[len(parts)-3:]...)
		}
	}
	return p
}
