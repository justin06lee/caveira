package ui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
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

func (m *markdown) render(src string) string {
	if m == nil || m.tr == nil {
		return src
	}
	// Tabs measure as nothing and draw as several cells; expand them first.
	out, err := m.tr.Render(strings.ReplaceAll(src, "\t", "    "))
	if err != nil {
		return src
	}
	// glamour pads the document with blank lines top and bottom, and pads
	// every line to the wrap width; the transcript spaces items itself.
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// transcriptStyle is glamour's base style recoloured to the theme: quiet
// headings, bone emphasis, inline code in brass, and a syntax palette cut
// from the same few colours so code blocks sit in the page instead of on it.
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
	s.H1.Color = c(hexOf(t.bone))
	s.H2.Prefix = ""
	s.H2.Color = c(hexOf(t.bone))
	s.H3.Prefix = ""
	s.H4.Prefix = ""
	s.H5.Prefix = ""
	s.H6.Prefix = ""
	s.H6.Color = c(hexOf(t.muted))

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
	p := func(col string) ansi.StylePrimitive { return ansi.StylePrimitive{Color: &col} }
	italic := true
	bold := true
	type pal struct{ text, comment, keyword, typ, op, punct, fn, builtin, num, str, esc string }
	v := pal{"#D6D4C8", "#6E7068", "#E5836B", "#C9CCB9", "#A9ABA1", "#8E9087", "#9FC2E0", "#D9B574", "#C5A3D9", "#A8C98C", "#D9B574"}
	if !t.dark {
		v = pal{"#2F302A", "#8A8C83", "#B8472F", "#4F5243", "#55574F", "#6B6D64", "#2F6391", "#8A5A12", "#7A4B96", "#4A7D2C", "#8A5A12"}
	}
	comment := p(v.comment)
	comment.Italic = &italic
	class := p(v.text)
	class.Bold = &bold
	return &ansi.Chroma{
		Text:                p(v.text),
		Error:               p(hexOf(t.err)),
		Comment:             comment,
		CommentPreproc:      p(v.keyword),
		Keyword:             p(v.keyword),
		KeywordReserved:     p(v.keyword),
		KeywordNamespace:    p(v.keyword),
		KeywordType:         p(v.typ),
		Operator:            p(v.op),
		Punctuation:         p(v.punct),
		Name:                p(v.text),
		NameOther:           p(v.text),
		NameException:       p(v.typ),
		Literal:             p(v.str),
		LiteralDate:         p(v.num),
		NameBuiltin:         p(v.builtin),
		NameTag:             p(v.keyword),
		NameAttribute:       p(v.fn),
		NameClass:           class,
		NameConstant:        p(v.num),
		NameDecorator:       p(v.builtin),
		NameFunction:        p(v.fn),
		LiteralNumber:       p(v.num),
		LiteralString:       p(v.str),
		LiteralStringEscape: p(v.esc),
		GenericDeleted:      p(hexOf(t.delFg)),
		GenericInserted:     p(hexOf(t.addFg)),
		GenericSubheading:   p(v.comment),
	}
}
