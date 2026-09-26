package ui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// markdown renders assistant text at a fixed width. It is rebuilt when the
// terminal is resized or the theme changes; rendering is cached per
// transcript item.
type markdown struct {
	width int
	tr    *glamour.TermRenderer
}

func newMarkdown(width int) *markdown {
	if width < 20 {
		width = 20
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(transcriptStyle(th)),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return &markdown{width: width}
	}
	return &markdown{width: width, tr: tr}
}

// render draws a reply: prose through glamour, fenced code blocks as
// panels of their own (codeblock.go). spots are where each block's copy
// button landed; copied is the block whose button says so, counting from
// one, or zero for none.
func (m *markdown) render(src string, copied int) (out string, spots []codeSpot) {
	var parts []string
	row := 0
	for _, seg := range splitFences(src) {
		var s string
		if seg.code {
			var spot codeSpot
			s, spot = codeBlock(seg, m.width, len(spots)+1 == copied)
			spot.row += row
			spots = append(spots, spot)
		} else if s = m.prose(seg.text); s == "" {
			continue
		}
		parts = append(parts, s)
		row += strings.Count(s, "\n") + 2
	}
	return strings.Join(parts, "\n\n"), spots
}

// prose renders markdown with no fenced blocks in it.
func (m *markdown) prose(src string) string {
	if m == nil || m.tr == nil {
		return lipgloss.Wrap(strings.TrimSpace(expandTabs(src)), max(m.width, 10), "")
	}
	out, err := m.tr.Render(expandTabs(src))
	if err != nil {
		return src
	}
	// glamour pads the document with blank lines top and bottom (some of
	// them spaces in colour codes), and pads every line to the wrap width;
	// the transcript spaces items itself.
	lines := strings.Split(out, "\n")
	blank := func(l string) bool { return strings.TrimSpace(xansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// transcriptStyle is glamour's base style recoloured to the theme: headings
// ranked by colour (a terminal has one type size), bone emphasis, inline
// code in brass, and the code palette for the indented code blocks glamour
// still draws.
func transcriptStyle(t *theme) ansi.StyleConfig {
	s := styles.DarkStyleConfig
	if !t.dark {
		s = styles.LightStyleConfig
	}
	c := func(col string) *string { return &col }
	zero, one := uint(0), uint(1)
	yes := true

	text := hexOf(t.text)
	s.Document.Margin = &zero
	s.Document.BlockPrefix = ""
	s.Document.BlockSuffix = ""
	s.Document.Color = c(text)
	s.Paragraph.Margin = &zero

	s.Heading.Color = c(text)
	s.Heading.BackgroundColor = nil
	s.Heading.Bold = &yes
	s.H1.Prefix, s.H1.Suffix = "", ""
	s.H1.BackgroundColor = nil
	s.H1.Color = c(hexOf(t.h1))
	s.H2.Prefix = ""
	s.H2.Color = c(hexOf(t.h2))
	s.H3.Prefix = ""
	s.H3.Color = c(hexOf(t.h3))
	s.H4.Prefix = ""
	s.H4.Color = c(text)
	s.H5.Prefix = ""
	s.H5.Color = c(hexOf(t.muted))
	s.H6.Prefix = ""
	s.H6.Color = c(hexOf(t.faint))
	for _, h := range []*ansi.StyleBlock{&s.H1, &s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Bold = &yes
	}

	s.Link.Color = c(hexOf(t.info))
	s.LinkText.Color = c(hexOf(t.info))
	s.LinkText.Bold = nil
	s.Code.Color = c(hexOf(t.code))
	s.Code.BackgroundColor = nil
	s.Code.Prefix, s.Code.Suffix = "", ""
	s.Emph.Color = c(text)
	s.Strong.Color = c(hexOf(t.bone))
	s.Item.Color = c(text)
	s.Item.BlockPrefix = "• "
	s.Enumeration.Color = c(hexOf(t.muted))
	s.BlockQuote.Color = c(hexOf(t.muted))
	s.BlockQuote.IndentToken = c("│ ")
	s.HorizontalRule.Color = c(hexOf(t.line))
	s.HorizontalRule.Format = "\n────────\n"
	s.Table.Color = c(text)

	s.CodeBlock.Margin = &one
	s.CodeBlock.Color = c(text)
	s.CodeBlock.Chroma = codePalette(t)
	return s
}

func codePalette(t *theme) *ansi.Chroma {
	p := func(tt chroma.TokenType) ansi.StylePrimitive {
		c, b, i := syntaxOf(t).of(tt)
		col := hexOf(c)
		sp := ansi.StylePrimitive{Color: &col}
		if b {
			sp.Bold = &b
		}
		if i {
			sp.Italic = &i
		}
		return sp
	}
	return &ansi.Chroma{
		Text:                p(chroma.Text),
		Error:               p(chroma.Text),
		Comment:             p(chroma.Comment),
		CommentPreproc:      p(chroma.CommentPreproc),
		Keyword:             p(chroma.Keyword),
		KeywordReserved:     p(chroma.KeywordReserved),
		KeywordNamespace:    p(chroma.KeywordNamespace),
		KeywordType:         p(chroma.KeywordType),
		Operator:            p(chroma.Operator),
		Punctuation:         p(chroma.Punctuation),
		Name:                p(chroma.Name),
		NameOther:           p(chroma.NameOther),
		NameException:       p(chroma.NameException),
		Literal:             p(chroma.Literal),
		LiteralDate:         p(chroma.LiteralDate),
		NameBuiltin:         p(chroma.NameBuiltin),
		NameTag:             p(chroma.NameTag),
		NameAttribute:       p(chroma.NameAttribute),
		NameClass:           p(chroma.NameClass),
		NameConstant:        p(chroma.NameConstant),
		NameDecorator:       p(chroma.NameDecorator),
		NameFunction:        p(chroma.NameFunction),
		LiteralNumber:       p(chroma.LiteralNumber),
		LiteralString:       p(chroma.LiteralString),
		LiteralStringEscape: p(chroma.LiteralStringEscape),
		GenericDeleted:      p(chroma.GenericDeleted),
		GenericInserted:     p(chroma.GenericInserted),
		GenericSubheading:   p(chroma.GenericSubheading),
	}
}
