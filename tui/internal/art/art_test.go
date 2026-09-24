package art

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMascotSizes(t *testing.T) {
	for scale := 1; scale <= 3; scale++ {
		for _, style := range []MascotStyle{MascotColor, MascotMonoDark, MascotMonoLight} {
			lines := Mascot(scale, MascotOptions{Style: style})
			if len(lines) != MascotHeight(scale) {
				t.Fatalf("scale %d style %d: %d rows, want %d", scale, style, len(lines), MascotHeight(scale))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != MascotWidth(scale) {
					t.Fatalf("scale %d style %d row %d: %d wide, want %d", scale, style, i, w, MascotWidth(scale))
				}
			}
		}
	}
}

func TestMonoMascotWritesNoColour(t *testing.T) {
	for _, style := range []MascotStyle{MascotMonoDark, MascotMonoLight} {
		s := strings.Join(Mascot(2, MascotOptions{Style: style}), "\n")
		if strings.Contains(s, "\x1b") {
			t.Fatalf("style %d wrote escape codes", style)
		}
	}
	// The two mono styles are negatives of each other inside the outline.
	dark := strings.Join(Mascot(1, MascotOptions{Style: MascotMonoDark}), "")
	light := strings.Join(Mascot(1, MascotOptions{Style: MascotMonoLight}), "")
	if dark == light {
		t.Fatal("mono styles drew the same picture")
	}
}

func TestMascotDissolve(t *testing.T) {
	count := func(hidden float64) int {
		s := strings.Join(Mascot(2, MascotOptions{Style: MascotMonoDark, Hidden: hidden}), "")
		return strings.Count(s, "█") + strings.Count(s, "▀") + strings.Count(s, "▄")
	}
	full, half, none := count(0), count(0.5), count(1)
	if none != 0 || !(half > 0 && half < full) {
		t.Fatalf("dissolve not progressive: full %d half %d none %d", full, half, none)
	}
}
