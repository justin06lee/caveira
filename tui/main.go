// Command caveira is the terminal client for caveira.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/ui"
)

func main() {
	auth, err := config.Load()
	if err != nil {
		// A corrupt token file should not be a wall: say so and sign in again.
		fmt.Fprintf(os.Stderr, "caveira: ignoring %s: %v\n", config.Path(), err)
		auth = nil
	}

	var token, email string
	if auth != nil {
		token, email = auth.Token, auth.Email
	}

	if _, err := tea.NewProgram(ui.New(token, email)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "caveira:", err)
		os.Exit(1)
	}
}
