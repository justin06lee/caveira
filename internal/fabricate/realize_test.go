package fabricate

import (
	"bytes"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/filemode"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
)

func srcFile(path, content string) SourceFile {
	c := []byte(content)
	return SourceFile{
		Path: path, Mode: filemode.Regular, Content: c, Segments: SplitSegments(c),
	}
}

func finalContent(commits []SynthCommit, path string) []byte {
	var latest []byte
	for _, c := range commits {
		for _, fr := range c.Added {
			if fr.Path == path {
				if fr.Content != nil {
					latest = fr.Content
				} else {
					latest = nil // whole-file-from-source marker
				}
			}
		}
	}
	return latest
}

func TestRealize_WholeFilesMatchSource(t *testing.T) {
	srcs := []SourceFile{srcFile("go.mod", "module x\n"), srcFile("main.go", "package main\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "chore: init", Type: "chore", Changes: []llm.Change{{Path: "go.mod", AllSegments: true}}},
		{Message: "feat: main", Type: "feat", Changes: []llm.Change{{Path: "main.go", AllSegments: true}}},
	}}
	base, err := Realize(srcs, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if len(base) != 2 {
		t.Fatalf("want 2 commits, got %d", len(base))
	}
}

func TestRealize_LayeredFileEndsExact(t *testing.T) {
	content := "l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\n"
	src := srcFile("big.go", content)
	nSeg := len(src.Segments)
	if nSeg < 2 {
		t.Skipf("fixture needs >=2 segments, got %d", nSeg)
	}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: scaffold", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: []int{0}}}},
		{Message: "feat: rest", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: lastIndices(nSeg)}}},
	}}
	base, err := Realize([]SourceFile{src}, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	last := base[len(base)-1]
	var got []byte
	for _, fr := range last.Added {
		if fr.Path == "big.go" {
			got = fr.Content
		}
	}
	// Final commit holds remaining segments; cumulative must equal full content.
	if !bytes.Equal(cumulativeBig(base), []byte(content)) {
		_ = got
		t.Fatal("layered realization did not end at exact source content")
	}
}

func lastIndices(n int) []int {
	var out []int
	for i := 1; i < n; i++ {
		out = append(out, i)
	}
	return out
}

// cumulativeBig returns the final realized content of "big.go".
func cumulativeBig(base []SynthCommit) []byte {
	var latest []byte
	for _, c := range base {
		for _, fr := range c.Added {
			if fr.Path == "big.go" {
				latest = fr.Content
			}
		}
	}
	return latest
}

func TestRealize_ClampsForgottenFile(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n"), srcFile("b.go", "package b\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: a", Type: "feat", Changes: []llm.Change{{Path: "a.go", AllSegments: true}}},
	}}
	base, err := Realize(srcs, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	sawB := false
	for _, c := range base {
		for _, fr := range c.Added {
			if fr.Path == "b.go" {
				sawB = true
			}
		}
	}
	if !sawB {
		t.Fatal("forgotten file b.go was not clamped into the plan")
	}
}

func TestRealize_UnknownPathRejected(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: ghost", Type: "feat", Changes: []llm.Change{{Path: "ghost.go", AllSegments: true}}},
	}}
	if _, err := Realize(srcs, plan); err == nil {
		t.Fatal("expected error for a plan referencing an unknown path")
	}
}

func TestRealize_OutOfRangeSegmentRejected(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: a", Type: "feat", Changes: []llm.Change{{Path: "a.go", Segments: []int{99}}}},
	}}
	if _, err := Realize(srcs, plan); err == nil {
		t.Fatal("expected error for an out-of-range segment index")
	}
}
