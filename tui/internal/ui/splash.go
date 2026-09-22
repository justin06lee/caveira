package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/tui/internal/art"
)

// The splash is the first frame on screen: the skull drawn in row by row
// like a CRT warming up, with the wordmark under it. It plays while the
// real startup work (config, git, session) runs on another goroutine, and
// ends when both the sweep and the work are done.

const (
	splashTick = 14 * time.Millisecond
	splashHold = 10 // ticks to linger once fully drawn
)

type splashTickMsg struct{}

type splash struct {
	revealed int // rows of the frame drawn so far
	hold     int
	done     bool
	skipped  bool
}

func splashTicker() tea.Cmd {
	return tea.Tick(splashTick, func(time.Time) tea.Msg { return splashTickMsg{} })
}

// splashFrame lays out the splash for the current size: where the skull
// sits (the same place the hero puts it, so nothing jumps at the cut) and
// where the wordmark goes.
type splashFrame struct {
	skullTop, skullRows, skullPX int
	wordTop, wordScale, wordRows int
	first, last                  int // rows the sweep covers
}

func (m *Model) splashLayout() splashFrame {
	h := m.heroLayout()
	f := splashFrame{skullTop: h.skullTop, skullRows: h.skullRows, skullPX: h.skullPX}
	f.wordScale = 1
	if m.width-2*sideMargin >= art.WordmarkWidth(2)+4 && m.height >= 30 {
		f.wordScale = 2
	}
	f.wordRows = len(art.Wordmark(f.wordScale, 0, 0, 0, art.Options{}))
	f.wordTop = f.skullTop + f.skullRows + 1
	if f.skullRows == 0 {
		f.wordTop = max((m.height-f.wordRows)/2, 0)
	}
	f.first = max(f.skullTop-1, 0)
	f.last = min(f.wordTop+f.wordRows, m.height-1)
	return f
}

func (m *Model) updateSplash(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case splashTickMsg:
		if m.width == 0 {
			return m, splashTicker()
		}
		f := m.splashLayout()
		total := f.last - f.first + 1
		if m.splash.revealed < total {
			step := 1
			if total > 44 {
				step = 2
			}
			m.splash.revealed = min(m.splash.revealed+step, total)
		} else if m.splash.hold < splashHold {
			m.splash.hold++
		} else {
			m.splash.done = true
		}
		if m.splash.done && m.booted {
			return m.leaveSplash()
		}
		return m, splashTicker()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Any other key skips the sweep; the cut still waits for boot.
		m.splash.skipped = true
		m.splash.done = true
		if m.booted {
			return m.leaveSplash()
		}
		return m, nil
	}
	return m, nil
}

// leaveSplash cuts to the hero, or straight into a resumed session.
func (m *Model) leaveSplash() (tea.Model, tea.Cmd) {
	if m.bootErr != nil {
		return m, tea.Quit
	}
	m.layout()
	if len(m.items) > 0 {
		m.phase = phaseSession
		m.dirty = true
		m.layout()
		if m.initial != "" {
			text := m.initial
			m.initial = ""
			return m, m.submit(text)
		}
		return m, nil
	}
	m.phase = phaseHero
	if m.initial != "" {
		text := m.initial
		m.initial = ""
		return m, m.submit(text)
	}
	return m, nil
}

func (m *Model) renderSplash() string {
	lines := make([]string, m.height)
	if m.width == 0 || m.height == 0 {
		return ""
	}
	f := m.splashLayout()
	inner := m.width - 2*sideMargin
	limit := f.first + m.splash.revealed // rows strictly below this are not drawn yet
	if m.splash.done || m.splash.skipped {
		limit = m.height
	}

	// The beam: the rows drawn most recently glow, then settle.
	beam := func(absRow int) float64 {
		if m.splash.done {
			return 1
		}
		switch limit - 1 - absRow {
		case 0:
			return 1.9
		case 1:
			return 1.4
		case 2:
			return 1.15
		}
		return 1
	}

	if f.skullRows > 0 {
		skull := art.Render(art.Skull(f.skullPX), art.Options{
			Scanlines: true, Grain: true,
			RowBrightness: func(row int) float64 { return beam(f.skullTop + row) },
		})
		for i, l := range skull {
			y := f.skullTop + i
			if y >= limit || y >= m.height {
				break
			}
			lines[y] = centerRaw(l, f.skullPX, inner)
		}
	}
	word := art.Wordmark(f.wordScale, 0xD9, 0xD2, 0xC3, art.Options{
		RowBrightness: func(row int) float64 { return beam(f.wordTop + row) },
	})
	for i, l := range word {
		y := f.wordTop + i
		if y >= limit || y >= m.height {
			break
		}
		lines[y] = centerRaw(l, art.WordmarkWidth(f.wordScale), inner)
	}
	// The beam itself on the row being drawn, across the whole width.
	if !m.splash.done && limit-1 >= 0 && limit-1 < m.height && lines[limit-1] == "" {
		lines[limit-1] = styleSlate.Render(strings.Repeat("─", inner))
	}

	margin := strings.Repeat(" ", sideMargin)
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return strings.Join(lines, "\n")
}

// centerRaw pads a line of known cell width to sit centred in width. Raw
// because the art lines carry their own escape codes and lipgloss should
// not re-measure them every frame.
func centerRaw(line string, lineWidth, width int) string {
	pad := (width - lineWidth) / 2
	if pad <= 0 {
		return line
	}
	return strings.Repeat(" ", pad) + line
}
