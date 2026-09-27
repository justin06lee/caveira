package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	globMaxResults = 500
	grepMaxLines   = 250
	grepMaxFiles   = 200
	grepMaxFile    = 2 << 20
)

// ---- glob ----

type globTool struct{ dir string }

func (t *globTool) Name() string { return "glob" }
func (t *globTool) Kind() Kind   { return KindRead }
func (t *globTool) Description() string {
	return "Find files by name pattern. Supports `*`, `?`, `[...]`, `{a,b}` and `**` for any depth, e.g. `src/**/*.ts` " +
		"or `**/Makefile`. Results are sorted by modification time, newest first. Ignores .git, node_modules and " +
		"similar build output. Use this instead of `find`."
}
func (t *globTool) Schema() map[string]any {
	return schema(map[string]any{
		"pattern": prop("string", "Glob pattern, relative to path."),
		"path":    prop("string", "Directory to search in. Defaults to the working directory."),
	}, "pattern")
}
func (t *globTool) Preview(args json.RawMessage) string {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	_ = decode(args, &a)
	if a.Path != "" && a.Path != "." {
		return a.Pattern + " in " + a.Path
	}
	return a.Pattern
}

func (t *globTool) Run(ctx context.Context, args json.RawMessage) Result {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if a.Pattern == "" {
		return errorResult("pattern is required")
	}
	root, rel := resolve(t.dir, a.Path)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return errorResult("%s is not a directory", rel)
	}
	pattern := strings.TrimPrefix(a.Pattern, "./")
	if !doublestar.ValidatePattern(pattern) {
		return errorResult("invalid glob pattern %q", a.Pattern)
	}

	type hit struct {
		path string
		mod  time.Time
	}
	var hits []hit
	total := 0
	err := doublestar.GlobWalk(os.DirFS(root), pattern, func(p string, d fs.DirEntry) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, part := range strings.Split(p, "/") {
			if skipDir[part] && !strings.Contains(pattern, part) {
				if d.IsDir() {
					return doublestar.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		total++
		info, err := d.Info()
		var mod time.Time
		if err == nil {
			mod = info.ModTime()
		}
		hits = append(hits, hit{p, mod})
		return nil
	}, doublestar.WithNoFollow())
	if err != nil && ctx.Err() != nil {
		return errorResult("cancelled")
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].mod.After(hits[j].mod) })
	if len(hits) > globMaxResults {
		hits = hits[:globMaxResults]
	}
	if len(hits) == 0 {
		return Result{Output: fmt.Sprintf("No files match %s in %s.", a.Pattern, rel), Summary: "no matches"}
	}
	var sb strings.Builder
	for _, h := range hits {
		if rel != "." {
			sb.WriteString(filepath.Join(rel, h.path))
		} else {
			sb.WriteString(h.path)
		}
		sb.WriteByte('\n')
	}
	if total > len(hits) {
		fmt.Fprintf(&sb, "\n[%d of %d matches shown; narrow the pattern to see the rest]\n", len(hits), total)
	}
	return Result{Output: sb.String(), Summary: plural(total, "file")}
}

// ---- grep ----

type grepTool struct{ dir string }

func (t *grepTool) Name() string { return "grep" }
func (t *grepTool) Kind() Kind   { return KindRead }
func (t *grepTool) Description() string {
	return "Search file contents with a regular expression (Go/RE2 syntax). By default lists the files that match; " +
		"set output_mode to `content` to see matching lines with line numbers, or `count` for match counts per file. " +
		"Restrict the files searched with glob (e.g. `*.go`, `src/**/*.tsx`). Skips binaries, .git and build output. " +
		"Use this instead of running grep or rg through bash."
}
func (t *grepTool) Schema() map[string]any {
	return schema(map[string]any{
		"pattern":          prop("string", "Regular expression to search for."),
		"path":             prop("string", "File or directory to search. Defaults to the working directory."),
		"glob":             prop("string", "Only search files whose path matches this glob, e.g. `*.py` or `**/*.test.ts`."),
		"output_mode":      map[string]any{"type": "string", "enum": []string{"files", "content", "count"}, "description": "What to return: `files` (default), `content` (matching lines), or `count`."},
		"case_insensitive": prop("boolean", "Ignore case. Defaults to false."),
		"context":          prop("integer", "Lines of context around each match in content mode. Defaults to 0."),
	}, "pattern")
}
func (t *grepTool) Preview(args json.RawMessage) string {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	_ = decode(args, &a)
	s := a.Pattern
	if a.Glob != "" {
		s += "  (" + a.Glob + ")"
	}
	if a.Path != "" && a.Path != "." {
		s += " in " + a.Path
	}
	return s
}

func (t *grepTool) Run(ctx context.Context, args json.RawMessage) Result {
	var a struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Glob            string `json:"glob"`
		OutputMode      string `json:"output_mode"`
		CaseInsensitive bool   `json:"case_insensitive"`
		Context         int    `json:"context"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if a.Pattern == "" {
		return errorResult("pattern is required")
	}
	expr := a.Pattern
	if a.CaseInsensitive {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return errorResult("invalid regular expression: %v", err)
	}
	mode := a.OutputMode
	switch mode {
	case "", "files", "files_with_matches":
		mode = "files"
	case "content", "count":
	default:
		return errorResult("output_mode must be files, content, or count")
	}
	if a.Glob != "" && !doublestar.ValidatePattern(a.Glob) {
		return errorResult("invalid glob %q", a.Glob)
	}

	root, rootRel := resolve(t.dir, a.Path)
	info, err := os.Stat(root)
	if err != nil {
		return errorResult("%s does not exist", rootRel)
	}

	type fileHit struct {
		path  string
		count int
		lines []string
	}
	var hits []fileHit
	totalLines := 0
	filesScanned := 0
	truncated := false

	visit := func(abs, rel string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if a.Glob != "" {
			ok, _ := doublestar.Match(a.Glob, rel)
			if !ok {
				if base := filepath.Base(rel); !strings.Contains(a.Glob, "/") {
					ok, _ = doublestar.Match(a.Glob, base)
				}
				if !ok {
					return nil
				}
			}
		}
		st, err := os.Stat(abs)
		if err != nil || st.Size() > grepMaxFile {
			return nil
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil
		}
		defer f.Close()
		head := make([]byte, 4096)
		n, _ := f.Read(head)
		if isBinary(head[:n]) {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}
		filesScanned++

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		var fh fileHit
		fh.path = rel
		var window []string // trailing context ring
		after := 0
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.MatchString(line) {
				fh.count++
				if mode == "content" && totalLines < grepMaxLines {
					for i, c := range window {
						fh.lines = append(fh.lines, fmt.Sprintf("%d-\t%s", lineNo-len(window)+i, clip(c, 400)))
					}
					window = window[:0]
					fh.lines = append(fh.lines, fmt.Sprintf("%d:\t%s", lineNo, clip(line, 400)))
					totalLines++
					after = a.Context
				} else if mode == "content" {
					truncated = true
				}
				continue
			}
			if mode == "content" && a.Context > 0 {
				if after > 0 {
					fh.lines = append(fh.lines, fmt.Sprintf("%d-\t%s", lineNo, clip(line, 400)))
					after--
				} else {
					window = append(window, line)
					if len(window) > a.Context {
						window = window[1:]
					}
				}
			}
		}
		if fh.count > 0 {
			hits = append(hits, fh)
			if len(hits) >= grepMaxFiles {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	}

	if !info.IsDir() {
		_ = visit(root, rootRel)
	} else {
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != root && skipDir[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			return visit(p, rel)
		})
		if err != nil && ctx.Err() != nil {
			return errorResult("cancelled")
		}
	}

	prefix := func(p string) string {
		if rootRel == "." || !info.IsDir() {
			if !info.IsDir() {
				return rootRel
			}
			return p
		}
		return filepath.Join(rootRel, p)
	}

	if len(hits) == 0 {
		return Result{Output: fmt.Sprintf("No matches for /%s/ in %s (%s searched).", a.Pattern, rootRel, plural(filesScanned, "file")), Summary: "no matches"}
	}
	var sb strings.Builder
	matches := 0
	for _, h := range hits {
		matches += h.count
	}
	switch mode {
	case "files":
		for _, h := range hits {
			sb.WriteString(prefix(h.path))
			sb.WriteByte('\n')
		}
	case "count":
		for _, h := range hits {
			fmt.Fprintf(&sb, "%s: %d\n", prefix(h.path), h.count)
		}
	case "content":
		for i, h := range hits {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(prefix(h.path))
			sb.WriteByte('\n')
			for _, l := range h.lines {
				sb.WriteString("  " + l + "\n")
			}
		}
	}
	if truncated {
		sb.WriteString("\n[results truncated; narrow the pattern, path, or glob]\n")
	}
	summary := fmt.Sprintf("%s in %s", plural(matches, "match"), plural(len(hits), "file"))
	summary = strings.Replace(summary, "matchs", "matches", 1)
	return Result{Output: sb.String(), Summary: summary}
}

// ---- list_dir ----

type listTool struct{ dir string }

func (t *listTool) Name() string { return "list_dir" }
func (t *listTool) Kind() Kind   { return KindRead }
func (t *listTool) Description() string {
	return "List the entries of a directory: names, with a trailing slash for directories and sizes for files. " +
		"Good for getting oriented when a task or question is about the project; use glob to find files by pattern across a tree."
}
func (t *listTool) Schema() map[string]any {
	return schema(map[string]any{
		"path": prop("string", "Directory to list. Defaults to the working directory."),
	})
}
func (t *listTool) Preview(args json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(args, &a)
	_, rel := resolve(t.dir, a.Path)
	return rel
}

func (t *listTool) Run(_ context.Context, args json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	abs, rel := resolve(t.dir, a.Path)
	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return errorResult("%s does not exist", rel)
		}
		return errorResult("%v", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s/\n", rel)
	dirs, files := 0, 0
	for i, e := range entries {
		if i >= 500 {
			fmt.Fprintf(&sb, "  … %d more entries\n", len(entries)-i)
			break
		}
		if e.IsDir() {
			dirs++
			fmt.Fprintf(&sb, "  %s/\n", e.Name())
			continue
		}
		files++
		size := ""
		if info, err := e.Info(); err == nil {
			size = humanBytes(int(info.Size()))
		}
		fmt.Fprintf(&sb, "  %s  %s\n", e.Name(), size)
	}
	return Result{Output: sb.String(), Summary: fmt.Sprintf("%s, %s", plural(dirs, "dir"), plural(files, "file")), Path: rel}
}
