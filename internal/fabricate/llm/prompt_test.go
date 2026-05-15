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
	big := strings.Repeat("x\n", 10000)
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
}
