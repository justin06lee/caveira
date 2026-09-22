package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
)

func testModel(t *testing.T, w, h int) *Model {
	t.Helper()
	m := New("test", nil)
	ag := agent.New(llm.New("http://localhost:1", ""), config.Settings{Model: "abliterated-model"}, "/tmp/project", "sys")
	m.Apply(Options{Agent: ag, Settings: config.Settings{Model: "abliterated-model"}, WorkDir: "/tmp/project"})
	m.width, m.height = w, h
	m.phase = phaseHero
	m.layout()
	return m
}

// dump writes a frame for eyeballing when CAVEIRA_DUMP names a directory.
func dump(t *testing.T, name, frame string) {
	t.Helper()
	dir := os.Getenv("CAVEIRA_DUMP")
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(frame), 0o644); err != nil {
		t.Fatal(err)
	}
}

func boxRow(frame string) int {
	for i, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, "❯") && strings.Contains(l, "│") {
			return i
		}
	}
	return -1
}

func TestHeroSlideMovesBoxDownAndFadesSkull(t *testing.T) {
	m := testModel(t, 110, 42)

	rest, cur := m.renderHero(0)
	dump(t, "slide-0", rest)
	if cur == nil {
		t.Fatal("hero should place the cursor")
	}
	restRow := boxRow(rest)
	if restRow < headerLines+6 || restRow > m.height-8 {
		t.Fatalf("hero box row %d not centred in %d rows", restRow, m.height)
	}
	if !strings.Contains(rest, "\x1b[38;2;") {
		t.Fatal("skull not drawn in colour")
	}
	if !strings.Contains(rest, "options") {
		t.Fatal("header buttons missing")
	}

	mid, _ := m.renderHero(0.5)
	dump(t, "slide-50", mid)
	midRow := boxRow(mid)
	if midRow <= restRow {
		t.Fatalf("box did not move down at t=0.5: %d vs %d", midRow, restRow)
	}

	end, _ := m.renderHero(1)
	dump(t, "slide-100", end)
	endRow := boxRow(end)
	if endRow <= midRow || endRow != m.height-m.bottomHeight()+1 {
		t.Fatalf("box did not land at the bottom: rest %d mid %d end %d height %d", restRow, midRow, endRow, m.height)
	}
	if strings.Contains(end, "▀\x1b[0m") && strings.Count(end, "\x1b[38;2;") > 200 {
		t.Fatal("skull still drawn at the end of the slide")
	}
	// The landed box is as wide as the session's.
	lines := strings.Split(end, "\n")
	if w := strings.Count(lines[endRow-1], "─"); w < m.width-2*sideMargin-4 {
		t.Fatalf("landed box too narrow: %d dashes for width %d", w, m.width)
	}
}

func TestSplashSweepsTopToBottom(t *testing.T) {
	m := testModel(t, 110, 42)
	m.phase = phaseSplash
	f := m.splashLayout()
	if f.skullRows == 0 || f.wordRows == 0 {
		t.Fatalf("splash layout empty: %+v", f)
	}
	m.splash.revealed = 3
	early := m.renderSplash()
	dump(t, "splash-early", early)
	m.splash.revealed = (f.last - f.first + 1) / 2
	half := m.renderSplash()
	dump(t, "splash-half", half)
	m.splash.done = true
	full := m.renderSplash()
	dump(t, "splash-full", full)

	count := func(s string) int { return strings.Count(s, "\x1b[38;2;") }
	if !(count(early) < count(half) && count(half) < count(full)) {
		t.Fatalf("sweep not progressive: %d %d %d", count(early), count(half), count(full))
	}
	// The wordmark sits under the skull once everything is drawn.
	lines := strings.Split(full, "\n")
	if strings.TrimSpace(lines[f.wordTop]) == "" {
		t.Fatal("wordmark row empty in the finished splash")
	}
}

func TestSmallTerminalDropsSkull(t *testing.T) {
	m := testModel(t, 60, 14)
	f := m.heroLayout()
	if f.skullRows != 0 {
		t.Fatalf("expected no skull at 60x14, got %d rows", f.skullRows)
	}
	frame, _ := m.renderHero(0)
	if boxRow(frame) < 0 {
		t.Fatal("input box missing on a small terminal")
	}
}
