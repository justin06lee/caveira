package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/importer"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/local"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is what the window talks to. Every exported method is callable from
// the frontend; events go the other way through emit.
type App struct {
	ctx     context.Context
	version string

	mu    sync.Mutex
	prefs prefs
	chats map[string]*chat
	// gen counts settings changes. A chat built under an older gen is
	// rebuilt from the new settings before its next turn.
	gen int
	// sink, when set, takes events instead of the Wails runtime: the
	// browser harness in harness_test.go.
	sink func(name string, data any)
	// picked is the last local model set up, so every new chat does not
	// ask the server again.
	picked *localPick
	// found is what the last look for other agents' chats found.
	found []importer.Chat
}

// localPick is what local.Apply settled on for one set of settings.
type localPick struct {
	key     string
	baseURL string
	model   string
	window  int
}

func NewApp(version string) *App {
	return &App{version: version, prefs: loadPrefs(), chats: map[string]*chat{}}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) emit(name string, data any) {
	if a.sink != nil {
		a.sink(name, data)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// Project is a folder caveira works in.
type Project struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Short  string `json:"short"` // the path with ~ for the home directory
	Branch string `json:"branch"`
}

// Boot is everything the window needs to draw its first frame.
type Boot struct {
	Version   string       `json:"version"`
	Settings  SettingsView `json:"settings"`
	Projects  []Project    `json:"projects"`
	Workspace Workspace    `json:"workspace"`
	// Onboarded is false until the first-run steps are done.
	Onboarded bool `json:"onboarded"`
}

func (a *App) Boot() Boot {
	waitShellEnv()
	a.mu.Lock()
	dirs := a.prefs.existingProjects()
	ws, onboarded := a.prefs.Workspace, a.prefs.Onboarded
	a.mu.Unlock()
	return Boot{Version: a.version, Settings: a.Settings(), Projects: projects(dirs), Workspace: workspace(ws), Onboarded: onboarded}
}

// Projects is the recent projects, for after an import adds some.
func (a *App) Projects() []Project {
	a.mu.Lock()
	dirs := a.prefs.existingProjects()
	a.mu.Unlock()
	return projects(dirs)
}

func projects(dirs []string) []Project {
	out := make([]Project, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, project(d))
	}
	return out
}

func project(dir string) Project {
	return Project{Path: dir, Name: filepath.Base(dir), Short: shortPath(dir), Branch: gitBranch(dir)}
}

// OpenProject makes dir the most recent project. The app has no notion
// of a current project; the frontend keeps that.
func (a *App) OpenProject(dir string) (Project, error) {
	dir = filepath.Clean(dir)
	st, err := os.Stat(dir)
	if err != nil {
		return Project{}, err
	}
	if !st.IsDir() {
		return Project{}, fmt.Errorf("%s is not a folder", dir)
	}
	a.mu.Lock()
	a.prefs.touch(dir)
	err = a.prefs.save()
	a.mu.Unlock()
	return project(dir), err
}

// ChooseProject asks for a folder. An empty path means the user cancelled.
func (a *App) ChooseProject() (Project, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Open a project",
		CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return Project{}, err
	}
	return a.OpenProject(dir)
}

func (a *App) ForgetProject(dir string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.forget(dir)
	return a.prefs.save()
}

// Reveal shows a folder in the Finder, or the file manager on Linux.
func (a *App) Reveal(dir string) error {
	opener := "open"
	if goruntime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	cmd := exec.Command(opener, dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// SettingsView is the settings screen's state.
type SettingsView struct {
	Local bool `json:"local"`
	// APIKey is a hint at the key in use ("ak_…3f2a"), never the key.
	APIKey string `json:"apiKey"`
	// KeySource is where that key came from: config.json, environment,
	// .env.local, and so on. Only a key from config.json can be changed
	// here; the others win over it.
	KeySource    string   `json:"keySource"`
	BaseURL      string   `json:"baseUrl"`
	Model        string   `json:"model"`
	Effort       string   `json:"effort"`
	Confirm      bool     `json:"confirm"`
	LocalBaseURL string   `json:"localBaseUrl"`
	LocalModel   string   `json:"localModel"`
	Theme        string   `json:"theme"`
	Efforts      []string `json:"efforts"`
	// Ready is false when a chat could not run: no key for a remote
	// endpoint. Local problems only show when a chat starts.
	Ready bool `json:"ready"`
}

func (a *App) Settings() SettingsView {
	home, _ := os.UserHomeDir()
	cfg, _ := config.Load(home)
	a.mu.Lock()
	p := a.prefs
	a.mu.Unlock()
	return SettingsView{
		Local:        p.Local,
		APIKey:       keyHint(cfg.APIKey),
		KeySource:    cfg.KeySource,
		BaseURL:      cfg.BaseURL,
		Model:        cfg.Model,
		Effort:       cfg.ReasoningEffort,
		Confirm:      cfg.Confirm,
		LocalBaseURL: firstNonEmpty(p.LocalBaseURL, local.DefaultBaseURL),
		LocalModel:   p.LocalModel,
		Theme:        p.Theme,
		Efforts:      config.Efforts,
		Ready:        p.Local || cfg.APIKey != "" || isLocal(cfg.BaseURL),
	}
}

// SettingsInput is a change from the settings screen. A nil APIKey
// leaves the stored key alone; an empty one removes it.
type SettingsInput struct {
	Local        bool    `json:"local"`
	APIKey       *string `json:"apiKey"`
	BaseURL      string  `json:"baseUrl"`
	Model        string  `json:"model"`
	Effort       string  `json:"effort"`
	Confirm      bool    `json:"confirm"`
	LocalBaseURL string  `json:"localBaseUrl"`
	LocalModel   string  `json:"localModel"`
	Theme        string  `json:"theme"`
}

func (a *App) SaveSettings(in SettingsInput) (SettingsView, error) {
	if in.Effort != "" && !config.ValidEffort(in.Effort) {
		return a.Settings(), fmt.Errorf("reasoning effort must be one of %s", strings.Join(config.Efforts, ", "))
	}
	if u := strings.TrimSpace(in.BaseURL); u != "" {
		if _, err := url.ParseRequestURI(u); err != nil {
			return a.Settings(), fmt.Errorf("%q is not a URL", u)
		}
	}

	file, err := config.ReadFile()
	if err != nil {
		return a.Settings(), err
	}
	if in.APIKey != nil {
		file.APIKey = strings.TrimSpace(*in.APIKey)
	}
	file.BaseURL = unlessDefault(strings.TrimRight(strings.TrimSpace(in.BaseURL), "/"), config.DefaultBaseURL)
	file.Model = unlessDefault(strings.TrimSpace(in.Model), config.DefaultModel)
	file.ReasoningEffort = in.Effort
	file.Confirm = in.Confirm
	if err := config.Save(file); err != nil {
		return a.Settings(), err
	}

	a.mu.Lock()
	a.prefs.Local = in.Local
	a.prefs.LocalBaseURL = unlessDefault(strings.TrimRight(strings.TrimSpace(in.LocalBaseURL), "/"), local.DefaultBaseURL)
	a.prefs.LocalModel = strings.TrimSpace(in.LocalModel)
	if in.Theme != "" {
		a.prefs.Theme = in.Theme
	}
	err = a.prefs.save()
	a.gen++
	a.mu.Unlock()
	return a.Settings(), err
}

// ModelOption is one model the pickers offer.
type ModelOption struct {
	ID       string `json:"id"`
	Context  int    `json:"context"`
	Note     string `json:"note"`
	Unusable string `json:"unusable,omitempty"`
	NoEffort bool   `json:"noEffort,omitempty"`
}

// Models lists what the endpoint serves: the local server's models when
// localMode is set, the API's otherwise, with prices where caveira knows
// them. When abliteration.ai cannot be asked, the models caveira knows
// about are offered anyway.
func (a *App) Models(localMode bool) ([]ModelOption, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if localMode {
		a.mu.Lock()
		base := firstNonEmpty(a.prefs.LocalBaseURL, local.DefaultBaseURL)
		a.mu.Unlock()
		choices, err := local.Choices(ctx, base)
		if err != nil {
			return nil, fmt.Errorf("nothing is answering at %s. Start Ollama with `ollama serve`", base)
		}
		out := make([]ModelOption, len(choices))
		for i, c := range choices {
			out[i] = ModelOption{ID: c.ID, Context: c.Context, Note: c.Note, Unusable: c.Unusable, NoEffort: c.NoEffort}
		}
		return out, nil
	}

	home, _ := os.UserHomeDir()
	cfg, _ := config.Load(home)
	models, err := llm.New(cfg.BaseURL, cfg.APIKey).Models(ctx)
	if err != nil && cfg.BaseURL == config.DefaultBaseURL {
		models, err = nil, nil
		for _, id := range config.KnownModels() {
			models = append(models, llm.ModelInfo{ID: id})
		}
	}
	out := make([]ModelOption, 0, len(models))
	for _, mi := range models {
		o := ModelOption{ID: mi.ID, Context: mi.ContextLength}
		if config.Known(mi.ID) {
			spec := config.Spec(mi.ID)
			if o.Context == 0 {
				o.Context = spec.ContextWindow
			}
			o.Note = fmt.Sprintf("$%s in · $%s out per M", price(spec.InputPerM), price(spec.OutputPerM))
		}
		out = append(out, o)
	}
	return out, err
}

func price(usd float64) string { return strconv.FormatFloat(usd, 'f', -1, 64) }

func keyHint(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return "…" + key[len(key)-2:]
	}
	return key[:3] + "…" + key[len(key)-4:]
}

func unlessDefault(v, def string) string {
	if v == def {
		return ""
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isLocal(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "0.0.0.0"
}

func shortPath(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return dir
}

// gitBranch is the branch checked out in dir, or "" outside a repository.
func gitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
