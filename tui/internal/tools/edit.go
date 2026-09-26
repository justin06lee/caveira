package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type editTool struct{ dir string }

func (t *editTool) Name() string { return "edit_file" }
func (t *editTool) Kind() Kind   { return KindWrite }
func (t *editTool) Description() string {
	return "Replace an exact string in a file, when the task calls for changing it. old_string must match the file text exactly, including whitespace and " +
		"indentation, and must occur exactly once unless replace_all is true; include a few surrounding lines to make it " +
		"unique. Read the file first so you quote it precisely. Do not include line-number prefixes from read_file in " +
		"old_string. Returns a diff of the change."
}
func (t *editTool) Schema() map[string]any {
	return schema(map[string]any{
		"path":        prop("string", "Path of the file to edit."),
		"old_string":  prop("string", "The exact text to replace."),
		"new_string":  prop("string", "The replacement text. May be empty to delete old_string."),
		"replace_all": prop("boolean", "Replace every occurrence instead of requiring a unique match. Defaults to false."),
	}, "path", "old_string", "new_string")
}
func (t *editTool) Preview(args json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(args, &a)
	_, rel := resolve(t.dir, a.Path)
	return rel
}

func (t *editTool) Run(_ context.Context, args json.RawMessage) Result {
	var a struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if a.Path == "" {
		return errorResult("path is required")
	}
	abs, rel := resolve(t.dir, a.Path)
	raw, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if a.OldString == "" {
				// Editing a missing file with an empty old_string is a
				// create; the model meant write_file, so behave like it.
				return (&writeTool{t.dir}).Run(context.Background(), mustJSON(map[string]string{"path": a.Path, "content": a.NewString}))
			}
			return errorResult("%s does not exist; use write_file to create it", rel)
		}
		return errorResult("%v", err)
	}
	if isBinary(raw) {
		return errorResult("%s is a binary file", rel)
	}
	if a.OldString == a.NewString {
		return errorResult("old_string and new_string are identical; nothing to do")
	}
	content := string(raw)

	if a.OldString == "" {
		return errorResult("old_string is empty; to replace the whole file use write_file")
	}
	count := strings.Count(content, a.OldString)
	if count == 0 {
		return errorResult("old_string not found in %s. %s", rel, notFoundHint(content, a.OldString))
	}
	if count > 1 && !a.ReplaceAll {
		return errorResult("old_string occurs %d times in %s; include more surrounding context to make it unique, or set replace_all", count, rel)
	}

	var updated string
	if a.ReplaceAll {
		updated = strings.ReplaceAll(content, a.OldString, a.NewString)
	} else {
		updated = strings.Replace(content, a.OldString, a.NewString, 1)
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return errorResult("%v", err)
	}

	diff := unifiedDiff(content, updated, 3)
	summary := "1 replacement"
	if count > 1 {
		summary = fmt.Sprintf("%d replacements", count)
	}
	return Result{
		Output:  fmt.Sprintf("Edited %s (%s).\n\n%s", rel, summary, diff),
		Summary: summary,
		Diff:    diff,
		Path:    rel,
	}
}

// notFoundHint tells the model what probably went wrong: line-number
// prefixes left in, or whitespace that does not match.
func notFoundHint(content, old string) string {
	trimmed := strings.TrimSpace(old)
	if trimmed != "" && strings.Contains(content, trimmed) {
		return "The text exists but the surrounding whitespace differs; match the file's indentation and line breaks exactly."
	}
	first := strings.SplitN(trimmed, "\n", 2)[0]
	if i := strings.IndexByte(first, '\t'); i > 0 && isDigits(first[:i]) {
		return "old_string seems to include the line-number prefix from read_file; quote only the file content after the tab."
	}
	if first != "" && !strings.Contains(content, first) {
		return "Not even the first line matches; re-read the file, it may have changed."
	}
	return "Re-read the file and copy the exact text."
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
