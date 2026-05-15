package llm

import (
	"fmt"
	"strings"
)

// SegmentInfo describes one segment of a file for the prompt's segment map.
type SegmentInfo struct {
	Index     int
	StartLine int
	EndLine   int
}

// FileInput is one source file presented to the LLM.
type FileInput struct {
	Path     string
	Kind     string // "chore", "code", or "test"
	Content  string
	Segments []SegmentInfo
}

const perFileContentCap = 8000 // bytes of content shown per file before truncation

// BuildPrompt builds the LLM prompt. Every file's path, kind, and full segment
// map are always included. File contents are included until the cumulative
// byte budget is exhausted; thereafter (and for any single file over
// perFileContentCap) content is truncated with an explicit marker. Truncation
// affects only the LLM's judgment — segment indices are validated downstream.
func BuildPrompt(files []FileInput, budget int) string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\n## Source files\n\n")

	used := 0
	for _, f := range files {
		fmt.Fprintf(&b, "### %s (%s) — %d segments\n", f.Path, f.Kind, len(f.Segments))
		for _, s := range f.Segments {
			fmt.Fprintf(&b, "  [%d] lines %d-%d\n", s.Index, s.StartLine, s.EndLine)
		}
		content := f.Content
		truncated := false
		if len(content) > perFileContentCap {
			content = content[:perFileContentCap]
			truncated = true
		}
		if used+len(content) > budget {
			remaining := budget - used
			if remaining < 0 {
				remaining = 0
			}
			content = content[:remaining]
			truncated = true
		}
		used += len(content)
		b.WriteString("```\n")
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		if truncated {
			b.WriteString("... [content truncated] ...\n")
		}
		b.WriteString("```\n\n")
	}
	return b.String()
}

const instructions = `You are designing a realistic git commit history for a codebase.

You are given the final state of every file in a repository, each split into
numbered segments (contiguous line ranges). Design a believable sequence of
commits that, applied in order, builds this codebase from nothing.

Rules:
- Group related files into features. Order commits so dependencies come first
  (configuration and scaffolding early, features next, tests after their code).
- A commit may include a whole file ("segments": "all") or, for larger files,
  only some segments — split a big file across several commits so earlier
  commits scaffold it and later commits flesh it out.
- Every segment of every file must appear in at least one commit overall.
- Write conventional-commit messages: "feat(scope): ...", "test(scope): ...",
  "chore: ...", "fix(scope): ...", "refactor(scope): ...".

Respond with ONLY a JSON object, no prose, in exactly this shape:

{
  "commits": [
    { "message": "chore: initialize module",
      "type": "chore",
      "changes": [ {"path": "go.mod", "segments": "all"} ] },
    { "message": "feat(walk): scaffold loader",
      "type": "feat",
      "changes": [ {"path": "internal/walk/load.go", "segments": [0, 1]} ] }
  ]
}`
