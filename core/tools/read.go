package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	readDefaultLimit = 2000
	readMaxLine      = 2000
	readMaxBytes     = 512 << 10
)

type readTool struct{ dir string }

func (t *readTool) Name() string { return "read_file" }
func (t *readTool) Kind() Kind   { return KindRead }
func (t *readTool) Description() string {
	return "Read a file the task or the user's question needs. Returns the contents with line numbers, in the form `LINE\\tCONTENT`, " +
		"so you can quote exact lines when editing. By default reads up to 2000 lines from the start; " +
		"use offset and limit to page through large files. Always read a file before editing it. " +
		"Paths are relative to the working directory unless absolute."
}
func (t *readTool) Schema() map[string]any {
	return schema(map[string]any{
		"path":   prop("string", "Path of the file to read."),
		"offset": prop("integer", "1-based line number to start from. Defaults to 1."),
		"limit":  prop("integer", "Maximum number of lines to return. Defaults to 2000."),
	}, "path")
}
func (t *readTool) Preview(args json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(args, &a)
	_, rel := resolve(t.dir, a.Path)
	return rel
}

func (t *readTool) Run(_ context.Context, args json.RawMessage) Result {
	var a struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if a.Path == "" {
		return errorResult("path is required")
	}
	abs, rel := resolve(t.dir, a.Path)
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return errorResult("%s does not exist", rel).withHint(abs)
		}
		return errorResult("%v", err)
	}
	if info.IsDir() {
		return errorResult("%s is a directory; use list_dir", rel)
	}
	f, err := os.Open(abs)
	if err != nil {
		return errorResult("%v", err)
	}
	defer f.Close()

	buf := make([]byte, readMaxBytes)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if isBinary(buf) {
		return Result{
			Output:  fmt.Sprintf("%s is a binary file (%s); not shown.", rel, humanBytes(int(info.Size()))),
			Summary: "binary file", Path: rel,
		}
	}
	// Read the rest if the file is bigger than the first chunk but still
	// reasonable; beyond that the line window has to do the paging.
	if int64(n) < info.Size() && info.Size() <= 8<<20 {
		rest, _ := os.ReadFile(abs)
		if len(rest) > 0 {
			buf = rest
		}
	}

	lines := strings.Split(string(buf), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)
	start := a.Offset
	if start < 1 {
		start = 1
	}
	limit := a.Limit
	if limit <= 0 {
		limit = readDefaultLimit
	}
	if start > total {
		if total == 0 {
			return Result{Output: fmt.Sprintf("%s is empty.", rel), Summary: "empty file", Path: rel}
		}
		return errorResult("offset %d is past the end of %s (%d lines)", start, rel, total)
	}
	end := start - 1 + limit
	if end > total {
		end = total
	}

	var sb strings.Builder
	for i := start - 1; i < end; i++ {
		line := lines[i]
		if len(line) > readMaxLine {
			line = line[:readMaxLine] + "…"
		}
		fmt.Fprintf(&sb, "%d\t%s\n", i+1, line)
	}
	if end < total {
		fmt.Fprintf(&sb, "\n[showing lines %d-%d of %d; call again with offset=%d to continue]\n", start, end, total, end+1)
	}
	summary := plural(total, "line")
	if start > 1 || end < total {
		summary = fmt.Sprintf("lines %d-%d of %d", start, end, total)
	}
	return Result{Output: sb.String(), Summary: summary, Path: rel}
}

// withHint adds the absolute path to a not-found error so a model that
// guessed a relative path can see what it actually resolved to.
func (r Result) withHint(abs string) Result {
	r.Output += fmt.Sprintf(" (resolved to %s)", abs)
	return r
}
