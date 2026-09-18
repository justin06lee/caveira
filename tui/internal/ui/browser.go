package ui

import (
	"os/exec"
	"runtime"
)

// openBrowser is best-effort on purpose: every screen that calls it also
// prints the URL, so a headless machine or a locked-down desktop just means
// the user copies the link instead.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
