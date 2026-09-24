// Package art renders caveira's pictures as terminal cells: the pixel skull
// mascot and the banner's pixel lettering, both as half-block characters so
// each cell holds two pixels stacked vertically. In colour, every cell
// carries a true-colour foreground and background; in mono, the half-blocks
// alone draw the shape in the terminal's own foreground colour.
package art

import (
	"fmt"
	"math"
	"strings"
)

// Pixel is one straight-alpha colour sample.
type Pixel struct {
	R, G, B float64 // 0..1
	A       float64 // 0..1
}

// Bitmap is a grid of pixels ready to be drawn.
type Bitmap struct {
	W, H int
	Pix  []Pixel
}

func (b *Bitmap) At(x, y int) Pixel {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return Pixel{}
	}
	return b.Pix[y*b.W+x]
}

// Options shape how a bitmap becomes cells.
type Options struct {
	// Brightness scales every colour; 1 (or 0) is as drawn.
	Brightness float64
	// Mono ignores colour: a pixel is either on, drawn in the terminal's
	// foreground, or off. Nothing but half-blocks and spaces is written.
	Mono bool
}

// Render draws a bitmap as rows of half-block cells. The result has
// ceil(H/2) lines, each W cells wide, with an SGR reset after every
// coloured cell. Transparent cells are spaces with no colour, so they show
// whatever is behind them.
func Render(b *Bitmap, o Options) []string {
	if o.Brightness == 0 {
		o.Brightness = 1
	}
	rows := (b.H + 1) / 2
	out := make([]string, rows)
	var sb strings.Builder
	for row := 0; row < rows; row++ {
		sb.Reset()
		for x := 0; x < b.W; x++ {
			top := b.At(x, row*2)
			bot := b.At(x, row*2+1)
			topOn := top.A > 0.35
			botOn := bot.A > 0.35
			if o.Mono {
				switch {
				case topOn && botOn:
					sb.WriteString("█")
				case topOn:
					sb.WriteString("▀")
				case botOn:
					sb.WriteString("▄")
				default:
					sb.WriteByte(' ')
				}
				continue
			}
			s := o.Brightness
			switch {
			case topOn && botOn:
				sb.WriteString(fg(top, s))
				sb.WriteString(bg(bot, s))
				sb.WriteString("▀\x1b[0m")
			case topOn:
				sb.WriteString(fg(top, s))
				sb.WriteString("▀\x1b[0m")
			case botOn:
				sb.WriteString(fg(bot, s))
				sb.WriteString("▄\x1b[0m")
			default:
				sb.WriteByte(' ')
			}
		}
		out[row] = sb.String()
	}
	return out
}

func clamp8(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 255
	}
	return int(math.Round(v * 255))
}

func fg(p Pixel, s float64) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", clamp8(p.R*s), clamp8(p.G*s), clamp8(p.B*s))
}

func bg(p Pixel, s float64) string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", clamp8(p.R*s), clamp8(p.G*s), clamp8(p.B*s))
}

// hash is a cheap, stable 0..1 value per cell.
func hash(x, y int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h&0xffff) / 0xffff
}

// Wordmark draws the banner's "caveira" lettering at an integer scale in
// the given colour, or in the terminal's foreground when o.Mono is set.
// Scale 1 is 4 rows tall.
func Wordmark(scale int, r, g, b uint8, o Options) []string {
	scale = max(scale, 1)
	w := len(wordmark[0]) * scale
	h := len(wordmark) * scale
	bm := &Bitmap{W: w, H: h, Pix: make([]Pixel, w*h)}
	p := rgb(r, g, b)
	for y := 0; y < h; y++ {
		src := wordmark[y/scale]
		for x := 0; x < w; x++ {
			if src[x/scale] != ' ' {
				bm.Pix[y*w+x] = p
			}
		}
	}
	return Render(bm, o)
}

// WordmarkWidth is the width in cells at a scale.
func WordmarkWidth(scale int) int { return len(wordmark[0]) * max(scale, 1) }
