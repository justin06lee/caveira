package ui

import (
	"image/color"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
)

// Fenced code blocks are drawn by caveira rather than glamour, as panels of
// their own: a header strip with the language on the left and a copy
// button on the right, then the code, highlighted, on a shaded body. The
// edges are horizontal half cells with the corners cut back, like the chat
// bubbles. Terminals without shading get a rounded outline instead.

// mdSegment is a run of a reply: prose for glamour, or one fenced block.
type mdSegment struct {
	code   bool
	text   string
	lang   string
	indent int // how far the fence was indented, as in a list item
}

// codeSpot is where a code block's copy button was drawn, in cells from
// the top left of what was rendered, and the code it copies.
type codeSpot struct {
	row, x0, x1 int
	code        string
}

var fenceOpen = regexp.MustCompile("^([ \t]*)(`{3,}|~{3,})[ \t]*([^ \t`]*)(.*)$")

// splitFences cuts markdown into prose and fenced code blocks. A block
// still open at the end (the reply is streaming) runs to the end, so the
// panel grows as the code arrives. Code keeps its tabs, so a copied
// Makefile or Go file is the same bytes the model wrote.
func splitFences(src string) []mdSegment {
	lines := strings.Split(src, "\n")
	var segs []mdSegment
	var prose []string
	flush := func() {
		if p := strings.Join(prose, "\n"); strings.TrimSpace(p) != "" {
			segs = append(segs, mdSegment{text: p})
		}
		prose = nil
	}
	for i := 0; i < len(lines); i++ {
		m := fenceOpen.FindStringSubmatch(lines[i])
		if m == nil || m[2][0] == '`' && strings.Contains(m[4], "`") {
			// Backticks after a backtick fence make it inline code.
			prose = append(prose, lines[i])
			continue
		}
		flush()
		fence := m[2]
		seg := mdSegment{code: true, lang: m[3], indent: len(expandTabs(m[1]))}
		var body []string
		j := i + 1
		for ; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if len(t) >= len(fence) && strings.Trim(t, fence[:1]) == "" {
				break
			}
			body = append(body, trimIndent(lines[j], seg.indent))
		}
		seg.text = strings.TrimRight(strings.Join(body, "\n"), "\n \t")
		segs = append(segs, seg)
		i = j
	}
	flush()
	return segs
}

// trimIndent removes up to n columns of leading spaces and tabs.
func trimIndent(s string, n int) string {
	i, col := 0, 0
	for i < len(s) && col < n && (s[i] == ' ' || s[i] == '\t') {
		if s[i] == '\t' {
			col += 4
		} else {
			col++
		}
		i++
	}
	return s[i:]
}

// expandTabs swaps tabs for four spaces: a tab measures as nothing and
// draws as several cells, so nothing is measured with one in it.
func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

const (
	copyLabel   = "copy"
	copiedLabel = "copied ✓"
)

// codeBlock draws one fenced block the given width across. copied swaps
// the button for a moment's confirmation after a click.
func codeBlock(seg mdSegment, width int, copied bool) (string, codeSpot) {
	ind := min(seg.indent, 4)
	w := max(width-ind, 16)
	lead := strings.Repeat(" ", ind)
	textW := w - 4 // two cells of padding either side

	label := strings.ToLower(seg.lang)
	if label == "" {
		label = "code"
	}
	button, btnFg := copyLabel, th.muted
	if copied {
		button, btnFg = copiedLabel, th.ok
	}
	bw := lipgloss.Width(button)
	if lipgloss.Width(label)+bw+2 > textW {
		label = ""
	}
	spot := codeSpot{code: seg.text}

	if th.surface == nil {
		lines := highlight(expandTabs(seg.text), seg.lang, textW, nil)
		line := lipgloss.NewStyle().Foreground(th.line)
		var out []string
		fill := max(w-8-lipgloss.Width(label)-bw, 0)
		top := line.Render("╭─ ") + th.Faint.Render(label) + line.Render(" "+strings.Repeat("─", fill)+" ") +
			lipgloss.NewStyle().Foreground(btnFg).Render(button) + line.Render(" ─╮")
		out = append(out, lead+top)
		spot.row, spot.x0 = 0, ind+3+lipgloss.Width(label)+1+fill+1
		spot.x1 = spot.x0 + bw
		for _, l := range lines {
			out = append(out, lead+line.Render("│ ")+l+line.Render(" │"))
		}
		out = append(out, lead+line.Render("╰"+strings.Repeat("─", w-2)+"╯"))
		return strings.Join(out, "\n"), spot
	}

	head, body := th.codeHead, th.surface
	headBg := lipgloss.NewStyle().Background(head)
	bodyBg := lipgloss.NewStyle().Background(body)
	gap := textW - lipgloss.Width(label) - bw

	out := []string{
		lead + " " + lipgloss.NewStyle().Foreground(head).Render(strings.Repeat("▄", w-2)),
		lead + headBg.Render("  ") +
			lipgloss.NewStyle().Foreground(th.faint).Background(head).Render(label) +
			headBg.Render(strings.Repeat(" ", gap)) +
			lipgloss.NewStyle().Foreground(btnFg).Background(head).Render(button) +
			headBg.Render("  "),
		// Half a row of the header over half a row of the body.
		lead + lipgloss.NewStyle().Foreground(head).Background(body).Render(strings.Repeat("▀", w)),
	}
	spot.row, spot.x0 = 1, ind+2+lipgloss.Width(label)+gap
	spot.x1 = spot.x0 + bw
	for _, l := range highlight(expandTabs(seg.text), seg.lang, textW, body) {
		out = append(out, lead+bodyBg.Render("  ")+l+bodyBg.Render("  "))
	}
	out = append(out, lead+" "+lipgloss.NewStyle().Foreground(body).Render(strings.Repeat("▀", w-2)))
	return strings.Join(out, "\n"), spot
}

// highlight colours code and cuts it into rows exactly width cells wide,
// long lines wrapped rather than cut off, each cell on bg (none if nil).
// The wrapping is only on screen: the copy button copies the code as it
// was written.
func highlight(code, lang string, width int, bg color.Color) []string {
	type span struct {
		text string
		tt   chroma.TokenType
	}
	rows := [][]span{nil}
	add := func(text string, tt chroma.TokenType) {
		for i, p := range strings.Split(text, "\n") {
			if i > 0 {
				rows = append(rows, nil)
			}
			if p != "" {
				rows[len(rows)-1] = append(rows[len(rows)-1], span{p, tt})
			}
		}
	}

	lexer := lexers.Get(strings.ToLower(lang))
	if lexer == nil && lang == "" {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	if it, err := chroma.Coalesce(lexer).Tokenise(nil, code); err == nil {
		for tok := it(); tok != chroma.EOF; tok = it() {
			add(tok.Value, tok.Type)
		}
	} else {
		add(code, chroma.Text)
	}
	// Lexers end the text with a newline the code may not have had.
	if n := strings.Count(code, "\n") + 1; len(rows) > n {
		rows = rows[:n]
	}

	syn := syntaxOf(th)
	styles := map[chroma.TokenType]lipgloss.Style{}
	style := func(tt chroma.TokenType) lipgloss.Style {
		if st, ok := styles[tt]; ok {
			return st
		}
		c, bold, italic := syn.of(tt)
		st := lipgloss.NewStyle().Foreground(c).Bold(bold).Italic(italic)
		if bg != nil {
			st = st.Background(bg)
		}
		styles[tt] = st
		return st
	}
	pad := lipgloss.NewStyle()
	if bg != nil {
		pad = pad.Background(bg)
	}

	var out []string
	for _, row := range rows {
		var runes []rune
		var tts []chroma.TokenType
		for _, sp := range row {
			for _, r := range sp.text {
				runes = append(runes, r)
				tts = append(tts, sp.tt)
			}
		}
		// Rows a long line wraps onto sit under its first character.
		lead := 0
		for lead < len(runes) && runes[lead] == ' ' {
			lead++
		}
		lead = min(lead, width/2)
		for k, seg := range wrapCode(runes, width, lead) {
			var sb strings.Builder
			w := 0
			if k > 0 {
				sb.WriteString(pad.Render(strings.Repeat(" ", lead)))
				w = lead
			}
			for i := seg[0]; i < seg[1]; {
				j := i
				for j < seg[1] && tts[j] == tts[i] {
					j++
				}
				text := string(runes[i:j])
				sb.WriteString(style(tts[i]).Render(text))
				w += ansi.StringWidth(text)
				i = j
			}
			out = append(out, sb.String()+pad.Render(strings.Repeat(" ", max(width-w, 0))))
		}
	}
	return out
}

// wrapCode cuts a line of code into rows that fit width, the rows after
// the first lead cells narrower. It breaks after a space where there is
// one not too far back, and mid-word where there is not.
func wrapCode(runes []rune, width, lead int) [][2]int {
	var rows [][2]int
	start := 0
	for start < len(runes) {
		room := width
		if len(rows) > 0 {
			room = width - lead
		}
		w, end, space := 0, start, -1
		for end < len(runes) {
			rw := ansi.StringWidth(string(runes[end]))
			if w+rw > room && end > start {
				break
			}
			if runes[end] == ' ' {
				space = end
			}
			w += rw
			end++
		}
		if end < len(runes) && space > start && space-start >= room/3 {
			end = space + 1
		}
		rows = append(rows, [2]int{start, end})
		start = end
	}
	if len(rows) == 0 {
		rows = [][2]int{{0, 0}}
	}
	return rows
}

// syntax is the code palette: a few colours cut to sit with the theme.
type syntax struct {
	text, comment, keyword, typ, op, punct, fn, builtin, num, str, esc color.Color
}

func syntaxOf(t *theme) syntax {
	if !t.dark {
		return syntax{hex("#2F302A"), hex("#8A8C83"), hex("#B8472F"), hex("#4F5243"), hex("#55574F"), hex("#6B6D64"), hex("#2F6391"), hex("#8A5A12"), hex("#7A4B96"), hex("#4A7D2C"), hex("#8A5A12")}
	}
	return syntax{hex("#D6D4C8"), hex("#6E7068"), hex("#E5836B"), hex("#C9CCB9"), hex("#A9ABA1"), hex("#8E9087"), hex("#9FC2E0"), hex("#D9B574"), hex("#C5A3D9"), hex("#A8C98C"), hex("#D9B574")}
}

// of is the colour for a token type, falling back through its subcategory
// and category.
func (s syntax) of(tt chroma.TokenType) (c color.Color, bold, italic bool) {
	switch tt {
	case chroma.CommentPreproc, chroma.CommentPreprocFile, chroma.NameTag:
		return s.keyword, false, false
	case chroma.KeywordType, chroma.NameException:
		return s.typ, false, false
	case chroma.NameFunction, chroma.NameFunctionMagic, chroma.NameAttribute:
		return s.fn, false, false
	case chroma.NameBuiltin, chroma.NameBuiltinPseudo, chroma.NameDecorator:
		return s.builtin, false, false
	case chroma.NameClass:
		return s.text, true, false
	case chroma.NameConstant, chroma.LiteralDate:
		return s.num, false, false
	case chroma.LiteralStringEscape:
		return s.esc, false, false
	case chroma.GenericDeleted:
		return th.delFg, false, false
	case chroma.GenericInserted:
		return th.addFg, false, false
	case chroma.GenericHeading, chroma.GenericSubheading:
		return s.comment, false, false
	}
	switch tt.SubCategory() {
	case chroma.LiteralNumber:
		return s.num, false, false
	case chroma.LiteralString:
		return s.str, false, false
	}
	switch tt.Category() {
	case chroma.Comment:
		return s.comment, false, true
	case chroma.Keyword:
		return s.keyword, false, false
	case chroma.Operator:
		return s.op, false, false
	case chroma.Punctuation:
		return s.punct, false, false
	case chroma.Literal:
		return s.str, false, false
	}
	return s.text, false, false
}
