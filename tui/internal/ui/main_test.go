package ui

import (
	"os"
	"testing"
)

// TestMain gives the whole run a home of its own. testModel sets HOME per
// test, but a turn a test starts can still be saving after the test has
// returned and HOME is the real one again; it lands here instead.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "caveira-ui-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
