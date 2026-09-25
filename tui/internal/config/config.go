// Package config resolves where caveira sends requests and as what: API key,
// base URL, model, reasoning effort. Sources, lowest to highest priority:
// built-in defaults, ~/.caveira/config.json, .env files between the
// repository root and the working directory, environment variables, and
// finally command-line flags (applied by the caller).
package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultBaseURL = "https://api.abliteration.ai/v1"
	DefaultModel   = "abliterated-model"
)

// Settings is the resolved configuration.
type Settings struct {
	APIKey          string `json:"api_key,omitempty"`
	BaseURL         string `json:"base_url,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// ContextWindow overrides the known size for the model, in tokens.
	ContextWindow int `json:"context_window,omitempty"`
	// MaxTokens caps each completion; 0 leaves it to the server.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Confirm asks before running commands or changing files.
	Confirm bool `json:"confirm,omitempty"`

	// KeySource says where the API key came from, for the status line and
	// for error messages when there is none.
	KeySource string `json:"-"`
}

// Dir is ~/.caveira.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".caveira"
	}
	return filepath.Join(home, ".caveira")
}

// Path is the settings file.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load resolves settings for a run rooted at workDir.
func Load(workDir string) (Settings, error) {
	s := Settings{BaseURL: DefaultBaseURL, Model: DefaultModel}

	var errs []error
	if b, err := os.ReadFile(Path()); err == nil {
		var file Settings
		if err := json.Unmarshal(b, &file); err != nil {
			errs = append(errs, errors.New(Path()+": "+err.Error()))
		} else {
			merge(&s, file, "config.json")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}

	// .env files: outermost first so the closest one wins.
	for _, f := range envFiles(workDir) {
		vars, err := parseEnvFile(f)
		if err != nil {
			continue
		}
		applyEnv(&s, func(k string) string { return vars[k] }, filepath.Base(f))
	}

	applyEnv(&s, os.Getenv, "environment")

	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	return s, errors.Join(errs...)
}

func merge(dst *Settings, src Settings, source string) {
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
		dst.KeySource = source
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.ReasoningEffort != "" {
		dst.ReasoningEffort = src.ReasoningEffort
	}
	if src.ContextWindow > 0 {
		dst.ContextWindow = src.ContextWindow
	}
	if src.MaxTokens > 0 {
		dst.MaxTokens = src.MaxTokens
	}
	if src.Confirm {
		dst.Confirm = true
	}
}

// Recognized variable names. CAVEIRA_* is ours; the ABLIT* names are what
// abliteration.ai's own docs and other tools already use, so a key exported
// for Codex or Claude Code works here too.
var (
	keyVars     = []string{"CAVEIRA_API_KEY", "ABLITERATION_API_KEY", "ABLIT_KEY"}
	baseURLVars = []string{"CAVEIRA_BASE_URL", "ABLITERATION_BASE_URL"}
	modelVars   = []string{"CAVEIRA_MODEL", "ABLITERATION_MODEL"}
	effortVars  = []string{"CAVEIRA_REASONING_EFFORT", "CAVEIRA_EFFORT"}
)

func applyEnv(s *Settings, get func(string) string, source string) {
	first := func(names []string) string {
		for _, n := range names {
			if v := strings.TrimSpace(get(n)); v != "" {
				return v
			}
		}
		return ""
	}
	if v := first(keyVars); v != "" {
		s.APIKey = v
		s.KeySource = source
	}
	if v := first(baseURLVars); v != "" {
		s.BaseURL = v
	}
	if v := first(modelVars); v != "" {
		s.Model = v
	}
	if v := first(effortVars); v != "" {
		s.ReasoningEffort = v
	}
	if v := strings.ToLower(get("CAVEIRA_CONFIRM")); v == "1" || v == "true" || v == "yes" {
		s.Confirm = true
	}
}

// envFiles lists .env and .env.local from the git root (or the directory
// itself when not in a repo) down to workDir, outermost first.
func envFiles(workDir string) []string {
	root := gitRoot(workDir)
	if root == "" {
		root = workDir
	}
	var dirs []string
	for d := workDir; ; d = filepath.Dir(d) {
		dirs = append([]string{d}, dirs...)
		if d == root || d == filepath.Dir(d) {
			break
		}
	}
	var out []string
	for _, d := range dirs {
		for _, name := range []string{".env", ".env.local"} {
			p := filepath.Join(d, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				out = append(out, p)
			}
		}
	}
	return out
}

func gitRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			return ""
		}
	}
}

// parseEnvFile reads KEY=VALUE lines. Quotes are stripped, `export` is
// allowed, comments and blank lines are skipped. Interpolation is not done.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	vars := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		vars[k] = v
	}
	return vars, sc.Err()
}

// Save writes the settings file, 0600 because it may hold the key.
func Save(s Settings) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(b, '\n'), 0o600)
}

// ModelSpec is what caveira knows about a model without asking the server.
type ModelSpec struct {
	ContextWindow int
	// Prices in USD per million tokens. Zero means unknown or free.
	InputPerM, CachedPerM, OutputPerM float64
}

var knownModels = map[string]ModelSpec{
	"abliterated-model":          {ContextWindow: 262_144, InputPerM: 1, CachedPerM: 0.10, OutputPerM: 3},
	"abliterated-model-large":    {ContextWindow: 1_000_000, InputPerM: 3, CachedPerM: 0.30, OutputPerM: 5},
	"abliterated-model-large-v2": {ContextWindow: 1_000_000, InputPerM: 3, CachedPerM: 0.30, OutputPerM: 5},
}

// Known reports whether caveira has a spec for model.
func Known(model string) bool {
	_, ok := knownModels[model]
	return ok
}

// KnownModels lists the models caveira has specs for, sorted.
func KnownModels() []string {
	out := make([]string, 0, len(knownModels))
	for id := range knownModels {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Spec returns what is known about model, falling back to a conservative
// context window for anything unrecognized (local models, other gateways).
func Spec(model string) ModelSpec {
	if s, ok := knownModels[model]; ok {
		return s
	}
	return ModelSpec{ContextWindow: 128_000}
}

// Efforts are the reasoning levels abliteration.ai accepts. Each model maps
// them onto the modes it actually has; "none" turns reasoning off where the
// model allows it.
var Efforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

func ValidEffort(e string) bool {
	for _, v := range Efforts {
		if v == e {
			return true
		}
	}
	return false
}
