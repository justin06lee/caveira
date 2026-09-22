package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/tools"
)

type itemKind int

const (
	itemUser itemKind = iota
	itemAssistant
	itemReasoning
	itemTool
	itemError
	itemNotice
)

// item is one block in the transcript. Rendering is cached by width; the
// item that is still streaming invalidates its own cache on every delta.
type item struct {
	kind itemKind
	text string

	// tool fields
	toolID   string
	toolName string
	preview  string
	toolKind tools.Kind
	result   *tools.Result
	started  time.Time
	elapsed  time.Duration
	running  bool
	denied   bool

	cache      string
	cacheWidth int
	cacheKey   string
}

func (it *item) invalidate() { it.cacheWidth = 0 }

// renderer draws items. It carries the state that changes how an item looks
// without the item itself changing: width, expansion toggles, spinner frame.
type renderer struct {
	width         int
	md            *markdown
	showReasoning bool
	expandTools   bool
	spinner       string
}

func (r *renderer) render(it *item) string {
	key := fmt.Sprintf("%v/%v/%s", r.showReasoning, r.expandTools, r.spinnerKey(it))
	if it.cacheWidth == r.width && it.cacheKey == key && it.cache != "" {
		return it.cache
	}
	var out string
	switch it.kind {
	case itemUser:
		out = r.renderUser(it)
	case itemAssistant:
		out = r.renderAssistant(it)
	case itemReasoning:
		out = r.renderReasoning(it)
	case itemTool:
		out = r.renderTool(it)
	case itemError:
		out = r.wrap(styleEmber.Render("✗ ")+styleEmber.Render(it.text), 0)
	case itemNotice:
		out = r.wrap(styleDim.Render(it.text), 0)
	}
	it.cache = out
	it.cacheWidth = r.width
	it.cacheKey = key
	return out
}

// spinnerKey makes running tools re-render every tick and nothing else.
func (r *renderer) spinnerKey(it *item) string {
	if it.kind == itemTool && it.running {
		return r.spinner
	}
	return ""
}

func (r *renderer) wrap(s string, indent int) string {
	w := r.width - indent
	if w < 10 {
		w = 10
	}
	wrapped := lipgloss.Wrap(s, w, "")
	if indent == 0 {
		return wrapped
	}
	pad := strings.Repeat(" ", indent)
	return pad + strings.ReplaceAll(wrapped, "\n", "\n"+pad)
}

func (r *renderer) renderUser(it *item) string {
	text := strings.TrimRight(it.text, "\n")
	body := r.wrap(styleUserText.Render(text), 2)
	// The mark sits on the first line only.
	return styleUserMark.Render("❯ ") + strings.TrimPrefix(body, "  ")
}

func (r *renderer) renderAssistant(it *item) string {
	text := strings.TrimSpace(it.text)
	if text == "" {
		return ""
	}
	if r.md == nil {
		return r.wrap(styleBody.Render(text), 2)
	}
	rendered := r.md.render(text)
	return "  " + strings.ReplaceAll(rendered, "\n", "\n  ")
}

func (r *renderer) renderReasoning(it *item) string {
	text := strings.TrimSpace(it.text)
	if text == "" {
		return ""
	}
	lines := strings.Count(text, "\n") + 1
	if !r.showReasoning {
		words := len(strings.Fields(text))
		return "  " + styleThinking.Render(fmt.Sprintf("∴ thought for %d words  ·  ctrl+t to show", words)) + strings.Repeat("", lines)
	}
	return "  " + styleThinking.Render("∴ thinking") + "\n" + r.wrap(styleThinking.Render(text), 4)
}

func (r *renderer) renderTool(it *item) string {
	mark := styleSlate.Render("⚙")
	if it.running {
		mark = styleBrass.Render(r.spinner)
	} else if it.result != nil && it.result.IsError {
		mark = styleEmber.Render("⚙")
	}
	head := mark + " " + styleToolName.Render(it.toolName)
	if it.preview != "" {
		head += "  " + styleToolArg.Render(oneLine(it.preview, r.width-len(it.toolName)-24))
	}
	if it.result != nil {
		summary := it.result.Summary
		if summary != "" {
			s := styleDim.Render("  ·  " + summary)
			if it.result.IsError {
				s = styleEmber.Render("  ·  " + summary)
			}
			head += s
		}
	}
	parts := []string{head}

	if it.result == nil {
		return strings.Join(parts, "\n")
	}
	if it.result.Diff != "" {
		parts = append(parts, r.renderDiff(it.result.Diff))
	} else if body := r.toolBody(it); body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

// toolBody shows a slice of the tool output: enough to follow along, never
// enough to drown the conversation. ctrl+o expands it.
func (r *renderer) toolBody(it *item) string {
	res := it.result
	switch it.toolName {
	case "read_file", "glob", "grep", "list_dir":
		if !r.expandTools && !res.IsError {
			return ""
		}
	}
	text := strings.TrimRight(res.Output, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	limit := 6
	if r.expandTools {
		limit = 200
	}
	if res.IsError && limit < 12 {
		limit = 12
	}
	shown := lines
	more := 0
	if len(lines) > limit {
		shown = lines[:limit]
		more = len(lines) - limit
	}
	var sb strings.Builder
	for _, l := range shown {
		sb.WriteString("    ")
		sb.WriteString(styleToolOut.Render(oneLine(l, r.width-6)))
		sb.WriteByte('\n')
	}
	if more > 0 {
		hint := "ctrl+o to expand"
		if r.expandTools {
			hint = "truncated"
		}
		sb.WriteString("    " + styleSlate.Render(fmt.Sprintf("… %d more lines  ·  %s", more, hint)))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (r *renderer) renderDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	limit := 40
	if r.expandTools {
		limit = 400
	}
	var sb strings.Builder
	for i, l := range lines {
		if i >= limit {
			sb.WriteString("    " + styleSlate.Render(fmt.Sprintf("… %d more lines  ·  ctrl+o to expand", len(lines)-i)))
			break
		}
		var st lipgloss.Style
		switch {
		case strings.HasPrefix(l, "@@"):
			st = styleDiffHdr
		case strings.HasPrefix(l, "+"):
			st = styleDiffAdd
		case strings.HasPrefix(l, "-"):
			st = styleDiffDel
		default:
			st = styleToolOut
		}
		sb.WriteString("    " + st.Render(oneLine(l, r.width-6)))
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func oneLine(s string, width int) string {
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	s = strings.ReplaceAll(s, "\t", "  ")
	if width < 8 {
		width = 8
	}
	if lipgloss.Width(s) > width {
		// Cut by runes, roughly; exact cell math is not worth it here.
		rs := []rune(s)
		if len(rs) > width-1 {
			rs = rs[:width-1]
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
		return "$0"
	case usd < 0.01:
		return fmt.Sprintf("$%.4f", usd)
	case usd < 1:
		return fmt.Sprintf("$%.3f", usd)
	default:
		return fmt.Sprintf("$%.2f", usd)
	}
}
