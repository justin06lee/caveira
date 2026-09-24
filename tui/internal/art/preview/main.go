// Command preview prints the art to stdout, for eyeballing the renderer
// outside the TUI: go run ./internal/art/preview -scale 2
package main

import (
	"flag"
	"fmt"

	"github.com/justin06lee/caveira/tui/internal/art"
)

func main() {
	scale := flag.Int("scale", 2, "mascot scale: 1 is 11 cells wide and 6 rows tall")
	flag.Parse()

	styles := []struct {
		name  string
		style art.MascotStyle
	}{
		{"colour", art.MascotColor},
		{"mono, dark background", art.MascotMonoDark},
		{"mono, light background", art.MascotMonoLight},
	}
	for _, s := range styles {
		fmt.Println(s.name)
		for _, l := range art.Mascot(*scale, art.MascotOptions{Style: s.style}) {
			fmt.Println("  " + l)
		}
		fmt.Println()
	}
}
