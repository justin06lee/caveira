package ui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// markdown renders assistant text at a fixed width. It is rebuilt when the
// terminal is resized; rendering is cached per transcript item.
type markdown struct {
	width int
	tr    *glamour.TermRenderer
}

func newMarkdown(width int) *markdown {
	if width < 20 {
		width = 20
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(transcriptStyle()),
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
	out, err := m.tr.Render(src)
	if err != nil {
		return src
	}
	// glamour pads the document with blank lines top and bottom; the
	// transcript spaces items itself.
	return strings.Trim(out, "\n")
}

func transcriptStyle() ansi.StyleConfig {
	s := styles.DarkStyleConfig
	zero := uint(0)
	one := uint(1)
	s.Document.Margin = &zero
	s.Document.BlockPrefix = ""
	s.Document.BlockSuffix = ""
	s.Document.Color = strPtr("#D9D2C3")
	s.Paragraph.Margin = &zero
	s.CodeBlock.Margin = &one
	s.Heading.Color = strPtr("#D9D2C3")
	s.Heading.BackgroundColor = nil
	s.H1.Prefix = ""
	s.H1.Suffix = ""
	s.H1.BackgroundColor = nil
	s.H1.Color = strPtr("#D9D2C3")
	s.H2.Prefix = "## "
	s.H3.Prefix = "### "
	s.Link.Color = strPtr("#6F7B8F")
	s.LinkText.Color = strPtr("#B8863F")
	s.Code.Color = strPtr("#B8863F")
	s.Code.BackgroundColor = nil
	s.Emph.Color = strPtr("#D9D2C3")
	s.Strong.Color = strPtr("#D9D2C3")
	s.Item.Color = strPtr("#D9D2C3")
	s.Enumeration.Color = strPtr("#8A8577")
	s.BlockQuote.Color = strPtr("#8A8577")
	s.HorizontalRule.Color = strPtr("#3E4451")
	return s
}

func strPtr(s string) *string { return &s }
