package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type writeTool struct{ dir string }

func (t *writeTool) Name() string { return "write_file" }
func (t *writeTool) Kind() Kind   { return KindWrite }
func (t *writeTool) Description() string {
	return "Create a file or completely overwrite an existing one with the given content. Parent directories are created. " +
		"Use it only when the task needs a new file or a full rewrite; prefer edit_file for changing part of an existing file: " +
		"it is smaller, safer, and shows a diff. Never write a file to hold a reply, a note to the user, or a test of the tool."
}
func (t *writeTool) Schema() map[string]any {
	return schema(map[string]any{
		"path":    prop("string", "Path of the file to write."),
		"content": prop("string", "The full content of the file."),
	}, "path", "content")
}
func (t *writeTool) Preview(args json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(args, &a)
	_, rel := resolve(t.dir, a.Path)
	return rel
}

func (t *writeTool) Run(_ context.Context, args json.RawMessage) Result {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if a.Path == "" {
		return errorResult("path is required")
	}
	abs, rel := resolve(t.dir, a.Path)
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return errorResult("%s is a directory", rel)
	}
	old, readErr := os.ReadFile(abs)
	existed := readErr == nil

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return errorResult("%v", err)
	}
	if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
		return errorResult("%v", err)
	}

	lines := strings.Count(a.Content, "\n")
	if a.Content != "" && !strings.HasSuffix(a.Content, "\n") {
		lines++
	}
	verb := "Created"
	summary := fmt.Sprintf("created · %s", plural(lines, "line"))
	var diff string
	if existed {
		verb = "Overwrote"
		summary = fmt.Sprintf("overwrote · %s", plural(lines, "line"))
		if !isBinary(old) {
			diff = unifiedDiff(string(old), a.Content, 3)
		}
	}
	return Result{
		Output:  fmt.Sprintf("%s %s (%s, %s).", verb, rel, plural(lines, "line"), humanBytes(len(a.Content))),
		Summary: summary,
		Diff:    diff,
		Path:    rel,
	}
}
