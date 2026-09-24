package ui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
	"github.com/justin06lee/caveira/tui/internal/tools"
)

func testModel(t *testing.T, w, h int) *Model {
	t.Helper()
	m := New("v0.4.0", nil)
	ag := agent.New(llm.New("http://localhost:1", ""), config.Settings{Model: "abliterated-model", ReasoningEffort: "high"}, "/tmp/project", "sys")
	m.Apply(Options{Agent: ag, Settings: config.Settings{Model: "abliterated-model"}, WorkDir: "/tmp/project", Branch: "master"})
	m.width, m.height = w, h
	m.phase = phaseHero
	m.layout()
	t.Cleanup(func() { th = newTheme(true, colorprofile.TrueColor, nil) })
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

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func plain(s string) string { return sgr.ReplaceAllString(s, "") }

// boxRow is the row of the input box's text line.
func boxRow(frame string) int {
	for i, l := range strings.Split(plain(frame), "\n") {
		if strings.Contains(l, "│ ›") {
			return i
		}
	}
	return -1
}

func TestHomeSlidesBoxDownAndDissolvesSkull(t *testing.T) {
	m := testModel(t, 110, 40)

	rest, cur := m.renderHome(0)
	dump(t, "home", rest)
	if cur == nil {
		t.Fatal("home should place the cursor")
	}
	restRow := boxRow(rest)
	if restRow < 10 || restRow > m.height-5 {
		t.Fatalf("home box row %d not placed sensibly in %d rows", restRow, m.height)
	}
	if !strings.Contains(rest, "\x1b[48;2;191;193;176m") {
		t.Fatal("colour mascot not drawn")
	}
	p := plain(rest)
	for _, want := range []string{"abliterated-model", "commands", "master"} {
		if !strings.Contains(p, want) {
			t.Fatalf("home missing %q", want)
		}
	}

	mid, _ := m.renderHome(0.5)
	dump(t, "slide-50", mid)
	midRow := boxRow(mid)
	if midRow <= restRow {
		t.Fatalf("box did not move down at t=0.5: %d vs %d", midRow, restRow)
	}

	end, _ := m.renderHome(1)
	dump(t, "slide-100", end)
	endRow := boxRow(end)
	if want := m.height - footerRows - 2; endRow != want {
		t.Fatalf("box did not land at the bottom: rest %d mid %d end %d, want %d", restRow, midRow, endRow, want)
	}
	if strings.Contains(end, "\x1b[48;2;191;193;176m") {
		t.Fatal("skull still drawn at the end of the slide")
	}
	lines := strings.Split(plain(end), "\n")
	if w := strings.Count(lines[endRow-1], "─"); w < m.inner()-2 {
		t.Fatalf("landed box too narrow: %d dashes for width %d", w, m.width)
	}
}

func TestIntroDissolvesIn(t *testing.T) {
	m := testModel(t, 110, 40)
	m.phase = phaseSplash
	count := func() int {
		s, _ := m.renderHome(0)
		return strings.Count(s, "▀") + strings.Count(s, "▄")
	}
	m.splash.frame = 4
	early := count()
	f, _ := m.renderHome(0)
	dump(t, "intro-early", f)
	m.splash.frame = 16
	mid := count()
	m.splash.frame = bootFrames
	full := count()
	f, _ = m.renderHome(0)
	dump(t, "intro-full", f)
	if !(early < mid && mid < full) {
		t.Fatalf("intro not progressive: %d %d %d", early, mid, full)
	}
	if !strings.Contains(plain(f), "loading session") {
		t.Fatal("no loading line while boot is still running")
	}
}

func TestMonoTerminalGetsOneColourSkull(t *testing.T) {
	m := testModel(t, 110, 40)
	m.profile = colorprofile.ANSI
	m.applyTheme()
	frame, _ := m.renderHome(0)
	dump(t, "home-mono", frame)
	if strings.Contains(frame, "48;2;191;193;176") {
		t.Fatal("colour mascot drawn in a 16-colour terminal")
	}
	if !strings.Contains(frame, "█") {
		t.Fatal("mono mascot missing")
	}
}

func TestHomeShowsStartupError(t *testing.T) {
	m := testModel(t, 110, 40)
	m.fatal = errors.New("no API key. Put ABLITERATION_API_KEY in your environment or in a .env.local in the project")
	frame, _ := m.renderHome(0)
	dump(t, "home-fatal", frame)
	if !strings.Contains(plain(frame), "no API key") {
		t.Fatal("startup error not shown")
	}
}

func TestSmallTerminalDropsSkull(t *testing.T) {
	m := testModel(t, 60, 16)
	if f := m.homeLayout(); f.scale != 0 {
		t.Fatalf("expected no skull at 60x16, got scale %d", f.scale)
	}
	frame, _ := m.renderHome(0)
	if boxRow(frame) < 0 {
		t.Fatal("input box missing on a small terminal")
	}
}

// session builds a transcript with one of everything.
func session(t *testing.T, w, h int) *Model {
	m := testModel(t, w, h)
	m.phase = phaseSession
	m.pushHeader()
	m.push(&item{kind: itemUser, text: "tighten up the greeting and make sure it still runs"})
	m.push(&item{kind: itemReasoning, text: "The user wants the greeting tightened. Look at main.go first.", elapsed: 3 * time.Second})
	m.push(&item{kind: itemAssistant, text: "I'll look at the entry point first."})
	m.push(&item{kind: itemTool, toolName: "read_file", preview: "main.go", result: &tools.Result{Summary: "27 lines", Output: "1\tpackage main"}})
	m.push(&item{kind: itemTool, toolName: "edit_file", preview: "main.go", result: &tools.Result{
		Summary: "1 replacement",
		Diff:    "@@ -8,5 +8,6 @@\n func greet(name string) string {\n-\treturn \"hi \" + name\n+\tname = strings.TrimSpace(name)\n+\treturn fmt.Sprintf(\"Hello, %s!\", name)\n }\n \n func main() {",
	}})
	m.push(&item{kind: itemTool, toolName: "bash", preview: "go vet ./... && go run . --name caveira", result: &tools.Result{
		Summary: "exit 0 · 1 line · 0.4s", Output: "Hello, caveira!\n[no output, exit code 0]",
	}})
	m.push(&item{kind: itemTool, toolName: "bash", preview: "go test ./...", result: &tools.Result{
		Summary: "exit 1 · 3 lines · 1.2s", IsError: true, Output: "--- FAIL: TestGreet (0.00s)\n    main_test.go:9: got \"Hello, x!\"\nFAIL\n[exit code 1]",
	}})
	m.push(&item{kind: itemAssistant, text: "## What changed\n\n- `greet` trims the name before using it\n- the greeting is now **Hello, name!**\n\n```go\nfunc greet(name string) string {\n\tname = strings.TrimSpace(name)\n\treturn fmt.Sprintf(\"Hello, %s!\", name)\n}\n```\n\nThe demo runs clean."})
	m.push(&item{kind: itemDivider, quiet: true, text: "worked for 14s · 4 tool calls · $0.0089"})
	m.push(&item{kind: itemDivider, text: "interrupted · tell caveira what to do instead", warn: true})
	m.push(&item{kind: itemError, text: "stream: 500 Internal Server Error"})
	m.totals = agent.Totals{Requests: 4, InputTokens: 21000, OutputTokens: 900, CostUSD: 0.0237}
	m.context = 61000
	m.layout()
	return m
}

func TestSessionFrame(t *testing.T) {
	m := session(t, 110, 60)
	frame := m.View().Content
	dump(t, "session", frame)
	p := plain(frame)
	for _, want := range []string{"caveira", "Edit main.go", "+2 −1", "Hello, caveira!", "What changed", "abliterated-model", "23%", " 8   func greet", "10 +"} {
		if !strings.Contains(p, want) {
			t.Errorf("session frame missing %q", want)
		}
	}
	if strings.Contains(p, "[no output, exit code 0]") {
		t.Error("model-only bash footer shown to the user")
	}
	if n := len(strings.Split(frame, "\n")); n != m.height {
		t.Errorf("frame has %d lines, want %d", n, m.height)
	}

	// Running: spinner and activity in the status line.
	m.running = true
	m.turnStart = time.Now().Add(-4 * time.Second)
	m.push(&item{kind: itemTool, toolName: "bash", preview: "go test ./...", running: true, started: time.Now()})
	m.streamed = 4800
	frame = m.View().Content
	dump(t, "session-running", frame)
	if p := plain(frame); !strings.Contains(p, "Running go test") || !strings.Contains(p, "1.2k tokens") {
		t.Error("status line does not say what is running")
	}
}

func TestPaletteCompletesCommands(t *testing.T) {
	m := session(t, 110, 40)
	m.input.SetValue("/mo")
	if got := len(m.paletteMatches()); got != 2 {
		t.Fatalf("want /model and /models, got %d matches", got)
	}
	m.layout()
	frame := m.View().Content
	dump(t, "palette", frame)
	if !strings.Contains(plain(frame), "list the models") {
		t.Fatal("palette not drawn")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if v := m.input.Value(); v != "/models" {
		t.Fatalf("tab completed to %q", v)
	}
}

func TestHomePaletteReplacesTips(t *testing.T) {
	m := testModel(t, 110, 40)
	m.input.SetValue("/")
	frame, _ := m.renderHome(0)
	dump(t, "home-palette", frame)
	p := plain(frame)
	if !strings.Contains(p, "/compact") || strings.Contains(p, "ctrl+c quit") {
		t.Fatal("palette should replace the tips on the home screen")
	}
}

func TestApprovalCardChoosesWithArrows(t *testing.T) {
	m := session(t, 110, 40)
	reply := make(chan agent.Decision, 1)
	m.running = true
	m.approval = &agent.ApprovalEvent{Name: "bash", Preview: "rm -rf build && go build ./...", Kind: tools.KindExecute, Reply: reply}
	m.layout()
	frame := m.View().Content
	dump(t, "approval", frame)
	p := plain(frame)
	if !strings.Contains(p, "Permission") || !strings.Contains(p, "don't ask again for Bash") {
		t.Fatal("approval card not drawn")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if d := <-reply; d != agent.AllowAlways {
		t.Fatalf("decision %v, want AllowAlways", d)
	}
}

func TestLightTerminalGetsLightPalette(t *testing.T) {
	m := session(t, 110, 50)
	m.Update(tea.BackgroundColorMsg{Color: hex("#FAFAF7")})
	if th.dark {
		t.Fatal("theme still dark after a light background report")
	}
	dump(t, "session-light", m.View().Content)
}

// Every screen fits the terminal exactly, from cramped to huge: never a
// line too wide (the terminal would wrap it and shear the frame) and never
// the wrong number of rows.
func TestFramesFitEverySize(t *testing.T) {
	sizes := [][2]int{{50, 16}, {60, 20}, {80, 24}, {100, 30}, {120, 50}, {200, 60}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		check := func(name, frame string) {
			t.Helper()
			lines := strings.Split(frame, "\n")
			if len(lines) != h {
				t.Errorf("%dx%d %s: %d rows", w, h, name, len(lines))
			}
			for i, l := range lines {
				if lw := len([]rune(plain(l))); lw > w {
					t.Errorf("%dx%d %s: row %d is %d wide", w, h, name, i, lw)
					break
				}
			}
		}
		m := testModel(t, w, h)
		home, _ := m.renderHome(0)
		check("home", home)
		m.phase = phaseSplash
		m.splash.frame = 20
		intro, _ := m.renderHome(0)
		check("intro", intro)

		s := session(t, w, h)
		check("session", s.View().Content)
		s.input.SetValue("/")
		s.layout()
		check("palette", s.View().Content)
		s.input.SetValue("")
		s.approval = &agent.ApprovalEvent{Name: "edit_file", Preview: "internal/very/long/path/to/some/file_that_is_long.go", Kind: tools.KindWrite, Reply: make(chan agent.Decision, 1)}
		s.layout()
		check("approval", s.View().Content)
		if w == 80 {
			dump(t, "home-80", home)
			dump(t, "session-80", s.View().Content)
		}
	}
}

// The terminal cursor sits where the next character goes: on the first
// cell of the placeholder when the box is empty, after the text once
// something is typed. Both the home screen and the session draw the box.
func TestCursorSitsInTheText(t *testing.T) {
	col := func(frame string, row int, s string) int {
		line := []rune(plain(strings.Split(frame, "\n")[row]))
		for i := range line {
			if strings.HasPrefix(string(line[i:]), s) {
				return i
			}
		}
		return -1
	}
	m := testModel(t, 100, 40)
	frame, cur := m.renderHome(0)
	if want := col(frame, cur.Position.Y, "Ask caveira"); cur.Position.X != want {
		t.Fatalf("home: cursor at column %d, placeholder starts at %d", cur.Position.X, want)
	}
	m.input.SetValue("hello")
	frame, cur = m.renderHome(0)
	if want := col(frame, cur.Position.Y, "hello") + 5; cur.Position.X != want {
		t.Fatalf("home: cursor at column %d after typing, want %d", cur.Position.X, want)
	}

	s := session(t, 100, 40)
	v := s.View()
	if want := col(v.Content, v.Cursor.Position.Y, "Ask caveira"); v.Cursor.Position.X != want {
		t.Fatalf("session: cursor at column %d, placeholder starts at %d", v.Cursor.Position.X, want)
	}
}

// Your messages are bubbles against the right edge: quarter-cell corners
// on half-row edges, never wider than the line, and a plain rounded outline
// where the terminal cannot shade.
func TestUserMessageIsABubbleOnTheRight(t *testing.T) {
	long := strings.Repeat("make the retry logic back off exponentially ", 6)
	for _, text := range []string{"hi", long, "two\nlines"} {
		out := bubble(text, 90, th.bubble, th.text, th.muted)
		lines := strings.Split(plain(out), "\n")
		if !strings.Contains(lines[0], "▗") || !strings.Contains(lines[len(lines)-1], "▝") {
			t.Fatalf("%q: no rounded edges:\n%s", text, plain(out))
		}
		for _, l := range lines {
			if w := len([]rune(l)); w > 90 {
				t.Fatalf("%q: bubble row %d wide in 90", text, w)
			}
		}
		// Right-aligned: the body ends two cells of padding plus the tail
		// column from the edge, and short messages start far to the right.
		if text == "hi" && strings.Index(lines[1], "hi") < 80 {
			t.Fatalf("short bubble not against the right edge: %q", lines[1])
		}
		if text == long && len([]rune(strings.TrimLeft(lines[1], " "))) > 90*3/4 {
			t.Fatal("long bubble wider than three quarters of the line")
		}
	}
	plainBox := bubble("hi", 90, nil, th.text, th.muted)
	if !strings.Contains(plainBox, "╭") {
		t.Fatal("no outline fallback without a fill colour")
	}
}

// A --dev session says so everywhere the model is named, so a local model
// is never mistaken for the API.
func TestDevSessionIsBadged(t *testing.T) {
	m := testModel(t, 110, 40)
	m.dev = true
	home, _ := m.renderHome(0)
	if !strings.Contains(plain(home), "DEV abliterated-model") {
		t.Fatal("home info row missing the DEV badge")
	}
	s := session(t, 110, 60)
	s.dev = true
	s.items = s.items[1:] // drop the card built without dev
	s.pushHeader()
	s.layout()
	frame := s.View().Content
	dump(t, "session-dev", frame)
	p := plain(frame)
	if !strings.Contains(p, "DEV local model") || !strings.Contains(p, "DEV abliterated-model") {
		t.Fatal("session card or footer missing the DEV badge")
	}
}
