package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// bubbleText wraps a message for a bubble against the right edge of width.
// Wide terminals keep the bubble to three quarters of the line, so it reads
// as a reply-side message rather than a banner.
func bubbleText(text string, width int) (lines []string, textW int) {
	maxW := min(max(width*3/4, 24), width-1)
	textCap := max(maxW-4, 8)
	lines = strings.Split(lipgloss.Wrap(text, textCap, ""), "\n")
	for _, l := range lines {
		textW = max(textW, lipgloss.Width(l))
	}
	return lines, textW
}

// bubble draws text as a chat bubble against the right edge of width: a
// filled block whose top and bottom edges are half a row thick, drawn with
// horizontal half cells only. Each corner is cut back by one column and
// half a row, which is about square on a cell twice as tall as it is wide,
// so it reads as rounded rather than notched. A small tail sticks out
// under the right corner. Where there is no fill colour to draw with
// (16-colour and colourless terminals), it falls back to a rounded outline.
func bubble(text string, width int, fill, fg, outline color.Color) string {
	wrapped, textW := bubbleText(text, width)

	if fill == nil {
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(outline).
			Foreground(fg).
			Padding(0, 1)
		b := box.Render(strings.Join(wrapped, "\n"))
		return lipgloss.PlaceHorizontal(width, lipgloss.Right, b)
	}

	w := textW + 4 // two cells of padding either side
	lead := strings.Repeat(" ", max(width-w-1, 0))
	edge := lipgloss.NewStyle().Foreground(fill)
	body := lipgloss.NewStyle().Background(fill).Foreground(fg)

	out := make([]string, 0, len(wrapped)+2)
	out = append(out, lead+" "+edge.Render(strings.Repeat("▄", w-2)))
	for _, l := range wrapped {
		pad := strings.Repeat(" ", textW-lipgloss.Width(l))
		out = append(out, lead+body.Render("  "+l+pad+"  "))
	}
	// The bottom edge keeps its right corner and runs one column past it:
	// the tail.
	out = append(out, lead+" "+edge.Render(strings.Repeat("▀", w)))
	return strings.Join(out, "\n")
}

// draftBubble is a message that has not been sent yet: the same size and
// place as a sent bubble, with the text in the same columns, but only a
// dashed outline where the fill would be.
func draftBubble(text string, width int, fg, outline color.Color) string {
	wrapped, textW := bubbleText(text, width)
	w := textW + 4
	lead := strings.Repeat(" ", max(width-w-1, 0))
	edge := lipgloss.NewStyle().Foreground(outline)
	body := lipgloss.NewStyle().Foreground(fg)

	out := make([]string, 0, len(wrapped)+2)
	out = append(out, lead+edge.Render("╭"+strings.Repeat("┄", w-2)+"╮"))
	for _, l := range wrapped {
		pad := strings.Repeat(" ", textW-lipgloss.Width(l))
		out = append(out, lead+edge.Render("┆")+" "+body.Render(l+pad)+" "+edge.Render("┆"))
	}
	out = append(out, lead+edge.Render("╰"+strings.Repeat("┄", w-2)+"╯"))
	return strings.Join(out, "\n")
}
