package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/art"
)

const (
	headerLines = 5 // four rows of wordmark, one blank
	statusLines = 1
	sideMargin  = 1
)

// button is a clickable region in the header. They are placeholders for
// now: clicking one says so.
type button struct {
	label          string
	x0, x1, y0, y1 int
}

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
	if m.phase == phaseHero {
		m.input.SetWidth(m.heroLayout().boxW - styleInputBox.GetHorizontalFrameSize())
	} else if m.phase == phaseSession {
		m.input.SetWidth(inner - styleInputBox.GetHorizontalFrameSize())
	}
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
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "caveira"
	if m.width == 0 {
		return v
	}

	switch m.phase {
	case phaseSplash:
		v.Content = m.renderSplash()
		return v
	case phaseHero:
		content, cur := m.renderHero(0)
		v.Content = content
		v.Cursor = cur
		return v
	case phaseSlide:
		content, _ := m.renderHero(m.slideT)
		v.Content = content
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
		bottom = m.renderInputBox(m.width - 2*sideMargin)
	}

	margin := strings.Repeat(" ", sideMargin)
	indent := func(s string) string {
		return margin + strings.ReplaceAll(s, "\n", "\n"+margin)
	}
	v.Content = lipgloss.JoinVertical(lipgloss.Left,
		indent(header),
		indent(transcript),
		indent(status),
		indent(bottom),
	)
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

// renderHeader is the wordmark on the left, the model and directory on the
// top right, and the two placeholder buttons under them. Always headerLines
// rows, the last one blank.
func (m *Model) renderHeader() string {
	inner := m.width - 2*sideMargin
	rows := make([]string, headerLines)
	m.buttons = m.buttons[:0]

	// Left: the pixel wordmark, or plain text when there is no room.
	var left []string
	leftW := art.WordmarkWidth(1)
	if inner >= leftW+24 {
		left = art.Wordmark(1, 0xD9, 0xD2, 0xC3, art.Options{})
	} else {
		left = []string{"", styleTitle.Render("caveira"), "", ""}
		leftW = 7
	}

	// Right: info line, then the buttons.
	info := ""
	if m.agent != nil {
		info = m.agent.Model
		if m.agent.Effort != "" {
			info += "  ·  effort " + m.agent.Effort
		}
		if m.cfg.Confirm {
			info += "  ·  confirm"
		}
		if m.workDir != "" {
			info += "  ·  " + shortPath(m.workDir)
		}
	}
	info = styleDim.Render(info)
	if lipgloss.Width(info) > inner-leftW-2 {
		info = styleDim.Render(shortPath(m.workDir))
	}

	btnOptions := styleButton.Render("options")
	btnTree := styleButton.Render("▤")
	btnW := lipgloss.Width(btnOptions) + 1 + lipgloss.Width(btnTree)
	showButtons := inner >= leftW+btnW+4

	optLines := strings.Split(btnOptions, "\n")
	treeLines := strings.Split(btnTree, "\n")

	for i := 0; i < headerLines-1; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		lw := leftW
		if i >= len(left) || left[i] == "" {
			lw = 0
			if i < len(left) {
				lw = lipgloss.Width(left[i])
			}
		}
		right := ""
		switch {
		case i == 0:
			right = info
		case showButtons && i-1 < len(optLines):
			right = optLines[i-1] + " " + treeLines[i-1]
		}
		gap := inner - lw - lipgloss.Width(right)
		if gap < 1 {
			right = ""
			gap = inner - lw
		}
		rows[i] = l + strings.Repeat(" ", max(gap, 0)) + right
	}
	if showButtons {
		x1 := sideMargin + inner
		treeW := lipgloss.Width(treeLines[0])
		optW := lipgloss.Width(optLines[0])
		m.buttons = append(m.buttons,
			button{label: "file tree", x0: x1 - treeW, x1: x1, y0: 1, y1: 4},
			button{label: "options", x0: x1 - treeW - 1 - optW, x1: x1 - treeW - 1, y0: 1, y1: 4},
		)
	}
	return strings.Join(rows, "\n")
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

// renderInputBox draws the prompt box at a given outer width.
func (m *Model) renderInputBox(width int) string {
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
	return box.Width(width).Render(body)
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
