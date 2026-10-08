package ui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/justin06lee/caveira/tui/internal/art"
)

// The home screen is the empty session: the skull, the name, and the
// prompt centred under them. It is also the boot screen: while settings,
// git state, and the last session load on another goroutine, the skull
// dissolves in pixel by pixel and the name types itself in above the empty
// spot where the box will be, so the cut to a usable screen moves nothing.
// The first prompt slides the box to the bottom as the skull dissolves out,
// and the transcript takes over.

const (
	animTick   = 16 * time.Millisecond
	bootFrames = 48 // the intro, about three quarters of a second
	slideTicks = 16
	homeBoxMax = 76
)

type splashTickMsg struct{}
type slideTickMsg struct{}

type splash struct {
	frame int
	done  bool
}

func splashTicker() tea.Cmd {
	return tea.Tick(animTick, func(time.Time) tea.Msg { return splashTickMsg{} })
}

// homeFrame is where everything on the home screen goes, in rows.
type homeFrame struct {
	scale     int // mascot scale; 0 leaves it out
	mascotTop int
	mascotX   int // column, inside the side margin
	titleY    int
	tagY      int
	boxY      int
	boxW      int
	boxX      int
	infoY     int
	tipsY     int
}

// homeTitle is the name as the home screen and the session card set it:
// capitals, spaced out, in bone.
const homeTitle = "C A V E I R A"

// homeLayout stacks the skull over the title, as large as the height
// allows, and leaves the skull out on very small terminals.
func (m *Model) homeLayout() homeFrame {
	inner := m.inner()
	var f homeFrame
	boxRows := m.input.Height() + 2
	// title, gap, tagline, gap, box, info, gap, tips
	fixed := 1 + 1 + 1 + 1 + boxRows + 1 + 1 + 1
	for _, s := range []struct{ scale, minH int }{{3, 40}, {2, 28}, {1, 20}} {
		if m.height >= s.minH && inner >= art.MascotWidth(s.scale)+4 {
			f.scale = s.scale
			break
		}
	}
	total := fixed
	if f.scale > 0 {
		total += art.MascotHeight(f.scale) + 1
	}
	// Sit a little above the middle, where the eye expects the centre.
	y := max(int(float64(m.height-total)*0.42), 0)
	center := func(w int) int { return (inner - w) / 2 }
	if f.scale > 0 {
		f.mascotTop = y
		f.mascotX = center(art.MascotWidth(f.scale))
		y += art.MascotHeight(f.scale) + 1
	}
	f.titleY = y
	f.tagY = y + 2
	y += 3 + 1
	f.boxY = y
	f.boxW = min(inner, homeBoxMax)
	f.boxX = center(f.boxW)
	y += boxRows
	f.infoY = y
	f.tipsY = y + 2
	return f
}

func (m *Model) updateSplash(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case splashTickMsg:
		if m.splash.done {
			return m, nil
		}
		m.splash.frame++
		if m.splash.frame >= bootFrames && m.booted {
			return m.leaveSplash()
		}
		// Keep ticking past the intro while boot finishes: the loading
		// spinner in the box's spot needs frames.
		return m, splashTicker()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Any other key skips the intro; the cut still waits for boot.
		// Typed text is kept for the box, so starting to type before the
		// intro ends loses nothing.
		if msg.Text != "" {
			m.input, _ = m.input.Update(msg)
		}
		m.splash.frame = max(m.splash.frame, bootFrames)
		if m.booted {
			return m.leaveSplash()
		}
		return m, nil
	}
	return m, nil
}

// leaveSplash cuts to the home screen, or straight into a resumed session.
func (m *Model) leaveSplash() (tea.Model, tea.Cmd) {
	if m.bootErr != nil {
		return m, tea.Quit
	}
	m.splash.done = true
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
	m.layout()
	if m.initial != "" {
		text := m.initial
		m.initial = ""
		return m, m.submit(text)
	}
	return m, nil
}

// beginSlide starts the transition; the submitted text is handled when the
// box lands, so the transcript appears with the prompt already in it.
func (m *Model) beginSlide(text string) tea.Cmd {
	m.phase = phaseSlide
	m.slideT = 0
	m.slideText = text
	return tea.Tick(animTick, func(time.Time) tea.Msg { return slideTickMsg{} })
}

func (m *Model) updateSlide(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case slideTickMsg:
		m.slideT += 1.0 / slideTicks
		if m.slideT < 1 {
			return m, tea.Tick(animTick, func(time.Time) tea.Msg { return slideTickMsg{} })
		}
		m.phase = phaseSession
		m.pushHeader()
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

// renderHome draws the home screen at slide progress t (0 is resting).
// During boot it plays the intro instead of showing the box.
func (m *Model) renderHome(t float64) (string, *tea.Cursor) {
	if m.width == 0 || m.height == 0 {
		return "", nil
	}
	inner := m.inner()
	f := m.homeLayout()
	e := easeOut(t)
	lines := make([]string, m.height)
	put := func(y, x int, s string) {
		if y >= 0 && y < len(lines) {
			lines[y] = strings.Repeat(" ", max(x, 0)) + s
		}
	}
	center := func(w int) int { return (inner - w) / 2 }

	booting := m.phase == phaseSplash
	frame := bootFrames
	if booting {
		frame = m.splash.frame
	}
	// progress of a stretch of the intro, 0..1
	span := func(from, to int) float64 {
		return math.Min(math.Max(float64(frame-from)/float64(to-from), 0), 1)
	}

	// The skull.
	if f.scale > 0 && e < 1 {
		o := art.MascotOptions{Style: th.mascot}
		if in := span(0, 30); in < 1 {
			o.Hidden = 1 - easeOut(in)
			o.Flash = 0.9
		}
		if e > 0 {
			o.Hidden = e
		}
		for i, l := range art.Mascot(f.scale, o) {
			put(f.mascotTop+i, f.mascotX, l)
		}
	}

	// The name types itself in under the skull, and backs out the same way.
	if reveal := easeOut(span(10, 34)) * (1 - math.Min(e*1.8, 1)); reveal > 0 {
		n := len([]rune(homeTitle))
		shown := string([]rune(homeTitle)[:int(math.Round(reveal*float64(n)))])
		put(f.titleY, center(n), th.Bone.Bold(true).Render(shown))
	}

	if t == 0 && frame >= 30 {
		tag := th.Muted.Render("coding agent for abliterated models")
		if m.version != "" && m.version != "dev" {
			tag += th.Faint.Render("  " + m.version)
		}
		put(f.tagY, center(lipgloss.Width(tag)), tag)
	}

	// The box, or the loading line where it will be.
	var cur *tea.Cursor
	switch {
	case booting && frame >= bootFrames:
		msg := th.Accent.Render(spinnerFrames[(frame/5)%len(spinnerFrames)]) + th.Faint.Render(" loading session…")
		put(f.boxY+1, center(lipgloss.Width(msg)), msg)
	case !booting:
		bottomY := m.height - footerRows - (m.input.Height() + 2)
		boxY := int(math.Round(float64(f.boxY) + float64(bottomY-f.boxY)*e))
		boxW := int(math.Round(float64(f.boxW) + float64(inner-f.boxW)*e))
		boxX := int(math.Round(float64(f.boxX) * (1 - e)))
		m.input.SetWidth(boxW - inputChrome)
		for i, l := range strings.Split(m.renderInputBox(boxW), "\n") {
			put(boxY+i, boxX, l)
		}
		if t == 0 {
			m.renderHomeInfo(lines, f)
			if m.input.Focused() {
				if c := m.input.Cursor(); c != nil {
					c.Position.X += sideMargin + boxX + inputLeft
					c.Position.Y += boxY + 1
					cur = c
				}
			}
		}
	}

	margin := strings.Repeat(" ", sideMargin)
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return strings.Join(lines, "\n"), cur
}

// renderHomeInfo fills in the lines under the box: the model and directory
// under its edges, the palette or the key tips below, and the startup
// error if there is one.
func (m *Model) renderHomeInfo(lines []string, f homeFrame) {
	put := func(y, x int, s string) {
		if y >= 0 && y < len(lines) {
			lines[y] = strings.Repeat(" ", max(x, 0)) + s
		}
	}
	inner := m.inner()
	if m.agent != nil {
		name, _ := m.agent.ModelInfo()
		left := th.Muted.Render(name)
		left += th.Faint.Render(" · " + effortName(m.agent.Effort) + " effort")
		if m.cfg.Confirm {
			left += th.Faint.Render(" · ") + th.Warn.Render("confirm")
		}
		right := th.Faint.Render(shortPath(m.workDir))
		if m.branch != "" {
			right += th.Faint.Render(" on ") + th.Muted.Render(m.branch)
		}
		gap := f.boxW - 4 - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			right, gap = "", 1
		}
		put(f.infoY, f.boxX+2, left+strings.Repeat(" ", gap)+right)
	}

	if m.fatal != nil {
		msg := lipgloss.Wrap("✗ "+m.fatal.Error(), min(inner-4, f.boxW-4), "")
		for i, l := range strings.Split(msg, "\n") {
			put(f.tipsY+i, f.boxX+2, th.Err.Render(l))
		}
		return
	}
	if p := m.renderPalette(f.boxW); p != "" {
		for i, l := range strings.Split(p, "\n") {
			put(f.infoY+1+i, f.boxX, l)
		}
		return
	}
	key := func(k, what string) string { return th.Muted.Render(k) + " " + th.Faint.Render(what) }
	hints := []string{key("/", "commands"), key("!", "shell"), key("↑", "history"), key("ctrl+c", "quit")}
	sep := th.Faint.Render("   ·   ")
	for len(hints) > 1 && lipgloss.Width(strings.Join(hints, sep)) > inner {
		hints = hints[:len(hints)-1]
	}
	tips := strings.Join(hints, sep)
	put(f.tipsY, (inner-lipgloss.Width(tips))/2, tips)
}

// pushHeader opens the transcript with the session card.
func (m *Model) pushHeader() {
	if len(m.items) > 0 && m.items[0].kind == itemHeader {
		return
	}
	h := &headerInfo{version: m.version, dir: m.workDir, branch: m.branch, confirm: m.cfg.Confirm}
	if m.agent != nil {
		h.model, h.effort, h.context = m.agent.Model, m.agent.Effort, m.agent.ContextWindow
	}
	m.items = append([]*item{{kind: itemHeader, header: h}}, m.items...)
	m.dirty = true
}
