// Command preview prints the art to stdout, for eyeballing the renderer
// outside the TUI: go run ./internal/art/preview -px 64
package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/justin06lee/caveira/tui/internal/art"
)

func main() {
	px := flag.Int("px", 64, "skull size in pixels (cells wide; half as many rows)")
	scan := flag.Bool("scanlines", true, "darken lower half-pixels")
	grain := flag.Bool("grain", true, "per-cell brightness wobble")
	flag.Parse()

	for _, l := range art.Render(art.Skull(*px), art.Options{Scanlines: *scan, Grain: *grain}) {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range art.Wordmark(1, 0xD9, 0xD2, 0xC3, art.Options{}) {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range art.Wordmark(2, 0xD9, 0xD2, 0xC3, art.Options{}) {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range art.PixelSkull(1) {
		fmt.Println(l)
	}
	fmt.Println(strings.Repeat("─", 20))
}
