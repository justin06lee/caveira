package fabricate

import (
	"bytes"
	"strings"
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
	// Two blank-line-delimited blocks of >=8 non-blank lines each, so
	// SplitSegments cuts at the blank line into at least 2 segments.
	content := strings.Repeat("code line\n", 10) + "\n" + strings.Repeat("more code line\n", 10)
	src := srcFile("big.go", content)
	nSeg := len(src.Segments)
	if nSeg < 2 {
		t.Fatalf("fixture must yield >=2 segments, got %d", nSeg)
	}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: scaffold", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: []int{0}}}},
		{Message: "feat: rest", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: lastIndices(nSeg)}}},
	}}
	base, err := Realize([]SourceFile{src}, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	// Final commit holds remaining segments; cumulative must equal full content.
	if !bytes.Equal(cumulativeBig(base), []byte(content)) {
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

// fingerprint flattens a realized base into a string that captures both the
// ordered commit messages and the ordered Added paths of each commit, so two
// runs producing differently ordered FileRefs yield different fingerprints.
func fingerprint(base []SynthCommit) string {
	var b strings.Builder
	for _, c := range base {
		b.WriteString(c.Message)
		b.WriteByte('|')
		for _, fr := range c.Added {
			b.WriteString(fr.Path)
			b.WriteByte(',')
		}
		b.WriteByte(';')
	}
	return b.String()
}

func TestRealize_Deterministic(t *testing.T) {
	// Three source files, each multi-segment. The plan touches all three in
	// commit 0 but OMITS a later segment of two of them, so the forgotten-
	// segments clamp path fires for multiple files into the same commit.
	block := strings.Repeat("code line\n", 10) + "\n" + strings.Repeat("more code line\n", 10)
	a := srcFile("a.go", block)
	b := srcFile("b.go", block)
	c := srcFile("c.go", block)
	if len(a.Segments) < 2 || len(b.Segments) < 2 || len(c.Segments) < 2 {
		t.Fatalf("fixtures must each yield >=2 segments")
	}
	srcs := []SourceFile{a, b, c}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: scaffold", Type: "feat", Changes: []llm.Change{
			{Path: "a.go", Segments: []int{0}}, // omits a.go segment 1+
			{Path: "b.go", Segments: []int{0}}, // omits b.go segment 1+
			{Path: "c.go", AllSegments: true},
		}},
	}}
	var want string
	for run := 0; run < 20; run++ {
		base, err := Realize(srcs, plan)
		if err != nil {
			t.Fatalf("run %d: Realize: %v", run, err)
		}
		fp := fingerprint(base)
		if run == 0 {
			want = fp
			continue
		}
		if fp != want {
			t.Fatalf("run %d produced non-deterministic output:\n want %q\n got  %q", run, want, fp)
		}
	}
}

func TestRealize_OutOfOrderSegments(t *testing.T) {
	// A single multi-segment file. Commit 0 takes a LATER segment index and
	// commit 1 takes segment 0; the final cumulative content must still be
	// byte-exact with the source because assembly is index-ordered.
	content := strings.Repeat("code line\n", 10) + "\n" +
		strings.Repeat("more code line\n", 10) + "\n" +
		strings.Repeat("final code line\n", 10)
	src := srcFile("big.go", content)
	nSeg := len(src.Segments)
	if nSeg < 3 {
		t.Fatalf("fixture must yield >=3 segments, got %d", nSeg)
	}
	// commit 0 takes the last segment; commit 1 takes the rest (incl. segment 0).
	var rest []int
	for i := 0; i < nSeg-1; i++ {
		rest = append(rest, i)
	}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: tail", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: []int{nSeg - 1}}}},
		{Message: "feat: head", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: rest}}},
	}}
	base, err := Realize([]SourceFile{src}, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if !bytes.Equal(cumulativeBig(base), []byte(content)) {
		t.Fatal("out-of-order segment plan did not assemble to exact source content")
	}
}
