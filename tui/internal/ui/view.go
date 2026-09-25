package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/config"
)

// The session screen, top to bottom: the transcript, a gap, the status line
// (what the model is doing), the input box (or an approval card, or the
// model picker), and the footer (keys on the left; model, effort, and what
// the session has used on the right). The command palette takes the
// footer's place while you type a slash command.

const (
	sideMargin = 1
	gapLines   = 1
	statusRows = 1
	footerRows = 1
)

// inputChrome is how much narrower the text is than the box around it:
// a border and a column of padding on each side, and the two-cell prompt
// mark. inputLeft is the part of that on the left, where the text starts.
const (
	inputChrome = 2 + 2 + 2
	inputLeft   = 1 + 1 + 2
)

// layout recomputes the sizes of the moving parts. Called on resize, on
// every input change (the box grows with its content), and when an approval
// card replaces the input.
func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	inner := m.inner()
	switch m.phase {
	case phaseHero, phaseSplash:
		m.input.SetWidth(m.homeLayout().boxW - inputChrome)
	case phaseSession:
		m.input.SetWidth(inner - inputChrome)
	}
	vh := max(m.height-m.bottomHeight(), 1)
	m.vp.SetWidth(inner)
	m.vp.SetHeight(vh)
	if m.rend.width != inner || m.rend.md == nil {
		m.rend.width = inner
		m.rend.md = newMarkdown(inner - 2)
		for _, it := range m.items {
			it.invalidate()
		}
		m.dirty = true
	}
}

func (m *Model) inner() int { return max(m.width-2*sideMargin, 20) }

// bottomHeight is everything under the transcript.
func (m *Model) bottomHeight() int {
	h := gapLines + statusRows
	switch {
	case m.approval != nil:
		h += lipgloss.Height(m.renderApproval())
	case m.picker != nil:
		h += lipgloss.Height(m.renderPicker())
	default:
		h += m.input.Height() + 2
	}
	if p := m.renderPalette(m.inner()); p != "" {
		h += lipgloss.Height(p)
	} else {
		h += footerRows
	}
	return h
}

func (m *Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "caveira"
	if m.workDir != "" {
		v.WindowTitle = "caveira · " + filepath.Base(m.workDir)
	}
	if m.width == 0 {
		return v
	}

	switch m.phase {
	case phaseSplash, phaseHero:
		content, cur := m.renderHome(0)
		v.Content = content
		v.Cursor = cur
		return v
	case phaseSlide:
		content, _ := m.renderHome(m.slideT)
		v.Content = content
		return v
	}

	m.refreshTranscript()
	inner := m.inner()
	var bottom []string
	bottom = append(bottom, strings.Repeat("\n", gapLines-1))
	bottom = append(bottom, m.renderStatus(inner))
	boxY := m.vp.Height() + gapLines + statusRows
	switch {
	case m.approval != nil:
		bottom = append(bottom, m.renderApproval())
	case m.picker != nil:
		bottom = append(bottom, m.renderPicker())
	default:
		bottom = append(bottom, m.renderInputBox(inner))
	}
	if p := m.renderPalette(inner); p != "" {
		bottom = append(bottom, p)
	} else {
		bottom = append(bottom, m.renderFooter(inner))
	}

	margin := strings.Repeat(" ", sideMargin)
	v.Content = indent(m.vp.View()+"\n"+strings.Join(bottom, "\n"), margin)
	if m.approval == nil && m.picker == nil && m.input.Focused() {
		if cur := m.input.Cursor(); cur != nil {
			cur.Position.X += sideMargin + inputLeft
			cur.Position.Y += boxY + 1
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
	parts := make([]string, 0, len(m.items)+len(m.queued)+1)
	for _, it := range m.items {
		s := m.rend.render(it)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	parts = append(parts, m.renderQueued()...)
	m.vp.SetContent(strings.Join(parts, "\n\n"))
	if m.follow {
		m.vp.GotoBottom()
	}
}

// renderQueued draws the messages waiting for the turn to end, under
// everything else, as dashed bubbles with a note on when they go.
func (m *Model) renderQueued() []string {
	if len(m.queued) == 0 {
		return nil
	}
	width := m.inner()
	var out []string
	for _, q := range m.queued {
		text := strings.ReplaceAll(strings.TrimRight(q, "\n"), "\t", "    ")
		out = append(out, draftBubble(text, width, th.muted, th.faint))
	}
	note := th.Faint.Render("queued · sends when this turn ends · ") + th.Muted.Render("↑") + th.Faint.Render(" to edit")
	last := len(out) - 1
	out[last] += "\n" + lipgloss.PlaceHorizontal(width-1, lipgloss.Right, note)
	return out
}

// ---- status ----

// activity is what the model is doing right now, in a few words: the
// command or file when a tool runs, and this step's rebel verb while the
// model thinks or writes.
func (m *Model) activity() string {
	if m.compacting {
		return "Compacting the conversation"
	}
	for i := len(m.items) - 1; i >= 0 && m.items[i].kind != itemUser; i-- {
		if it := m.items[i]; it.kind == itemTool && it.running {
			return toolActivity(it)
		}
	}
	if m.verb == "" {
		return "Thinking"
	}
	return m.verb
}

func toolActivity(it *item) string {
	arg := it.preview
	switch it.toolName {
	case "read_file":
		return "Reading " + filepath.Base(arg)
	case "edit_file":
		return "Editing " + filepath.Base(arg)
	case "write_file":
		return "Writing " + filepath.Base(arg)
	case "bash":
		f := strings.Fields(arg)
		switch {
		case len(f) > 1 && !strings.HasPrefix(f[1], "-") && len(f[1]) <= 12 && !strings.ContainsAny(f[1], "/.&|;"):
			return "Running " + f[0] + " " + f[1]
		case len(f) > 0:
			return "Running " + f[0]
		}
		return "Running a command"
	case "grep":
		return "Searching"
	case "glob":
		return "Finding files"
	case "list_dir":
		return "Looking around"
	}
	return "Working"
}

func (m *Model) renderStatus(width int) string {
	switch {
	case m.approval != nil:
		return th.Warn.Render("◆ ") + th.Text.Render("Waiting for your go-ahead")
	case m.running:
		line := th.Accent.Render(m.rend.spinner()) + " " + shimmer(m.activity()+"…", m.frame)
		var meta []string
		meta = append(meta, formatDuration(max(time.Since(m.turnStart).Truncate(time.Second), time.Second)))
		if m.streamed > 0 {
			meta = append(meta, "↓ "+formatTokens(m.streamed/4)+" tokens")
		}
		switch n := len(m.queued); n {
		case 0:
		case 1:
			meta = append(meta, "1 message queued")
		default:
			meta = append(meta, fmt.Sprintf("%d messages queued", n))
		}
		line += th.Faint.Render("  " + strings.Join(meta, " · "))
		if m.notice != "" {
			line += th.Faint.Render(" · ") + th.Warn.Render(m.notice)
		}
		return oneLineANSI(line, width)
	case m.notice != "":
		return th.Muted.Render(m.notice)
	case !m.follow:
		return th.Faint.Render("↓ more below · ") + th.Muted.Render("ctrl+end") + th.Faint.Render(" to jump back")
	}
	return ""
}

// shimmer draws text with a soft highlight sweeping across it.
func shimmer(text string, frame int) string {
	rs := []rune(text)
	pos := float64(frame%(len(rs)+12)) - 6
	var sb strings.Builder
	for i, r := range rs {
		d := math.Abs(float64(i) - pos)
		k := math.Max(0, 1-d/4)
		c := mix(th.muted, th.text, 0.35+0.65*k)
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
	}
	return sb.String()
}

// oneLineANSI cuts a styled line to a width.
func oneLineANSI(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// ---- input ----

// renderInputBox draws the prompt box at a given outer width.
func (m *Model) renderInputBox(width int) string {
	border := mix(th.line, th.muted, 0.35)
	mark := th.Accent.Render("›")
	switch {
	case m.fatal != nil:
		border = th.err
	case m.running:
		border = th.line
		mark = th.Faint.Render("›")
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width)
	// The mark column is inputLeft's two cells on every row, so the text
	// sits where the cursor is placed however many lines the box shows.
	marks := make([]string, m.input.Height())
	marks[0] = mark + " "
	for i := 1; i < len(marks); i++ {
		marks[i] = "  "
	}
	return box.Render(lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(marks, "\n"), m.input.View()))
}

// renderFooter is the row under the input: the keys that matter right now
// on the left, the model and what the session has used on the right.
func (m *Model) renderFooter(width int) string {
	key := func(k, what string) string { return th.Muted.Render(k) + " " + th.Faint.Render(what) }
	sep := th.Faint.Render("  ·  ")
	var hints []string
	switch {
	case m.approval != nil:
		hints = []string{key("↑↓", "choose"), key("enter", "confirm"), key("esc", "deny")}
	case m.picker != nil:
		hints = []string{key("↑↓", "model"), key("←→", "effort"), key("enter", "switch"), key("esc", "cancel")}
	case m.running:
		hints = []string{key("esc", "interrupt"), key("enter", "queue a message")}
		if len(m.queued) > 0 && m.input.Value() == "" {
			hints = []string{key("esc", "interrupt"), key("↑", "edit queued"), key("enter", "queue another")}
		}
	case m.input.Value() != "":
		hints = []string{key("enter", "send"), key("shift+enter", "newline")}
	default:
		hints = []string{key("/", "commands"), key("!", "shell"), key("ctrl+t", "thinking"), key("ctrl+o", "output")}
	}
	// Two hints stay if the info fits beside them without losing more than
	// the spend and the meter; otherwise one does, and the info gives up
	// what it must. Hints past those drop from the end.
	const gutter = 4 // the least space between the hints and the info
	room := func(n int) int {
		return width - lipgloss.Width("  "+strings.Join(hints[:min(n, len(hints))], sep)) - gutter
	}
	right, ok := m.footerInfo(room(2), keepMeter+1)
	if !ok {
		right, _ = m.footerInfo(room(1), keepAll)
	}
	for len(hints) > 1 && lipgloss.Width("  "+strings.Join(hints, sep))+lipgloss.Width(right)+gutter > width {
		hints = hints[:len(hints)-1]
	}
	left := "  " + strings.Join(hints, sep)
	if lipgloss.Width(left)+lipgloss.Width(right)+gutter > width {
		right = ""
	}
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// How much the footer info wants to keep each part: lower gives way first.
const (
	keepSpend = iota
	keepMeter
	keepTokens
	keepConfirm
	keepEffort
	keepModel
	keepAll
)

// footerInfo is the right of the footer in at most room cells: the model,
// the effort, the tokens used, how full the context is, and the spend. To
// fit, it drops parts that matter less than below, least first; ok is
// false when that is not enough.
func (m *Model) footerInfo(room, below int) (info string, ok bool) {
	if m.agent == nil {
		return "", true
	}
	type part struct {
		s    string
		keep int // lower goes first
	}
	model := th.Muted.Render(m.agent.Model)
	if m.dev {
		model = devBadge() + " " + model
	}
	parts := []part{{model, keepModel}, {th.Faint.Render(effortName(m.agent.Effort) + " effort"), keepEffort}}
	if m.cfg.Confirm {
		parts = append(parts, part{th.Warn.Render("confirm"), keepConfirm})
	}
	if m.context > 0 && m.agent.ContextWindow > 0 {
		parts = append(parts, part{contextMeter(float64(m.context) / float64(m.agent.ContextWindow)), keepMeter})
	}
	if n := m.totals.InputTokens + m.totals.OutputTokens; n > 0 {
		parts = append(parts, part{th.Faint.Render(formatTokens(n) + " tokens"), keepTokens})
	}
	if m.totals.Requests > 0 && config.Spec(m.agent.Model).InputPerM > 0 {
		// Local and unknown models have no price; $0.00 would be a guess.
		parts = append(parts, part{th.Faint.Render(formatCost(m.totals.CostUSD)), keepSpend})
	}
	join := func() string {
		ss := make([]string, len(parts))
		for i, p := range parts {
			ss[i] = p.s
		}
		return strings.Join(ss, th.Faint.Render(" · ")) + " "
	}
	for len(parts) > 1 && lipgloss.Width(join()) > room {
		low := 0
		for i, p := range parts {
			if p.keep < parts[low].keep {
				low = i
			}
		}
		if parts[low].keep >= below {
			return join(), false
		}
		parts = append(parts[:low], parts[low+1:]...)
	}
	return join(), lipgloss.Width(join()) <= room
}

// devBadge marks a session running on a local model, so it is never
// mistaken for the real thing.
func devBadge() string {
	return lipgloss.NewStyle().Foreground(th.warn).Bold(true).Render("DEV")
}

// contextMeter is a small bar of how full the context window is.
func contextMeter(frac float64) string {
	const cells = 8
	frac = math.Min(math.Max(frac, 0), 1)
	filled := int(math.Round(frac * cells))
	if frac > 0 && filled == 0 {
		filled = 1
	}
	st := th.Muted
	switch {
	case frac >= 0.9:
		st = th.Err
	case frac >= 0.7:
		st = th.Warn
	}
	return st.Render(strings.Repeat("▰", filled)) + th.Line.Render(strings.Repeat("▱", cells-filled)) +
		" " + st.Render(fmt.Sprintf("%.0f%%", frac*100))
}

// ---- approval ----

var approvalChoices = []string{"Yes", "Yes, and don't ask again for %s this session", "No, and tell caveira what to do instead"}

// renderApproval is the card that replaces the input while the agent waits
// for a yes or no on a command or a file change.
func (m *Model) renderApproval() string {
	a := m.approval
	if a == nil {
		return ""
	}
	width := m.inner()
	label := toolLabel(a.Name)
	what := "wants to run a command"
	if a.Kind.String() == "write" {
		what = "wants to change a file"
	}
	head := th.Title.Render(label) + th.Text.Render(" "+what)

	preview := strings.TrimSpace(a.Preview)
	if preview == "" {
		preview = "(see the transcript above)"
	}
	if a.Name == "bash" {
		preview = "$ " + preview
	}
	room := width - 8
	pv := lipgloss.Wrap(preview, room, "")
	pvLines := strings.Split(pv, "\n")
	if len(pvLines) > 8 {
		pvLines = append(pvLines[:7], "…")
	}
	for i, l := range pvLines {
		pvLines[i] = "  " + th.Code.Render(l)
	}

	var choices []string
	for i, c := range approvalChoices {
		if strings.Contains(c, "%s") {
			c = fmt.Sprintf(c, label)
		}
		num := fmt.Sprintf("%d. ", i+1)
		if i == m.apSel {
			choices = append(choices, th.Accent.Render("› ")+th.Title.Render(num+c))
		} else {
			choices = append(choices, "  "+th.Muted.Render(num+c))
		}
	}
	body := head + "\n\n" + strings.Join(pvLines, "\n") + "\n\n" + strings.Join(choices, "\n")
	return titledBox("Permission", body, width, th.warn)
}

// titledBox is a rounded box with a title set into its top edge.
func titledBox(title, body string, width int, border color.Color) string {
	b := lipgloss.RoundedBorder()
	st := lipgloss.NewStyle().Foreground(border)
	inner := width - 2
	lines := strings.Split(body, "\n")
	t := " " + title + " "
	top := st.Render(b.TopLeft+b.Top) + lipgloss.NewStyle().Foreground(border).Bold(true).Render(t) +
		st.Render(strings.Repeat(b.Top, max(inner-1-lipgloss.Width(t), 0))+b.TopRight)
	out := []string{top}
	pad := lipgloss.NewStyle().Width(inner - 2)
	for _, l := range lines {
		out = append(out, st.Render(b.Left)+" "+pad.Render(l)+" "+st.Render(b.Right))
	}
	out = append(out, st.Render(b.BottomLeft+strings.Repeat(b.Bottom, inner)+b.BottomRight))
	return strings.Join(out, "\n")
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
