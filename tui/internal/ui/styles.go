package ui

import "charm.land/lipgloss/v2"

// The palette is the banner's: bone on near-black, slate for anything the eye
// should skip, ember red used only where it means something.
var (
	bone  = lipgloss.Color("#D9D2C3")
	slate = lipgloss.Color("#6F7B8F")
	ember = lipgloss.Color("#D0463B")
	brass = lipgloss.Color("#B8863F")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(bone)
	subtitleStyle = lipgloss.NewStyle().Foreground(slate)
	bodyStyle     = lipgloss.NewStyle().Foreground(bone)
	hintStyle     = lipgloss.NewStyle().Foreground(slate)
	errorStyle    = lipgloss.NewStyle().Foreground(ember)
	accentStyle   = lipgloss.NewStyle().Bold(true).Foreground(brass)
	codeStyle     = lipgloss.NewStyle().Bold(true).Foreground(bone).
			Border(lipgloss.RoundedBorder()).BorderForeground(brass).Padding(0, 2)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(slate).
			Padding(1, 2).
			Width(30)

	cardSelectedStyle = cardStyle.
				BorderForeground(ember).
				BorderStyle(lipgloss.ThickBorder())
)
