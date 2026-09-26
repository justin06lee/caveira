package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// copyTarget is a copy button on screen: a row of the transcript and the
// columns it covers, and the block it belongs to.
type copyTarget struct {
	line, x0, x1 int
	it           *item
	block        int
}

type copiedDoneMsg struct {
	it    *item
	block int
}

// writeClipboard is the OS clipboard; tests swap it out.
var writeClipboard = clipboard.WriteAll

// click presses the copy button under the mouse, if there is one.
func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft || ms.Y < 0 || ms.Y >= m.vp.Height() {
		return nil
	}
	line := m.vp.YOffset() + ms.Y
	col := ms.X - sideMargin
	for _, t := range m.targets {
		// A cell of slack either side: the button is a small target.
		if t.line == line && col >= t.x0-1 && col <= t.x1 {
			return m.copyBlock(t.it, t.block)
		}
	}
	return nil
}

// copyBlock copies one code block and has its button say so for a moment.
func (m *Model) copyBlock(it *item, block int) tea.Cmd {
	if block >= len(it.spots) {
		return nil
	}
	it.copied = block + 1
	it.invalidate()
	m.dirty = true
	return tea.Batch(
		copyText(it.spots[block].code),
		tea.Tick(2*time.Second, func(time.Time) tea.Msg { return copiedDoneMsg{it, block} }),
	)
}

func (m *Model) copiedDone(msg copiedDoneMsg) {
	if msg.it.copied == msg.block+1 {
		msg.it.copied = 0
		msg.it.invalidate()
		m.dirty = true
	}
}

// copyLast is /copy: the last code block of the latest reply, or the
// whole reply when it has none.
func (m *Model) copyLast() tea.Cmd {
	for i := len(m.items) - 1; i >= 0; i-- {
		it := m.items[i]
		if it.kind != itemAssistant || strings.TrimSpace(it.text) == "" {
			continue
		}
		m.rend.render(it) // places the blocks if the reply has not been drawn yet
		if n := len(it.spots); n > 0 {
			code := it.spots[n-1].code
			m.push(&item{kind: itemNotice, text: "copied the last code block · " + plural(strings.Count(code, "\n")+1, "line")})
			return m.copyBlock(it, n-1)
		}
		m.push(&item{kind: itemNotice, text: "copied the last reply · " + plural(len(strings.Fields(it.text)), "word")})
		return copyText(strings.TrimSpace(it.text))
	}
	m.push(&item{kind: itemNotice, text: "nothing to copy yet"})
	return nil
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// copyText puts text on the clipboard. On your own machine that is the
// OS clipboard; over SSH, or where the OS has no clipboard tool, it asks
// the terminal to do it (OSC 52), so the text lands where you are sitting.
func copyText(s string) tea.Cmd {
	return func() tea.Msg {
		local := os.Getenv("SSH_TTY") == "" && os.Getenv("SSH_CONNECTION") == ""
		if local && writeClipboard(s) == nil {
			return nil
		}
		return tea.SetClipboard(s)()
	}
}
