package art

// The mascot is an 11 by 11 pixel skull: a black outline around a bone
// face, lit from the left, with the shading stepping down the right side.
// Each character is one pixel; '.' is transparent.
var mascot = []string{
	"..#######..",
	".##aaaab##.",
	"##aaaaabc##",
	"#aaaaabbcd#",
	"#aaaabbbcd#",
	"###aab##bd#",
	"###a#b##bd#",
	"#aaaabbbcd#",
	"##aaaab####",
	".#a#a#b###.",
	".########..",
}

// mascotPalette is the colour mascot, sampled from the reference art.
var mascotPalette = map[byte]Pixel{
	'#': rgb(0x00, 0x00, 0x00),
	'a': rgb(0xBF, 0xC1, 0xB0),
	'b': rgb(0xB7, 0xB9, 0xA0),
	'c': rgb(0xA0, 0xA1, 0x97),
	'd': rgb(0x89, 0x8A, 0x83),
}

func rgb(r, g, b uint8) Pixel {
	return Pixel{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255, A: 1}
}

// MascotSize is the mascot's side in pixels at scale 1.
const MascotSize = 11

// MascotStyle picks how the mascot is drawn for what the terminal can show.
type MascotStyle int

const (
	// MascotColor is the painted skull, for 256-colour and true-colour
	// terminals.
	MascotColor MascotStyle = iota
	// MascotMonoDark draws the face in the terminal's own foreground and
	// leaves the outline to the background: the one-colour skull, for dark
	// terminals without colour.
	MascotMonoDark
	// MascotMonoLight draws the outline instead, which is the same picture
	// on a light background.
	MascotMonoLight
)

// MascotOptions shape one frame of the mascot.
type MascotOptions struct {
	Style MascotStyle
	// Hidden is the fraction of pixels not drawn yet, 0 to 1. Pixels come
	// and go in a fixed scattered order, so animating it dissolves the
	// skull in or out.
	Hidden float64
	// Flash brightens pixels that have just appeared, as a fraction of
	// their colour (colour style only).
	Flash float64
	// Brightness scales the colour; 0 means as drawn.
	Brightness float64
}

// MascotWidth is the mascot's width in cells at a scale.
func MascotWidth(scale int) int { return MascotSize * max(scale, 1) }

// MascotHeight is its height in rows at a scale: two pixels to a row.
func MascotHeight(scale int) int { return (MascotSize*max(scale, 1) + 1) / 2 }

// Mascot draws the skull at an integer scale as half-block rows. Every
// line is MascotWidth(scale) cells wide.
func Mascot(scale int, o MascotOptions) []string {
	scale = max(scale, 1)
	w := MascotSize * scale
	bm := &Bitmap{W: w, H: w, Pix: make([]Pixel, w*w)}
	shown := 1 - o.Hidden
	for gy, row := range mascot {
		for gx := 0; gx < len(row); gx++ {
			c := row[gx]
			if c == '.' {
				continue
			}
			// Each grid pixel appears when the reveal passes its threshold.
			t := hash(gx*7+3, gy*13+5)
			if o.Hidden > 0 && t >= shown {
				continue
			}
			var p Pixel
			switch o.Style {
			case MascotMonoDark:
				if c == '#' {
					continue
				}
				p = Pixel{A: 1}
			case MascotMonoLight:
				if c != '#' {
					continue
				}
				p = Pixel{A: 1}
			default:
				p = mascotPalette[c]
				if o.Flash > 0 && o.Hidden > 0 {
					// Newest pixels glow, then settle over the next stretch.
					if age := shown - t; age < 0.18 {
						k := 1 + o.Flash*(1-age/0.18)
						p = Pixel{R: min(p.R*k+0.08*(k-1), 1), G: min(p.G*k+0.08*(k-1), 1), B: min(p.B*k+0.08*(k-1), 1), A: 1}
					}
				}
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					bm.Pix[(gy*scale+sy)*w+gx*scale+sx] = p
				}
			}
		}
	}
	return Render(bm, Options{
		Brightness: o.Brightness,
		Mono:       o.Style != MascotColor,
	})
}
