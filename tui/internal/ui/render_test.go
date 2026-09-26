package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/justin06lee/caveira/tui/internal/tools"
)

const richReply = "# Plan\n\nThree steps.\n\n## Build\n\n1. Write the loop:\n\n   ```go\n   for i := 0; i < 3; i++ {\n   \tfmt.Println(i, \"a line long enough that it has to wrap inside the panel instead of running off the side\")\n   }\n   ```\n\n2. Run it.\n\n### Notes\n\n> quoted\n\n```\nplain text block\n```\n\nDone with `go run .`"

// fg is the SGR a colour sets as foreground, as lipgloss writes it.
func fg(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestSplitFences(t *testing.T) {
	segs := splitFences("intro\n\n```go\nx := 1\n```\n\n   ~~~\n   indented\n   ~~~\nsee ```inline``` here\n```py\nstill streaming")
	var kinds []string
	for _, s := range segs {
		if s.code {
			kinds = append(kinds, "code:"+s.lang+":"+s.text)
		} else {
			kinds = append(kinds, "prose:"+strings.TrimSpace(s.text))
		}
	}
	want := []string{"prose:intro", "code:go:x := 1", "code::indented", "prose:see ```inline``` here", "code:py:still streaming"}
	if strings.Join(kinds, "|") != strings.Join(want, "|") {
		t.Fatalf("segments\n got %q\nwant %q", kinds, want)
	}
	if segs[2].indent != 3 {
		t.Fatalf("indented fence recorded at %d", segs[2].indent)
	}
}

// Code blocks are panels of their own: a header strip with the language
// and a copy button, the code highlighted and wrapped inside, every row
// the same width, edges in half cells with quarter-cell corners.
func TestCodeBlockIsAPanelWithACopyButton(t *testing.T) {
	m := session(t, 100, 60)
	it := &item{kind: itemAssistant, text: richReply}
	m.push(it)
	m.layout()
	frame := m.View().Content
	dump(t, "markdown", frame)

	out := m.rend.render(it)
	lines := strings.Split(out, "\n")
	p := strings.Split(plain(out), "\n")
	if len(it.spots) != 2 {
		t.Fatalf("want 2 copy buttons, got %d", len(it.spots))
	}
	for i, sp := range it.spots {
		row := []rune(p[sp.row])
		if got := string(row[sp.x0:sp.x1]); got != copyLabel {
			t.Fatalf("block %d: button at row %d cols %d-%d reads %q\n%s", i, sp.row, sp.x0, sp.x1, got, plain(out))
		}
	}
	if !strings.Contains(p[it.spots[0].row], "go") || !strings.Contains(p[it.spots[1].row], "code") {
		t.Fatal("header strips should name the language, or say code")
	}
	if !strings.Contains(it.spots[0].code, "\tfmt.Println(i,") || strings.Contains(it.spots[0].code, "```") {
		t.Fatalf("block copies %q", it.spots[0].code)
	}
	// The panel: ▄ edge above the header, ▀ below the body, rows between
	// all as wide as each other, and the long line wrapped, not cut.
	first := it.spots[0].row
	top, bottom := p[first-1], ""
	for _, l := range p[first+1:] {
		if strings.Contains(l, "▀") && !strings.ContainsAny(strings.Trim(l, " ▀"), "abcdefghijklmnopqrstuvwxyz") && l != p[first+1] {
			bottom = l
			break
		}
	}
	if !strings.Contains(top, "▄") || bottom == "" {
		t.Fatalf("no half-cell edges around the block:\n%s", plain(out))
	}
	if !strings.Contains(top, "▗") || !strings.Contains(bottom, "▝") {
		t.Fatalf("panel corners not rounded:\n%s", plain(out))
	}
	if !strings.Contains(plain(out), "has to wrap") || !strings.Contains(plain(out), "running off the side") {
		t.Fatalf("long line cut instead of wrapped:\n%s", plain(out))
	}
	for i := first; i < len(p) && p[i] != bottom; i++ {
		if w := lipgloss.Width(lines[i]); w != lipgloss.Width(lines[first]) {
			t.Fatalf("panel row %d is %d wide, header %d:\n%s", i, w, lipgloss.Width(lines[first]), plain(out))
		}
	}
	// Highlighted: the keyword and the string get their own colours.
	syn := syntaxOf(th)
	if !strings.Contains(out, fg(syn.keyword)) || !strings.Contains(out, fg(syn.str)) {
		t.Fatal("code not highlighted")
	}
	// The indented block sits under its list item.
	if !strings.HasPrefix(p[first], "  "+strings.Repeat(" ", 3)) {
		t.Fatalf("list block not indented: %q", p[first])
	}
}

// Headings are ranked by colour, since a terminal cannot size them.
func TestHeadingsAreRankedByColour(t *testing.T) {
	m := session(t, 100, 40)
	out := m.rend.render(&item{kind: itemAssistant, text: "# One\n\n## Two\n\n### Three\n\n#### Four"})
	lines := strings.Split(out, "\n")
	find := func(word string) string {
		for _, l := range lines {
			if strings.Contains(plain(l), word) {
				return l
			}
		}
		t.Fatalf("heading %q missing:\n%s", word, plain(out))
		return ""
	}
	for word, c := range map[string]string{"One": fg(th.h1), "Two": fg(th.h2), "Three": fg(th.h3), "Four": fg(th.text)} {
		if l := find(word); !strings.Contains(l, c) {
			t.Errorf("heading %q not drawn in its colour %s: %q", word, c, l)
		}
	}
	if strings.Contains(plain(out), "#") {
		t.Fatal("heading markers left in")
	}
}

// Clicking copy puts the block on the clipboard and the button says so
// for a moment; /copy does the same for the last block by keyboard.
func TestClickingCopyCopiesTheBlock(t *testing.T) {
	var got []string
	orig := writeClipboard
	writeClipboard = func(s string) error { got = append(got, s); return nil }
	t.Cleanup(func() { writeClipboard = orig })
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")

	m := session(t, 100, 60)
	it := &item{kind: itemAssistant, text: richReply}
	m.push(it)
	m.layout()
	m.View()
	if len(m.targets) < 2 {
		t.Fatalf("want copy targets for both blocks, got %d", len(m.targets))
	}
	tgt := m.targets[len(m.targets)-2] // the Go block
	y := tgt.line - m.vp.YOffset()
	if y < 0 || y >= m.vp.Height() {
		t.Fatalf("copy button off screen at line %d (offset %d)", tgt.line, m.vp.YOffset())
	}

	// A click beside the button does nothing.
	if cmd := m.click(tea.Mouse{X: sideMargin + tgt.x0 - 6, Y: y, Button: tea.MouseLeft}); cmd != nil {
		t.Fatal("a click beside the button copied")
	}
	_, cmd := m.Update(tea.MouseClickMsg{X: sideMargin + tgt.x0 + 1, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clicking copy did nothing")
	}
	runCmds(cmd)
	if len(got) != 1 || !strings.HasPrefix(got[0], "for i := 0") {
		t.Fatalf("clipboard got %q", got)
	}
	frame := plain(m.View().Content)
	dump(t, "markdown-copied", m.View().Content)
	if !strings.Contains(frame, copiedLabel) {
		t.Fatal("button does not say copied")
	}
	m.Update(copiedDoneMsg{it: it, block: 0})
	if strings.Contains(plain(m.View().Content), copiedLabel) {
		t.Fatal("copied stays after the moment passes")
	}

	m.input.SetValue("/copy")
	runCmds(m.slash("/copy"))
	if len(got) != 2 || got[1] != "plain text block" {
		t.Fatalf("/copy copied %q", got)
	}
}

// runCmds runs a command and any batch it holds, ignoring timers.
func runCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				runCmds(c)
			}
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// A tool call shimmers while it runs, then settles into darker gray; only
// a failure keeps a colour.
func TestToolCallsAreGrayAndShimmerWhileRunning(t *testing.T) {
	m := session(t, 100, 40)
	running := &item{kind: itemTool, toolName: "bash", preview: "go test ./...", running: true, started: time.Now()}
	a := m.rend.render(running)
	m.rend.frame += 3
	running.invalidate()
	b := m.rend.render(running)
	if a == b {
		t.Fatal("a running call should shimmer from frame to frame")
	}
	call := func(s string) string { return string([]rune(plain(s))[1:]) } // past the spinner
	if call(a) != call(b) || !strings.Contains(plain(a), "Bash $ go test ./...") {
		t.Fatalf("running call reads %q", plain(a))
	}
	if strings.Contains(a, fg(th.ok)) || strings.Contains(a, fg(th.accent)) {
		t.Fatal("running call should be gray")
	}

	done := &item{kind: itemTool, toolName: "bash", preview: "go test ./...", result: &tools.Result{Summary: "exit 0 · 1 line"}}
	d := strings.Split(m.rend.render(done), "\n")[0]
	if !strings.Contains(d, fg(th.faint)) || strings.Contains(d, fg(th.ok)) || strings.Contains(d, fg(th.text)) {
		t.Fatalf("finished call should be darker gray: %q", d)
	}
	failed := &item{kind: itemTool, toolName: "bash", preview: "go test ./...", result: &tools.Result{Summary: "exit 1", IsError: true}}
	if !strings.Contains(m.rend.render(failed), fg(th.err)) {
		t.Fatal("a failed call should say so in red")
	}
}

// Terminals without shading get the block as a rounded outline, the copy
// button set in its top edge.
func TestCodeBlockOutlineWithoutShading(t *testing.T) {
	m := session(t, 90, 40)
	th = newTheme(true, colorprofile.ANSI, nil)
	m.rend.md = newMarkdown(m.inner() - 2)
	it := &item{kind: itemAssistant, text: "```sh\necho hi\n```"}
	out := plain(m.rend.render(it))
	lines := strings.Split(out, "\n")
	if len(it.spots) != 1 || !strings.Contains(lines[0], "╭─ sh") || !strings.Contains(lines[0], "copy ─╮") || !strings.Contains(out, "│ echo hi") {
		t.Fatalf("outline block:\n%s", out)
	}
	if got := string([]rune(lines[0])[it.spots[0].x0:it.spots[0].x1]); got != copyLabel {
		t.Fatalf("button spot reads %q", got)
	}
}
