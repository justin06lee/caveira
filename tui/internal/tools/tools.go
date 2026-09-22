// Package tools is what the agent can do to the machine: read, search, edit,
// write, and run. Each tool describes itself to the model and executes one
// call. Outputs are bounded so a careless `cat` cannot flood the context.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/justin06lee/caveira/tui/internal/llm"
)

// Kind is how much a tool can change: it decides what needs confirmation.
type Kind int

const (
	KindRead    Kind = iota // looks at files; never needs confirmation
	KindWrite               // changes files under the working directory
	KindExecute             // runs arbitrary commands
)

func (k Kind) String() string {
	switch k {
	case KindWrite:
		return "write"
	case KindExecute:
		return "execute"
	default:
		return "read"
	}
}

// Result is what a tool call produced.
type Result struct {
	// Output is the text the model sees.
	Output string
	// Summary is one short line for the UI: "42 lines", "exit 0 · 3 lines".
	Summary string
	// Diff, when set, is a unified-ish diff the UI shows for edits.
	Diff string
	// Path is the file touched, if any, relative to the working directory.
	Path    string
	IsError bool
}

func errorResult(format string, args ...any) Result {
	msg := fmt.Sprintf(format, args...)
	return Result{Output: "Error: " + msg, Summary: msg, IsError: true}
}

// Tool is one capability.
type Tool interface {
	Name() string
	Description() string
	// Schema is the JSON schema of the arguments object.
	Schema() map[string]any
	Kind() Kind
	// Preview is the one-liner shown while the call is running, from the
	// parsed arguments: the path, the command, the pattern.
	Preview(args json.RawMessage) string
	Run(ctx context.Context, args json.RawMessage) Result
}

// Registry is the tool set for one working directory.
type Registry struct {
	WorkDir string
	tools   map[string]Tool
	order   []string
}

// Default is the standard coding tool set rooted at dir.
func Default(dir string) *Registry {
	r := &Registry{WorkDir: dir, tools: map[string]Tool{}}
	for _, t := range []Tool{
		&readTool{dir}, &writeTool{dir}, &editTool{dir},
		&bashTool{dir}, &globTool{dir}, &grepTool{dir}, &listTool{dir},
	} {
		r.Register(t)
	}
	return r
}

func (r *Registry) Register(t Tool) {
	if _, dup := r.tools[t.Name()]; !dup {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions is the tool list in the wire shape the model expects.
func (r *Registry) Definitions() []llm.Tool {
	out := make([]llm.Tool, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		schema, _ := json.Marshal(t.Schema())
		out = append(out, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  schema,
			},
		})
	}
	return out
}

// Run executes one call. Unknown tools and unparseable arguments come back as
// error results rather than Go errors, so the model gets to read them and
// correct itself.
func (r *Registry) Run(ctx context.Context, name string, args json.RawMessage) Result {
	t, ok := r.tools[name]
	if !ok {
		names := make([]string, 0, len(r.order))
		names = append(names, r.order...)
		sort.Strings(names)
		return errorResult("unknown tool %q. Available tools: %s", name, strings.Join(names, ", "))
	}
	if len(strings.TrimSpace(string(args))) == 0 {
		args = json.RawMessage("{}")
	}
	if !json.Valid(args) {
		return errorResult("arguments for %s are not valid JSON: %s", name, clip(string(args), 200))
	}
	return t.Run(ctx, args)
}

// resolve turns a model-supplied path into an absolute one under dir, and a
// display form relative to dir. Absolute paths are honored: the agent is
// allowed to work outside the project when asked to.
func resolve(dir, p string) (abs, rel string) {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return dir, "."
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	abs = filepath.Clean(p)
	if r, err := filepath.Rel(dir, abs); err == nil && !strings.HasPrefix(r, "..") {
		rel = r
	} else {
		rel = abs
	}
	return abs, rel
}

func decode(args json.RawMessage, into any) error {
	dec := json.NewDecoder(strings.NewReader(string(args)))
	return dec.Decode(into)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// truncateOutput keeps the head and tail of a long output with a marker in
// between: both ends matter for command output (the command echo and the
// final error), and the middle is usually the least informative part.
func truncateOutput(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	head := limit * 2 / 3
	tail := limit - head
	omitted := len(s) - head - tail
	// Cut on line boundaries where possible so the marker sits on its own line.
	if i := strings.LastIndexByte(s[:head], '\n'); i > head/2 {
		head = i
	}
	if i := strings.IndexByte(s[len(s)-tail:], '\n'); i >= 0 && i < tail/2 {
		tail -= i + 1
	}
	return s[:head] + fmt.Sprintf("\n\n… [%d bytes truncated] …\n\n", omitted) + s[len(s)-tail:], true
}

func isBinary(b []byte) bool {
	if len(b) > 8000 {
		b = b[:8000]
	}
	if len(b) == 0 {
		return false
	}
	if !utf8.Valid(b) {
		// Could be a cut multibyte sequence at the end; check for NULs too.
		nul := 0
		for _, c := range b {
			if c == 0 {
				nul++
			}
		}
		return nul > 0 || !utf8.Valid(b[:len(b)-4])
	}
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// skipDir is the set of directories no search should descend into unless
// asked for by name. They are huge and never what the model wants.
var skipDir = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true, ".cache": true,
	".turbo": true, "vendor": true, ".idea": true, ".vscode": true, ".DS_Store": true,
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
