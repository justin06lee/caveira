package ui

import "charm.land/lipgloss/v2"

// The palette is the banner's: bone on near-black, slate for anything the eye
// should skip, ember red only where it means something, brass for the
// cursor and the one thing that wants attention.
var (
	bone  = lipgloss.Color("#D9D2C3")
	dim   = lipgloss.Color("#8A8577")
	slate = lipgloss.Color("#6F7B8F")
	ember = lipgloss.Color("#D0463B")
	brass = lipgloss.Color("#B8863F")
	moss  = lipgloss.Color("#7FA35B")
	ink   = lipgloss.Color("#1A1A1A")

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(bone)
	styleBody     = lipgloss.NewStyle().Foreground(bone)
	styleDim      = lipgloss.NewStyle().Foreground(dim)
	styleSlate    = lipgloss.NewStyle().Foreground(slate)
	styleEmber    = lipgloss.NewStyle().Foreground(ember)
	styleBrass    = lipgloss.NewStyle().Foreground(brass)
	styleMoss     = lipgloss.NewStyle().Foreground(moss)
	styleThinking = lipgloss.NewStyle().Foreground(slate).Italic(true)

	styleUserMark = lipgloss.NewStyle().Foreground(ember).Bold(true)
	styleUserText = lipgloss.NewStyle().Foreground(bone)

	styleToolName = lipgloss.NewStyle().Foreground(bone).Bold(true)
	styleToolArg  = lipgloss.NewStyle().Foreground(dim)
	styleToolOut  = lipgloss.NewStyle().Foreground(dim)

	styleDiffAdd = lipgloss.NewStyle().Foreground(moss)
	styleDiffDel = lipgloss.NewStyle().Foreground(ember)
	styleDiffHdr = lipgloss.NewStyle().Foreground(slate)

	styleInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(slate).
			Padding(0, 1)
	styleInputBoxBusy = styleInputBox.BorderForeground(lipgloss.Color("#3E4451"))

	styleApprovalBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ember).
				Padding(0, 1)

	styleKey = lipgloss.NewStyle().Foreground(bone)
)
