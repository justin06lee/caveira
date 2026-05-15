package fabricate

import (
	"fmt"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
)

// SourceFile is one source HEAD-tree file with its content and segmentation.
type SourceFile struct {
	Path     string
	Mode     filemode.FileMode
	Content  []byte
	Segments []Segment
}

// fileChange is one file's resolved segment indices within a plan commit.
type fileChange struct {
	path string
	segs []int
}

// PlanCommitView is a resolved, clamped commit ready for realization.
type PlanCommitView struct {
	Message string
	Changes []fileChange
}

// Realize converts an LLM Plan over the given source files into a linear base
// sequence of SynthCommits. It validates the plan, clamps any segment or file
// the plan omitted, and computes per-commit cumulative-content blobs so the
// final state is byte-exact with the sources. The returned commits have IDs
// 0..n-1 and a linear parent chain; authors are left zero for a reshaper.
func Realize(sources []SourceFile, plan *llm.Plan) ([]SynthCommit, error) {
	byPath := make(map[string]SourceFile, len(sources))
	for _, s := range sources {
		byPath[s.Path] = s
	}

	// Per-commit, per-file resolved segment index sets.
	resolved := make([][]fileChange, len(plan.Commits))

	// Validate and resolve.
	for ci, pc := range plan.Commits {
		for _, ch := range pc.Changes {
			sf, ok := byPath[ch.Path]
			if !ok {
				return nil, fmt.Errorf("plan commit %d references unknown path %q", ci, ch.Path)
			}
			n := len(sf.Segments)
			var segs []int
			if ch.AllSegments {
				for i := 0; i < n; i++ {
					segs = append(segs, i)
				}
			} else {
				for _, idx := range ch.Segments {
					if idx < 0 || idx >= n {
						return nil, fmt.Errorf("plan commit %d: segment %d out of range for %q (0..%d)",
							ci, idx, ch.Path, n-1)
					}
					segs = append(segs, idx)
				}
			}
			resolved[ci] = append(resolved[ci], fileChange{path: ch.Path, segs: segs})
		}
	}

	// Clamp: ensure every segment of every file is assigned somewhere.
	assigned := map[string]map[int]bool{}
	lastCommit := map[string]int{} // path -> last commit index touching it
	for ci, changes := range resolved {
		for _, fc := range changes {
			if assigned[fc.path] == nil {
				assigned[fc.path] = map[int]bool{}
			}
			for _, s := range fc.segs {
				assigned[fc.path][s] = true
			}
			lastCommit[fc.path] = ci
		}
	}
	// Forgotten segments of touched files -> append to that file's last commit.
	// Iterate paths in sorted order so the resulting fileChange order (and thus
	// FileRef order in sc.Added) is deterministic across runs.
	assignedPaths := make([]string, 0, len(assigned))
	for path := range assigned {
		assignedPaths = append(assignedPaths, path)
	}
	sort.Strings(assignedPaths)
	for _, path := range assignedPaths {
		segset := assigned[path]
		sf := byPath[path]
		var missing []int
		for i := 0; i < len(sf.Segments); i++ {
			if !segset[i] {
				missing = append(missing, i)
			}
		}
		if len(missing) > 0 {
			ci := lastCommit[path]
			resolved[ci] = append(resolved[ci], fileChange{path: path, segs: missing})
		}
	}
	// Files never mentioned -> a final reconciliation commit.
	var forgotten []fileChange
	for _, s := range sources {
		if assigned[s.Path] == nil {
			all := make([]int, len(s.Segments))
			for i := range all {
				all[i] = i
			}
			forgotten = append(forgotten, fileChange{path: s.Path, segs: all})
		}
	}
	commits := make([]PlanCommitView, len(plan.Commits))
	for i, pc := range plan.Commits {
		commits[i] = PlanCommitView{Message: pc.Message, Changes: resolved[i]}
	}
	if len(forgotten) > 0 {
		sort.Slice(forgotten, func(i, j int) bool { return forgotten[i].path < forgotten[j].path })
		commits = append(commits, PlanCommitView{
			Message: "chore: finalize", Changes: forgotten,
		})
	}

	// Realize: walk commits, maintain cumulative per-file segment sets.
	cum := map[string]map[int]bool{}
	out := make([]SynthCommit, 0, len(commits))
	for ci, pcv := range commits {
		sc := SynthCommit{ID: ci, Message: pcv.Message, Feature: featureOf(pcv)}
		if ci > 0 {
			sc.Parents = []int{ci - 1}
		}
		deltaLines, newFiles := 0, 0
		for _, fc := range pcv.Changes {
			sf := byPath[fc.path]
			if cum[fc.path] == nil {
				cum[fc.path] = map[int]bool{}
				newFiles++ // first commit to touch this file
			}
			for _, s := range fc.segs {
				if !cum[fc.path][s] {
					deltaLines += segLineCount(sf.Segments[s])
				}
				cum[fc.path][s] = true
			}
			content := assembleContent(sf, cum[fc.path])
			sc.Added = append(sc.Added, FileRef{
				Path:    fc.path,
				Mode:    sf.Mode,
				Content: content,
				Blob:    plumbing.ComputeHash(plumbing.BlobObject, content),
			})
		}
		sc.Stats = &DiffStat{Lines: deltaLines, Files: len(pcv.Changes), NewFiles: newFiles}
		out = append(out, sc)
	}

	// Verify: every file's final cumulative set is complete.
	for _, s := range sources {
		if len(cum[s.Path]) != len(s.Segments) {
			return nil, fmt.Errorf("internal error: %q not fully realized (%d/%d segments)",
				s.Path, len(cum[s.Path]), len(s.Segments))
		}
	}
	return out, nil
}

// assembleContent concatenates the in-set segments of sf in index order.
func assembleContent(sf SourceFile, set map[int]bool) []byte {
	var b []byte
	for i, seg := range sf.Segments {
		if set[i] {
			b = append(b, seg.Bytes...)
		}
	}
	return b
}

func segLineCount(s Segment) int {
	n := s.EndLine - s.StartLine
	if n < 1 {
		// An empty or zero-line segment still counts as one visible edit.
		return 1
	}
	return n
}

// featureOf derives a commit's Feature label. It first honors an explicit
// conventional-commit scope in the message ("feat(walk): ..." -> "walk"). When
// the message carries no (scope) — common and valid for LLM plans — it falls
// back to the changed file paths: the first change under a non-root directory
// names the feature (via featureDir + basenameDir, mirroring how
// FlurrySequence labels features). A commit touching only root files keeps an
// empty Feature so it correctly stays on master in reshapeRats.
func featureOf(pcv PlanCommitView) string {
	if scope := scopeOf(pcv.Message); scope != "" {
		return scope
	}
	for _, fc := range pcv.Changes {
		if dir := featureDir(fc.path); dir != "." {
			return basenameDir(dir)
		}
	}
	return ""
}

// scopeOf extracts the conventional-commit scope from a message, e.g.
// "feat(walk): ..." -> "walk". Returns "" when there is no (scope).
func scopeOf(msg string) string {
	open := -1
	for i := 0; i < len(msg); i++ {
		switch msg[i] {
		case '(':
			open = i
		case ')':
			if open >= 0 && i > open+1 {
				return msg[open+1 : i]
			}
			return ""
		case ':':
			return ""
		}
	}
	return ""
}
