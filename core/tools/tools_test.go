package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func args(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReadWriteEdit(t *testing.T) {
	dir := t.TempDir()
	reg := Default(dir)
	ctx := context.Background()

	r := reg.Run(ctx, "write_file", args(t, map[string]any{"path": "a/b.txt", "content": "one\ntwo\nthree\n"}))
	if r.IsError {
		t.Fatalf("write failed: %s", r.Output)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b.txt")); err != nil {
		t.Fatal("file not created")
	}

	r = reg.Run(ctx, "read_file", args(t, map[string]any{"path": "a/b.txt"}))
	if r.IsError || !strings.Contains(r.Output, "2\ttwo") {
		t.Fatalf("read: %s", r.Output)
	}
	r = reg.Run(ctx, "read_file", args(t, map[string]any{"path": "a/b.txt", "offset": 2, "limit": 1}))
	if !strings.Contains(r.Output, "2\ttwo") || strings.Contains(r.Output, "1\tone") {
		t.Fatalf("paged read: %s", r.Output)
	}

	r = reg.Run(ctx, "edit_file", args(t, map[string]any{"path": "a/b.txt", "old_string": "two", "new_string": "2"}))
	if r.IsError || !strings.Contains(r.Diff, "-two") || !strings.Contains(r.Diff, "+2") {
		t.Fatalf("edit: %s / %s", r.Output, r.Diff)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a", "b.txt"))
	if string(got) != "one\n2\nthree\n" {
		t.Fatalf("content after edit: %q", got)
	}

	// Not unique without replace_all.
	_ = os.WriteFile(filepath.Join(dir, "dup.txt"), []byte("x\nx\n"), 0o644)
	r = reg.Run(ctx, "edit_file", args(t, map[string]any{"path": "dup.txt", "old_string": "x", "new_string": "y"}))
	if !r.IsError || !strings.Contains(r.Output, "2 times") {
		t.Fatalf("expected ambiguity error, got %s", r.Output)
	}
	r = reg.Run(ctx, "edit_file", args(t, map[string]any{"path": "dup.txt", "old_string": "x", "new_string": "y", "replace_all": true}))
	if r.IsError {
		t.Fatalf("replace_all: %s", r.Output)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dup.txt"))
	if string(got) != "y\ny\n" {
		t.Fatalf("replace_all content: %q", got)
	}

	// Line-number prefix hint.
	r = reg.Run(ctx, "edit_file", args(t, map[string]any{"path": "a/b.txt", "old_string": "1\tone", "new_string": "uno"}))
	if !r.IsError || !strings.Contains(r.Output, "line-number prefix") {
		t.Fatalf("expected prefix hint, got %s", r.Output)
	}
	// Whitespace hint.
	r = reg.Run(ctx, "edit_file", args(t, map[string]any{"path": "a/b.txt", "old_string": "  three", "new_string": "3"}))
	if !r.IsError || !strings.Contains(r.Output, "whitespace") {
		t.Fatalf("expected whitespace hint, got %s", r.Output)
	}
}

func TestBash(t *testing.T) {
	dir := t.TempDir()
	reg := Default(dir)
	ctx := context.Background()

	r := reg.Run(ctx, "bash", args(t, map[string]any{"command": "echo hi; echo err >&2; exit 3"}))
	if !r.IsError || !strings.Contains(r.Output, "hi") || !strings.Contains(r.Output, "err") || !strings.Contains(r.Output, "[exit code 3]") {
		t.Fatalf("bash: %s", r.Output)
	}
	r = reg.Run(ctx, "bash", args(t, map[string]any{"command": "pwd"}))
	if strings.TrimSpace(strings.Split(r.Output, "\n")[0]) != dir {
		// macOS may resolve /var → /private/var; accept a suffix match.
		if !strings.HasSuffix(strings.TrimSpace(strings.Split(r.Output, "\n")[0]), strings.TrimPrefix(dir, "/private")) {
			t.Fatalf("cwd: %s", r.Output)
		}
	}
	r = reg.Run(ctx, "bash", args(t, map[string]any{"command": "sleep 5", "timeout": 1}))
	if !r.IsError || !strings.Contains(r.Output, "timed out") {
		t.Fatalf("timeout: %s", r.Output)
	}
	r = reg.Run(ctx, "bash", args(t, map[string]any{"command": "true"}))
	if r.IsError || !strings.Contains(r.Output, "no output") {
		t.Fatalf("silent success: %s", r.Output)
	}
}

func TestGlobGrepList(t *testing.T) {
	dir := t.TempDir()
	must := func(p, content string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		_ = os.WriteFile(filepath.Join(dir, p), []byte(content), 0o644)
	}
	must("src/a.go", "package a\nfunc Hello() {}\n")
	must("src/deep/b.go", "package deep\nfunc hello() {}\n")
	must("node_modules/x/c.go", "func Hello() {}\n")
	must("README.md", "# hi\n")
	reg := Default(dir)
	ctx := context.Background()

	r := reg.Run(ctx, "glob", args(t, map[string]any{"pattern": "**/*.go"}))
	if r.IsError || !strings.Contains(r.Output, "src/a.go") || !strings.Contains(r.Output, "src/deep/b.go") || strings.Contains(r.Output, "node_modules") {
		t.Fatalf("glob: %s", r.Output)
	}
	r = reg.Run(ctx, "grep", args(t, map[string]any{"pattern": "func Hello", "output_mode": "content"}))
	if r.IsError || !strings.Contains(r.Output, "src/a.go") || !strings.Contains(r.Output, "2:") || strings.Contains(r.Output, "deep") {
		t.Fatalf("grep content: %s", r.Output)
	}
	r = reg.Run(ctx, "grep", args(t, map[string]any{"pattern": "hello", "case_insensitive": true, "glob": "*.go"}))
	if r.IsError || !strings.Contains(r.Output, "src/deep/b.go") || !strings.Contains(r.Summary, "2 files") {
		t.Fatalf("grep files: %s (%s)", r.Output, r.Summary)
	}
	r = reg.Run(ctx, "grep", args(t, map[string]any{"pattern": "nothing-here"}))
	if r.IsError || !strings.Contains(r.Output, "No matches") {
		t.Fatalf("grep none: %s", r.Output)
	}
	r = reg.Run(ctx, "list_dir", args(t, map[string]any{}))
	if r.IsError || !strings.Contains(r.Output, "src/") || !strings.Contains(r.Output, "README.md") {
		t.Fatalf("list: %s", r.Output)
	}
	r = reg.Run(ctx, "nope", args(t, map[string]any{}))
	if !r.IsError || !strings.Contains(r.Output, "unknown tool") {
		t.Fatalf("unknown tool: %s", r.Output)
	}
	r = reg.Run(ctx, "read_file", json.RawMessage(`{"path": `))
	if !r.IsError || !strings.Contains(r.Output, "not valid JSON") {
		t.Fatalf("bad json: %s", r.Output)
	}
}

func TestUnifiedDiff(t *testing.T) {
	d := unifiedDiff("a\nb\nc\nd\ne\n", "a\nb\nX\nd\ne\n", 1)
	want := "@@ -2,3 +2,3 @@\n b\n-c\n+X\n d"
	if d != want {
		t.Fatalf("diff:\n%s\nwant:\n%s", d, want)
	}
	if got := unifiedDiff("", "new\n", 3); !strings.Contains(got, "+new") {
		t.Fatalf("create diff: %s", got)
	}
}

func TestTruncateOutput(t *testing.T) {
	s := strings.Repeat("line\n", 1000)
	out, cut := truncateOutput(s, 500)
	if !cut || len(out) > 700 || !strings.Contains(out, "truncated") {
		t.Fatalf("truncate: %d %v", len(out), cut)
	}
}

// echo and printf of fixed text are refused: their output is known before
// they run and reaches only the model, so a model calling them is trying
// to talk through the shell. Anything that reads, expands, or writes runs.
func TestBashRefusesPrintingFixedText(t *testing.T) {
	dir := t.TempDir()
	reg := Default(dir)
	run := func(cmd string) Result {
		b, _ := json.Marshal(map[string]string{"command": cmd})
		return reg.Run(context.Background(), "bash", b)
	}
	for _, cmd := range []string{`echo \nHello, world!\n`, `echo 'hello'`, `echo "Hello, world!"`, `printf "hi\n"`, `echo`, `  echo hi  `} {
		r := run(cmd)
		if !r.IsError || !strings.Contains(r.Output, "Say it in your reply") {
			t.Errorf("%q should be refused, got %+v", cmd, r)
		}
	}
	for _, cmd := range []string{`echo $HOME`, `echo hi > out.txt`, `echo a | wc -c`, `echo hi && ls`, `echo $(pwd)`, `echo *`, "echo one\necho two"} {
		if r := run(cmd); r.IsError {
			t.Errorf("%q should run, got %+v", cmd, r)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "out.txt")); err != nil || string(b) != "hi\n" {
		t.Errorf("echo with a redirect did not write the file: %q, %v", b, err)
	}
}

// Small models quote booleans and numbers; the tools read them anyway.
func TestQuotedScalarsAreRead(t *testing.T) {
	var a struct {
		Path       string `json:"path"`
		ReplaceAll bool   `json:"replace_all"`
		Limit      int    `json:"limit"`
	}
	if err := decode(json.RawMessage(`{"path":"12","replace_all":"true","limit":"50"}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.Path != "12" || !a.ReplaceAll || a.Limit != 50 {
		t.Fatalf("%+v", a)
	}
	if err := decode(json.RawMessage(`{"replace_all":"sometimes"}`), &a); err == nil {
		t.Fatal("read a string that is not a boolean as one")
	}
}
