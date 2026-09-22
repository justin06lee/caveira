// Package art renders caveira's pictures as terminal cells: the painted
// skull from a PNG, and the banner's pixel lettering, both as half-block
// characters with true-colour foreground and background, so each cell holds
// two pixels stacked vertically.
package art

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"sync"
)

//go:embed skull.png
var skullPNG []byte

var (
	skullOnce sync.Once
	skullImg  *image.NRGBA
	skullBox  image.Rectangle // the square around the visible skull
)

// source decodes the embedded skull once.
func source() *image.NRGBA {
	skullOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(skullPNG))
		if err != nil {
			skullImg = image.NewNRGBA(image.Rect(0, 0, 1, 1))
			return
		}
		b := img.Bounds()
		out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				r, g, bb, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				i := out.PixOffset(x, y)
				if a == 0 {
					continue
				}
				// Un-premultiply to straight alpha.
				out.Pix[i+0] = uint8(r * 0xffff / a >> 8)
				out.Pix[i+1] = uint8(g * 0xffff / a >> 8)
				out.Pix[i+2] = uint8(bb * 0xffff / a >> 8)
				out.Pix[i+3] = uint8(a >> 8)
			}
		}
		skullImg = out
		skullBox = visibleSquare(out)
	})
	return skullImg
}

// visibleSquare is the smallest square, centred on the opaque part of the
// image, that contains all of it; the empty margins of the PNG would
// otherwise waste rows on screen.
func visibleSquare(img *image.NRGBA) image.Rectangle {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			if img.Pix[i+3] < 90 {
				continue
			}
			lum := 0.299*float64(img.Pix[i]) + 0.587*float64(img.Pix[i+1]) + 0.114*float64(img.Pix[i+2])
			if lum < 0.09*255 {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x+1), max(maxY, y+1)
		}
	}
	if minX >= maxX || minY >= maxY {
		return b
	}
	side := max(maxX-minX, maxY-minY)
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	r := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	// Slide back inside the image if the square pokes out.
	if r.Min.X < b.Min.X {
		r = r.Add(image.Pt(b.Min.X-r.Min.X, 0))
	}
	if r.Min.Y < b.Min.Y {
		r = r.Add(image.Pt(0, b.Min.Y-r.Min.Y))
	}
	if r.Max.X > b.Max.X {
		r = r.Add(image.Pt(b.Max.X-r.Max.X, 0))
	}
	if r.Max.Y > b.Max.Y {
		r = r.Add(image.Pt(0, b.Max.Y-r.Max.Y))
	}
	return r.Intersect(b)
}

// Pixel is one straight-alpha colour sample.
type Pixel struct {
	R, G, B float64 // 0..1
	A       float64 // 0..1
}

// Bitmap is a square of pixels ready to be drawn.
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

// Skull resamples the painted skull to px pixels on a side with a box
// filter. Near-black pixels count as background, so the image floats on
// whatever the terminal's background is.
func Skull(px int) *Bitmap {
	if px < 2 {
		px = 2
	}
	src := source()
	box := skullBox
	sw, sh := box.Dx(), box.Dy()
	out := &Bitmap{W: px, H: px, Pix: make([]Pixel, px*px)}
	for y := 0; y < px; y++ {
		y0 := box.Min.Y + y*sh/px
		y1 := max(box.Min.Y+(y+1)*sh/px, y0+1)
		for x := 0; x < px; x++ {
			x0 := box.Min.X + x*sw/px
			x1 := max(box.Min.X+(x+1)*sw/px, x0+1)
			var r, g, b, a float64
			n := 0
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					i := src.PixOffset(sx, sy)
					pa := float64(src.Pix[i+3]) / 255
					r += float64(src.Pix[i+0]) / 255 * pa
					g += float64(src.Pix[i+1]) / 255 * pa
					b += float64(src.Pix[i+2]) / 255 * pa
					a += pa
					n++
				}
			}
			if a > 0 {
				r, g, b = r/a, g/a, b/a
			}
			a /= float64(n)
			// The source sits on black; treat dark fringe as transparent so
			// the silhouette stays crisp on any background.
			lum := 0.299*r + 0.587*g + 0.114*b
			if lum < 0.045 {
				a = 0
			} else if lum < 0.09 {
				a *= (lum - 0.045) / 0.045
			}
			out.Pix[y*px+x] = Pixel{R: r, G: g, B: b, A: a}
		}
	}
	return out
}

// Options shape how a bitmap becomes cells.
type Options struct {
	// Brightness scales every colour; 1 is as drawn, 0 is black.
	Brightness float64
	// Scanlines darkens the lower pixel of every cell a little, the way a
	// CRT's beam leaves a gap between lines.
	Scanlines bool
	// Grain adds a faint per-cell brightness wobble.
	Grain bool
	// RowBrightness, when set, scales one row's brightness on top of the
	// global one; the splash uses it for the beam that sweeps down.
	RowBrightness func(row int) float64
}

// Render draws a bitmap as rows of half-block cells. The result has
// ceil(H/2) lines, each W cells wide, with an SGR reset at the end of each
// line. Transparent cells are spaces with no colour, so they show whatever
// is behind them.
func Render(b *Bitmap, o Options) []string {
	if o.Brightness == 0 {
		o.Brightness = 1
	}
	rows := (b.H + 1) / 2
	out := make([]string, rows)
	var sb strings.Builder
	for row := 0; row < rows; row++ {
		sb.Reset()
		scale := o.Brightness
		if o.RowBrightness != nil {
			scale *= o.RowBrightness(row)
		}
		for x := 0; x < b.W; x++ {
			top := b.At(x, row*2)
			bot := b.At(x, row*2+1)
			g := 1.0
			if o.Grain {
				g = 0.94 + 0.12*hash(x, row)
			}
			ts := scale * g
			bs := scale * g
			if o.Scanlines {
				bs *= 0.86
			}
			topOn := top.A > 0.35
			botOn := bot.A > 0.35
			switch {
			case topOn && botOn:
				sb.WriteString(fg(top, ts))
				sb.WriteString(bg(bot, bs))
				sb.WriteString("▀")
				sb.WriteString("\x1b[0m")
			case topOn:
				sb.WriteString(fg(top, ts))
				sb.WriteString("▀\x1b[0m")
			case botOn:
				sb.WriteString(fg(bot, bs))
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

// hash is a cheap, stable 0..1 value per cell for grain.
func hash(x, y int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h&0xffff) / 0xffff
}

// Wordmark draws the banner's "caveira" lettering at an integer scale in
// the given colour. Scale 1 is 3 rows tall; scale 2 is 5.
func Wordmark(scale int, r, g, b uint8, o Options) []string {
	return glyph(wordmark, scale, o, func(byte) Pixel {
		return Pixel{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255, A: 1}
	})
}

// WordmarkWidth is the width in cells at a scale.
func WordmarkWidth(scale int) int { return len(wordmark[0]) * max(scale, 1) }

// PixelSkull draws the banner's sword-through-skull mark at a scale.
func PixelSkull(scale int) []string {
	return glyph(pixelSkull, scale, Options{}, func(c byte) Pixel {
		rgb, ok := pixelPalette[c]
		if !ok {
			return Pixel{}
		}
		return Pixel{R: float64(rgb[0]) / 255, G: float64(rgb[1]) / 255, B: float64(rgb[2]) / 255, A: 1}
	})
}

func glyph(rows []string, scale int, o Options, colour func(byte) Pixel) []string {
	if scale < 1 {
		scale = 1
	}
	w := len(rows[0]) * scale
	h := len(rows) * scale
	bm := &Bitmap{W: w, H: h, Pix: make([]Pixel, w*h)}
	for y := 0; y < h; y++ {
		src := rows[y/scale]
		for x := 0; x < w; x++ {
			c := src[x/scale]
			if c == ' ' {
				continue
			}
			bm.Pix[y*w+x] = colour(c)
		}
	}
	return Render(bm, o)
}
