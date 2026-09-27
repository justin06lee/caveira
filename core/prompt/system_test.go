package prompt

import (
	"strings"
	"testing"
)

// The desktop prompt must not tell the model it is in a terminal, and
// must otherwise be the same prompt.
func TestDesktopPromptSaysWhereTheUserIs(t *testing.T) {
	dir := t.TempDir()
	term := Build(Options{WorkDir: dir})
	desk := Build(Options{WorkDir: dir, Desktop: true})
	if strings.Contains(strings.ToLower(desk), "terminal") {
		t.Fatal("desktop prompt still mentions a terminal")
	}
	if !strings.Contains(desk, "caveira desktop app") || !strings.Contains(desk, "reading a chat window") {
		t.Fatal("desktop prompt does not say where the user is")
	}
	if !strings.Contains(term, "running in the user's terminal") {
		t.Fatal("terminal prompt changed")
	}
	if len(desk)-len(term) > 200 || len(term)-len(desk) > 200 {
		t.Fatalf("prompts differ by more than the two phrases: %d vs %d bytes", len(term), len(desk))
	}
}
