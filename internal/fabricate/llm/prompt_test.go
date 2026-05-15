package llm

import (
	"strings"
	"testing"
)

func TestBuildPrompt_IncludesPathsAndSegmentMaps(t *testing.T) {
	files := []FileInput{
		{Path: "go.mod", Kind: "chore", Content: "module x\n",
			Segments: []SegmentInfo{{Index: 0, StartLine: 0, EndLine: 1}}},
		{Path: "internal/walk/load.go", Kind: "code", Content: "package walk\n",
			Segments: []SegmentInfo{
				{Index: 0, StartLine: 0, EndLine: 12},
				{Index: 1, StartLine: 12, EndLine: 30},
			}},
	}
	p := BuildPrompt(files, 1<<20)
	for _, want := range []string{"go.mod", "internal/walk/load.go", "2 segments", "JSON", "commits"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestBuildPrompt_BudgetTruncatesContent(t *testing.T) {
	big := strings.Repeat("x\n", 10000) // 20000 bytes
	files := []FileInput{
		{Path: "big.go", Kind: "code", Content: big,
			Segments: []SegmentInfo{{Index: 0, StartLine: 0, EndLine: 10000}}},
	}
	p := BuildPrompt(files, 200)
	if !strings.Contains(p, "truncated") {
		t.Fatal("expected oversized content to be marked truncated")
	}
	if !strings.Contains(p, "big.go") {
		t.Fatal("truncated file's path and segment map must still appear")
	}
	// The 200-byte budget must actually be enforced: the whole prompt is the
	// fixed instructions preamble (~1238 bytes) plus headers plus at most
	// budget bytes of content. A regression that ignored budget entirely would
	// emit the full 20000-byte input and blow well past this bound.
	if len(p) >= 4000 {
		t.Fatalf("budget not enforced: prompt is %d bytes, want < 4000", len(p))
	}
}

func TestBuildPrompt_PerFileCapTruncates(t *testing.T) {
	content := strings.Repeat("x\n", 8000) // 16000 bytes, over perFileContentCap
	files := []FileInput{
		{Path: "huge.go", Kind: "code", Content: content,
			Segments: []SegmentInfo{{Index: 0, StartLine: 0, EndLine: 8000}}},
	}
	// Large budget so truncation is driven by the per-file cap, not the budget.
	p := BuildPrompt(files, 1<<20)
	if !strings.Contains(p, "truncated") {
		t.Fatal("expected content over perFileContentCap to be marked truncated")
	}
	if len(p) >= len(content) {
		t.Fatalf("per-file cap not enforced: prompt is %d bytes, input was %d", len(p), len(content))
	}
}
