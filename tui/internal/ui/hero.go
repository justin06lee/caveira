package ui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/art"
)

// The hero is the empty session: wordmark and buttons up top, the skull in
// the middle, the input box centred under it. The first prompt slides the
// box to the bottom and the skull fades, and the transcript takes over.

const (
	slideTick   = 16 * time.Millisecond
	slideFrames = 14
	heroBoxMax  = 76
	skullMaxRow = 32
)

type slideTickMsg struct{}

type heroFrame struct {
	skullTop, skullRows, skullPX int
	boxY, boxW, boxX             int
	hintY                        int
}

func (m *Model) heroLayout() heroFrame {
	inner := m.width - 2*sideMargin
	var f heroFrame
	// Rows available between the header and the bottom hint.
	region := m.height - headerLines - 1
	// The block: skull, gap, box (3 rows), gap, hint.
	rows := region - 6
	// Leave air around the composition: the skull takes at most six tenths
	// of what is free, so the box has somewhere to slide to.
	f.skullRows = min(skullMaxRow, rows*6/10)
	if f.skullRows < 6 || inner < 20 {
		f.skullRows = 0
	}
	f.skullPX = f.skullRows * 2
	if f.skullPX > inner-4 {
		f.skullPX = (inner - 4) &^ 1
		f.skullRows = f.skullPX / 2
	}
	block := f.skullRows + 6
	if f.skullRows == 0 {
		block = 5
	}
	top := headerLines + max((region-block)/2, 0)
	f.skullTop = top
	f.boxY = top + f.skullRows + 1
	if f.skullRows == 0 {
		f.boxY = top
	}
	f.boxW = min(inner, heroBoxMax)
	f.boxX = sideMargin + (inner-f.boxW)/2
	f.hintY = f.boxY + m.input.Height() + styleInputBox.GetVerticalFrameSize() + 1
	return f
}

// beginSlide starts the transition; the submitted text is handled when the
// box lands, so the transcript appears with the prompt already in it.
func (m *Model) beginSlide(text string) tea.Cmd {
	m.phase = phaseSlide
	m.slideT = 0
	m.slideText = text
	return tea.Tick(slideTick, func(time.Time) tea.Msg { return slideTickMsg{} })
}

func (m *Model) updateSlide(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case slideTickMsg:
		m.slideT += 1.0 / slideFrames
		if m.slideT < 1 {
			return m, tea.Tick(slideTick, func(time.Time) tea.Msg { return slideTickMsg{} })
		}
		m.phase = phaseSession
		m.dirty = true
		m.layout()
		text := m.slideText
		m.slideText = ""
		return m, m.submit(text)
	case tea.KeyPressMsg:
		// Keys during the slide go nowhere; it is over in a quarter second.
		return m, nil
	}
	return m, nil
}

func easeOut(t float64) float64 {
	t = math.Min(math.Max(t, 0), 1)
	return 1 - math.Pow(1-t, 3)
}

// renderHero draws the hero at slide progress t (0 = resting).
func (m *Model) renderHero(t float64) (string, *tea.Cursor) {
	if m.width == 0 || m.height == 0 {
		return "", nil
	}
	inner := m.width - 2*sideMargin
	f := m.heroLayout()
	e := easeOut(t)
	lines := make([]string, m.height)

	header := strings.Split(m.renderHeader(), "\n")
	for i := 0; i < headerLines && i < len(lines); i++ {
		if i < len(header) {
			lines[i] = header[i]
		}
	}

	if f.skullRows > 0 && e < 1 {
		key := [2]int{f.skullPX, int(e * 100)}
		if m.skullCache == nil || m.skullKey != key {
			m.skullCache = art.Render(art.Skull(f.skullPX), art.Options{
				Brightness: 1 - e, Scanlines: true, Grain: true,
			})
			m.skullKey = key
		}
		for i, l := range m.skullCache {
			y := f.skullTop + i
			if y >= m.height {
				break
			}
			lines[y] = centerRaw(l, f.skullPX, inner)
		}
	}

	// The box travels from its hero spot to the bottom and widens to the
	// full width on the way.
	bottomY := m.height - m.bottomHeight()
	boxY := int(math.Round(float64(f.boxY) + (float64(bottomY)-float64(f.boxY))*e))
	boxW := int(math.Round(float64(f.boxW) + (float64(inner)-float64(f.boxW))*e))
	boxX := sideMargin + (inner-boxW)/2
	m.input.SetWidth(boxW - styleInputBox.GetHorizontalFrameSize())
	box := strings.Split(m.renderInputBox(boxW), "\n")
	for i, l := range box {
		y := boxY + i
		if y < 0 || y >= m.height {
			continue
		}
		lines[y] = strings.Repeat(" ", boxX-sideMargin) + l
	}

	if t == 0 {
		hint := styleSlate.Render("enter send · / commands · esc interrupts")
		if m.fatal != nil {
			hint = styleEmber.Render(lipgloss.Wrap("✗ "+m.fatal.Error(), min(inner-2, 90), ""))
		}
		for i, hl := range strings.Split(hint, "\n") {
			y := f.hintY + i
			if y < m.height {
				lines[y] = centerRaw(hl, lipgloss.Width(hl), inner)
			}
		}
	}

	margin := strings.Repeat(" ", sideMargin)
	for i := range lines {
		lines[i] = margin + lines[i]
	}

	var cur *tea.Cursor
	if t == 0 && m.input.Focused() {
		if c := m.input.Cursor(); c != nil {
			c.Position.X += boxX + 2 + 1
			c.Position.Y += boxY + 1
			cur = c
		}
	}
	return strings.Join(lines, "\n"), cur
}
