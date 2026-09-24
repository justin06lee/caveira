package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// bubble draws text as a chat bubble against the right edge of width: a
// filled block whose top and bottom edges are half a row thick and whose
// corners are cut back to quarter cells, which is as round as a terminal
// cell gets. A small tail sticks out under the right corner. Where there
// is no fill colour to draw with (16-colour and colourless terminals), it
// falls back to a rounded outline.
func bubble(text string, width int, fill, fg, outline color.Color) string {
	// Wide terminals keep the bubble to three quarters of the line, so it
	// reads as a reply-side message rather than a banner.
	maxW := min(max(width*3/4, 24), width-1)
	textCap := max(maxW-4, 8)
	wrapped := strings.Split(lipgloss.Wrap(text, textCap, ""), "\n")
	textW := 0
	for _, l := range wrapped {
		textW = max(textW, lipgloss.Width(l))
	}

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
	out = append(out, lead+edge.Render("▗"+strings.Repeat("▄", w-2)+"▖"))
	for _, l := range wrapped {
		pad := strings.Repeat(" ", textW-lipgloss.Width(l))
		out = append(out, lead+body.Render("  "+l+pad+"  "))
	}
	out = append(out, lead+edge.Render("▝"+strings.Repeat("▀", w-1)+"▘"))
	return strings.Join(out, "\n")
}
