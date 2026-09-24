package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/justin06lee/caveira/tui/internal/art"
	"github.com/justin06lee/caveira/tui/internal/tools"
)

type itemKind int

const (
	itemHeader    itemKind = iota // the session card: mascot, model, directory
	itemUser                      // a message you sent
	itemCommand                   // a slash command you ran, echoed
	itemAssistant                 // the model's reply
	itemReasoning                 // the model's thinking
	itemTool                      // one tool call and its result
	itemError                     // something failed
	itemNotice                    // a line or two of command output
	itemPanel                     // titled rows of command output
	itemDivider                   // a labelled rule: interrupted, compacted
)

// item is one block in the transcript. Rendering is cached by width and
// toggles; the item that is still streaming invalidates its own cache on
// every delta.
type item struct {
	kind itemKind
	text string

	// panel fields
	title string
	rows  [][2]string

	// divider tone: warn for interruptions, faint otherwise; quiet ones
	// close a turn with a short rule
	warn, quiet bool

	// header fields
	header *headerInfo

	// tool fields; started and elapsed also time reasoning
	toolID   string
	toolName string
	preview  string
	toolKind tools.Kind
	result   *tools.Result
	started  time.Time
	elapsed  time.Duration
	running  bool
	user     bool // a !command you ran yourself

	cache      string
	cacheWidth int
	cacheKey   string
}

func (it *item) invalidate() { it.cacheWidth = 0 }

type headerInfo struct {
	version, model, effort, dir, branch string
	confirm, dev                        bool
}

// renderer draws items. It carries the state that changes how an item looks
// without the item itself changing: width, expansion toggles, the
// animation frame.
type renderer struct {
	width         int
	md            *markdown
	showReasoning bool
	expandTools   bool
	frame         int
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (r *renderer) spinner() string { return spinnerFrames[r.frame%len(spinnerFrames)] }

func (r *renderer) render(it *item) string {
	key := fmt.Sprintf("%v/%v/%s", r.showReasoning, r.expandTools, r.animKey(it))
	if it.cacheWidth == r.width && it.cacheKey == key && it.cache != "" {
		return it.cache
	}
	var out string
	switch it.kind {
	case itemHeader:
		out = r.renderHeader(it)
	case itemUser:
		out = r.renderUser(it)
	case itemCommand:
		out = th.Faint.Render("› ") + th.Muted.Render(oneLine(it.text, r.width-2))
	case itemAssistant:
		out = r.renderAssistant(it)
	case itemReasoning:
		out = r.renderReasoning(it)
	case itemTool:
		out = r.renderTool(it)
	case itemError:
		out = r.hang(th.Err.Render("✗ "), th.Err, it.text)
	case itemNotice:
		out = r.hang(th.Faint.Render("└ "), th.Muted, it.text)
	case itemPanel:
		out = r.renderPanel(it)
	case itemDivider:
		out = r.renderDivider(it)
	}
	it.cache = clampWidth(out, r.width)
	it.cacheWidth = r.width
	it.cacheKey = key
	return it.cache
}

// clampWidth cuts every line to the width. glamour pads lines out to its
// wrap width, and one line too wide would widen the whole viewport.
func clampWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > width {
			lines[i] = ansi.Truncate(l, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// animKey makes running items re-render every frame and nothing else.
func (r *renderer) animKey(it *item) string {
	if (it.kind == itemTool || it.kind == itemReasoning) && it.running {
		return strconv.Itoa(r.frame)
	}
	return ""
}

// hang wraps text in a style with a mark on the first line and the rest
// indented under it.
func (r *renderer) hang(mark string, st lipgloss.Style, text string) string {
	mw := lipgloss.Width(mark)
	text = strings.ReplaceAll(strings.TrimRight(text, "\n"), "\t", "    ")
	body := lipgloss.Wrap(text, max(r.width-mw, 10), "")
	lines := strings.Split(body, "\n")
	pad := strings.Repeat(" ", mw)
	for i, l := range lines {
		lead := pad
		if i == 0 {
			lead = mark
		}
		lines[i] = lead + st.Render(l)
	}
	return strings.Join(lines, "\n")
}

// indent prefixes every line.
func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// ---- header ----

// renderHeader is the card the transcript opens with: the small mascot
// beside what this session is running with.
func (r *renderer) renderHeader(it *item) string {
	h := it.header
	title := th.Bone.Bold(true).Render(homeTitle)
	if h.version != "" && h.version != "dev" {
		title += th.Faint.Render("  " + h.version)
	}
	model := th.Text.Render(h.model)
	if h.effort != "" {
		model += th.Muted.Render(" · " + h.effort + " effort")
	}
	if h.dev {
		model += th.Muted.Render(" · ") + devBadge() + th.Muted.Render(" local model")
	}
	dir := th.Text.Render(shortPath(h.dir))
	if h.branch != "" {
		dir += th.Muted.Render("  on ") + th.Text.Render(h.branch)
	}
	tip := th.Faint.Render("/help for commands · ! runs a shell command")
	if h.confirm {
		tip = th.Faint.Render("asks before commands and edits · /help for more")
	}
	label := func(s string) string { return th.Faint.Render(fmt.Sprintf("%-7s", s)) }
	right := []string{
		title,
		th.Muted.Render("coding agent for abliterated models"),
		"",
		label("model") + model,
		label("cwd") + dir,
		tip,
	}

	rightW := 0
	for _, l := range right {
		rightW = max(rightW, lipgloss.Width(l))
	}
	var body []string
	mascotW := art.MascotWidth(1)
	withMascot := r.width >= mascotW+rightW+10
	if withMascot {
		skull := art.Mascot(1, art.MascotOptions{Style: th.mascot})
		for i := range max(len(skull), len(right)) {
			l, rr := strings.Repeat(" ", mascotW), ""
			if i < len(skull) {
				l = skull[i]
			}
			if i < len(right) {
				rr = right[i]
			}
			body = append(body, l+"   "+rr)
		}
	} else {
		body = right
	}
	inner := 0
	for _, l := range body {
		inner = max(inner, lipgloss.Width(l))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.line).
		Padding(1, 2)
	w := min(inner+box.GetHorizontalFrameSize(), r.width)
	return box.Width(w).Render(strings.Join(body, "\n"))
}

// ---- user ----

// renderUser draws your message as a chat bubble on the right, so your
// side of the conversation is easy to find when scrolling back.
func (r *renderer) renderUser(it *item) string {
	text := strings.ReplaceAll(strings.TrimRight(it.text, "\n"), "\t", "    ")
	return bubble(text, r.width, th.bubble, th.text, th.muted)
}

// ---- assistant ----

func (r *renderer) renderAssistant(it *item) string {
	text := strings.TrimSpace(it.text)
	if text == "" {
		return ""
	}
	var body string
	if r.md == nil {
		body = lipgloss.Wrap(th.Text.Render(text), max(r.width-2, 10), "")
	} else {
		body = r.md.render(text)
	}
	return th.Bone.Render("●") + " " + strings.ReplaceAll(body, "\n", "\n  ")
}

// ---- reasoning ----

func (r *renderer) renderReasoning(it *item) string {
	text := strings.TrimSpace(it.text)
	if text == "" && !it.running {
		return ""
	}
	var head string
	switch {
	case it.running:
		head = th.Thinking.Render("∴ Thinking…")
	case it.elapsed > 0:
		head = th.Thinking.Render("∴ Thought for " + formatDuration(it.elapsed))
	default:
		head = th.Thinking.Render(fmt.Sprintf("∴ Thought · %d words", len(strings.Fields(text))))
	}
	if !r.showReasoning {
		if it.running {
			// The status line already says so; the block appears when
			// there is a duration to report.
			return ""
		}
		return head + th.Faint.Render("  ctrl+t")
	}
	if text == "" {
		return head
	}
	body := lipgloss.Wrap(strings.ReplaceAll(text, "\t", "    "), max(r.width-4, 10), "")
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = "  " + th.Line.Render("│ ") + th.Dim.Render(l)
	}
	return head + "\n" + strings.Join(lines, "\n")
}

// ---- tools ----

// toolLabels are the names people read; the model sees the real ones.
var toolLabels = map[string]string{
	"read_file":  "Read",
	"write_file": "Write",
	"edit_file":  "Edit",
	"bash":       "Bash",
	"glob":       "Find",
	"grep":       "Search",
	"list_dir":   "List",
}

func toolLabel(name string) string {
	if l, ok := toolLabels[name]; ok {
		return l
	}
	return name
}

func (r *renderer) renderTool(it *item) string {
	var mark string
	switch {
	case it.running:
		mark = th.Accent.Render(r.spinner())
	case it.result != nil && it.result.IsError:
		mark = th.Err.Render("●")
	default:
		mark = th.OK.Render("●")
	}
	label := toolLabel(it.toolName)
	if it.user {
		label = "Shell"
	}
	head := mark + " " + th.Title.Render(label)
	arg := it.preview
	if it.toolName == "bash" {
		arg = "$ " + arg
	}
	var stat string
	if it.result != nil && it.result.Diff != "" {
		add, del := diffStat(it.result.Diff)
		stat = "  " + th.OK.Render(fmt.Sprintf("+%d", add)) + " " + th.Err.Render(fmt.Sprintf("−%d", del))
	}
	if arg != "" {
		room := r.width - lipgloss.Width(head) - lipgloss.Width(stat) - 1
		head += " " + th.Muted.Render(oneLine(arg, room))
	}
	head += stat
	if it.result == nil {
		if it.running && !it.started.IsZero() {
			if d := time.Since(it.started); d >= 2*time.Second {
				head += "\n" + th.Faint.Render("  └ running · "+formatDuration(d))
			}
		}
		return head
	}

	res := it.result
	summary := res.Summary
	st := th.Muted
	if res.IsError {
		st = th.Err
	}
	parts := []string{head}
	if summary != "" {
		parts = append(parts, th.Faint.Render("  └ ")+st.Render(oneLine(summary, r.width-4)))
	}
	if res.Diff != "" {
		parts = append(parts, r.renderDiff(res.Diff))
	} else if body := r.toolBody(it); body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

// modelOnly are the lines bash appends for the model's benefit; the result
// line already says the same to a person.
var modelOnly = regexp.MustCompile(`^\[(no output, exit code \d+|exit code -?\d+|output was truncated;.*)\]$`)

// toolBody shows a slice of the tool output: enough to follow along, never
// enough to drown the conversation. ctrl+o expands it.
func (r *renderer) toolBody(it *item) string {
	res := it.result
	switch it.toolName {
	case "read_file", "glob", "grep", "list_dir", "write_file":
		if !r.expandTools && !res.IsError {
			return ""
		}
	}
	lines := strings.Split(strings.TrimRight(res.Output, "\n"), "\n")
	for len(lines) > 0 && (modelOnly.MatchString(strings.TrimSpace(lines[len(lines)-1])) || strings.TrimSpace(lines[len(lines)-1]) == "") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || res.IsError && len(lines) == 1 && strings.TrimPrefix(lines[0], "Error: ") == res.Summary {
		return ""
	}
	limit := 5
	if r.expandTools {
		limit = 400
	} else if res.IsError {
		limit = 10
	}
	shown, more := lines, 0
	if len(lines) > limit {
		shown, more = lines[:limit], len(lines)-limit
	}
	var sb strings.Builder
	for i, l := range shown {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("    " + th.Muted.Render(oneLine(l, r.width-4)))
	}
	if more > 0 {
		hint := "ctrl+o to expand"
		if r.expandTools {
			hint = "truncated"
		}
		sb.WriteString("\n    " + th.Faint.Render(fmt.Sprintf("… %d more lines · %s", more, hint)))
	}
	return sb.String()
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func diffStat(diff string) (add, del int) {
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
		case strings.HasPrefix(l, "+"):
			add++
		case strings.HasPrefix(l, "-"):
			del++
		}
	}
	return
}

// renderDiff draws the diff with line numbers, added and removed lines on
// tinted rows the full width of the transcript.
func (r *renderer) renderDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	// The gutter fits the largest line number in the diff.
	maxLine := 0
	{
		oldN, newN := 0, 0
		for _, l := range lines {
			if m := hunkHeader.FindStringSubmatch(l); m != nil {
				oldN, _ = strconv.Atoi(m[1])
				newN, _ = strconv.Atoi(m[2])
				continue
			}
			maxLine = max(maxLine, oldN, newN)
			switch {
			case strings.HasPrefix(l, "-"):
				oldN++
			case strings.HasPrefix(l, "+"):
				newN++
			default:
				oldN++
				newN++
			}
		}
	}
	gw := len(strconv.Itoa(maxLine))
	rowW := r.width - 4
	limit := 30
	if r.expandTools {
		limit = 600
	}

	add := lipgloss.NewStyle().Foreground(th.addFg)
	del := lipgloss.NewStyle().Foreground(th.delFg)
	if th.addBg != nil {
		add = add.Background(th.addBg)
		del = del.Background(th.delBg)
	}
	add, del = add.Width(rowW), del.Width(rowW)
	ctx := th.Muted

	var out []string
	oldN, newN, shown := 0, 0, 0
	for i, l := range lines {
		if m := hunkHeader.FindStringSubmatch(l); m != nil {
			oldN, _ = strconv.Atoi(m[1])
			newN, _ = strconv.Atoi(m[2])
			if i > 0 {
				out = append(out, "    "+th.Faint.Render(strings.Repeat(" ", gw)+" ⋮"))
			}
			continue
		}
		if shown >= limit {
			rest := len(lines) - i
			out = append(out, "    "+th.Faint.Render(fmt.Sprintf("… %d more lines · ctrl+o to expand", rest)))
			break
		}
		shown++
		sign, text := " ", l
		if l != "" {
			sign, text = l[:1], l[1:]
		}
		text = strings.ReplaceAll(text, "\t", "    ")
		n := newN
		switch sign {
		case "-":
			n = oldN
			oldN++
		case "+":
			newN++
		default:
			oldN++
			newN++
		}
		num := fmt.Sprintf("%*d", gw, n)
		room := rowW - gw - 3
		switch sign {
		case "+":
			out = append(out, "    "+add.Render(num+" + "+oneLine(text, room)))
		case "-":
			out = append(out, "    "+del.Render(num+" - "+oneLine(text, room)))
		default:
			out = append(out, "    "+th.Faint.Render(num)+"   "+ctx.Render(oneLine(text, room)))
		}
	}
	return strings.Join(out, "\n")
}

// ---- panels and dividers ----

func (r *renderer) renderPanel(it *item) string {
	keyW := 0
	for _, row := range it.rows {
		keyW = max(keyW, lipgloss.Width(row[0]))
	}
	var out []string
	if it.title != "" {
		out = append(out, th.Faint.Render("└ ")+th.Title.Render(it.title))
	}
	for _, row := range it.rows {
		if row[0] == "" && row[1] == "" {
			out = append(out, "")
			continue
		}
		if row[1] == "" && row[0] != "" && strings.HasPrefix(row[0], "#") {
			// A subheading inside the panel.
			out = append(out, "", "  "+th.Title.Render(strings.TrimPrefix(row[0], "#")))
			continue
		}
		key := fmt.Sprintf("%-*s", keyW, row[0])
		val := lipgloss.Wrap(row[1], max(r.width-keyW-8, 10), "")
		vl := strings.Split(val, "\n")
		for i, v := range vl {
			k := strings.Repeat(" ", keyW)
			if i == 0 {
				k = key
			}
			out = append(out, "    "+th.Bone.Render(k)+"   "+th.Muted.Render(v))
		}
	}
	return strings.Join(out, "\n")
}

func (r *renderer) renderDivider(it *item) string {
	st := th.Faint
	if it.warn {
		st = th.Warn
	}
	if it.quiet {
		// The end of a turn: a short rule, not a full-width one.
		return th.Line.Render("──") + st.Render(" "+it.text)
	}
	label := " " + it.text + " "
	left := 2
	right := max(r.width-left-lipgloss.Width(label), 0)
	return th.Line.Render(strings.Repeat("─", left)) + st.Render(label) + th.Line.Render(strings.Repeat("─", right))
}

// ---- helpers ----

func oneLine(s string, width int) string {
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	s = strings.ReplaceAll(s, "\t", "  ")
	if width < 8 {
		width = 8
	}
	if lipgloss.Width(s) > width {
		rs := []rune(s)
		for len(rs) > 0 && lipgloss.Width(string(rs)) > width-1 {
			cut := max(lipgloss.Width(string(rs))-(width-1), 1)
			rs = rs[:max(len(rs)-cut, 0)]
		}
		s = string(rs) + "…"
	}
	return s
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatCost(usd float64) string {
	switch {
	case usd == 0:
		return "$0.00"
	case usd < 0.01:
		return fmt.Sprintf("$%.4f", usd)
	case usd < 1:
		return fmt.Sprintf("$%.3f", usd)
	default:
		return fmt.Sprintf("$%.2f", usd)
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}
