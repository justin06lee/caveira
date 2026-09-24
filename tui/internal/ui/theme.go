package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/justin06lee/caveira/tui/internal/art"
)

// theme is every colour the session draws with. The palette comes from the
// mascot: bone on black, with one ember accent for the things that are
// caveira's own (the prompt mark, the spinner) and plain signal colours for
// success, warnings, and errors. It is rebuilt when the terminal reports its
// background, so light terminals get a light palette, and when it reports
// its colour profile, so terminals without 256 colours drop the shaded
// panels and get the one-colour mascot.
type theme struct {
	dark    bool
	profile colorprofile.Profile
	mascot  art.MascotStyle

	text, muted, faint, line   color.Color
	bone, accent, code         color.Color
	ok, warn, err, info        color.Color
	surface, bubble            color.Color // nil where shading would not survive
	addBg, delBg               color.Color
	addFg, delFg, accentShadow color.Color

	Text, Muted, Faint, Line  lipgloss.Style
	Bone, Accent, Code        lipgloss.Style
	OK, Warn, Err, Info       lipgloss.Style
	Title, Key, Thinking, Dim lipgloss.Style
}

// th is the theme in use. The model swaps it when the terminal answers.
var th = newTheme(true, colorprofile.TrueColor, nil)

func hex(s string) color.Color { return lipgloss.Color(s) }

func newTheme(dark bool, profile colorprofile.Profile, bg color.Color) *theme {
	t := &theme{dark: dark, profile: profile}
	if dark {
		if bg == nil {
			bg = hex("#161719")
		}
		t.text = hex("#E4E2D8")
		t.muted = hex("#9B9D93")
		t.faint = hex("#65675F")
		t.line = hex("#3A3C37")
		t.bone = hex("#C9CCB9")
		t.accent = hex("#E5583E")
		t.accentShadow = hex("#8A3A2B")
		t.code = hex("#D9B574")
		t.ok = hex("#93B874")
		t.warn = hex("#DDAA52")
		t.err = hex("#EF6B61")
		t.info = hex("#86A3C3")
		t.addFg = hex("#B5DA9B")
		t.delFg = hex("#F0A097")
		t.surface = mix(bg, t.text, 0.07)
		t.bubble = mix(mix(bg, t.text, 0.10), t.accent, 0.10)
		t.addBg = mix(bg, hex("#3FAF5A"), 0.17)
		t.delBg = mix(bg, hex("#E0564B"), 0.17)
	} else {
		if bg == nil {
			bg = hex("#FAFAF7")
		}
		t.text = hex("#262722")
		t.muted = hex("#66685F")
		t.faint = hex("#9D9F96")
		t.line = hex("#D2D3CB")
		t.bone = hex("#4F5243")
		t.accent = hex("#C9452C")
		t.accentShadow = hex("#E6B1A5")
		t.code = hex("#8A5A12")
		t.ok = hex("#4A7D2C")
		t.warn = hex("#99680F")
		t.err = hex("#C33A34")
		t.info = hex("#3B6690")
		t.addFg = hex("#2F6B1F")
		t.delFg = hex("#A8322A")
		t.surface = mix(bg, t.text, 0.055)
		t.bubble = mix(mix(bg, t.text, 0.05), t.accent, 0.09)
		t.addBg = mix(bg, hex("#3FAF5A"), 0.15)
		t.delBg = mix(bg, hex("#E0564B"), 0.15)
	}

	t.mascot = art.MascotColor
	if profile < colorprofile.ANSI256 {
		// Sixteen colours cannot shade a panel without shouting, and the
		// painted skull would round to white; draw both in one colour.
		t.surface, t.bubble, t.addBg, t.delBg = nil, nil, nil, nil
		t.mascot = art.MascotMonoDark
		if !dark {
			t.mascot = art.MascotMonoLight
		}
	}

	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	t.Text = fg(t.text)
	t.Muted = fg(t.muted)
	t.Faint = fg(t.faint)
	t.Line = fg(t.line)
	t.Bone = fg(t.bone)
	t.Accent = fg(t.accent)
	t.Code = fg(t.code)
	t.OK = fg(t.ok)
	t.Warn = fg(t.warn)
	t.Err = fg(t.err)
	t.Info = fg(t.info)
	t.Title = fg(t.text).Bold(true)
	t.Key = fg(t.text)
	t.Thinking = fg(t.muted).Italic(true)
	t.Dim = fg(t.faint).Italic(true)
	return t
}

// mix blends a toward b by f (0 is a, 1 is b).
func mix(a, b color.Color, f float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	l := func(x, y uint32) uint8 {
		return uint8((float64(x>>8)*(1-f) + float64(y>>8)*f) + 0.5)
	}
	return color.RGBA{R: l(ar, br), G: l(ag, bg), B: l(ab, bb), A: 0xff}
}

// hexOf formats a colour for glamour, which takes strings.
func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	const digits = "0123456789ABCDEF"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint32{r >> 8, g >> 8, b >> 8} {
		out[1+2*i] = digits[v>>4]
		out[2+2*i] = digits[v&0xF]
	}
	return string(out)
}
