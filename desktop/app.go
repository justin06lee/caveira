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
	// found is what the last look for other agents' chats found.
	found []importer.Chat
	// namer names a new chat from its first message; nil leaves chats
	// with that message's first line, as the tests do.
	namer func(ctx context.Context, client *llm.Client, first string) (string, error)
}

func NewApp(version string) *App {
	return &App{version: version, prefs: loadPrefs(), chats: map[string]*chat{}, namer: nameChat}
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

// TitleBarDoubleClick does what a double-click on a window's title bar
// does: zooms the window, or back. A Mac set to minimize instead, or to do
// nothing, gets that.
func (a *App) TitleBarDoubleClick() {
	if a.ctx == nil {
		return
	}
	switch doubleClickAction() {
	case "Minimize":
		runtime.WindowMinimise(a.ctx)
	case "None":
	default:
		runtime.WindowToggleMaximise(a.ctx)
	}
}

// doubleClickAction is the Mac's "Double-click a window's title bar to"
// setting: Maximize (Zoom), Fill, Minimize, or None; "" when unset, which
// is Zoom, and elsewhere.
func doubleClickAction() string {
	if goruntime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleActionOnDoubleClick").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	// Plan is the plan this install is on, "" for none, and Plans what
	// there is to pick from.
	Plan  string `json:"plan"`
	Plans []Plan `json:"plans"`
}

func (a *App) Boot() Boot {
	waitShellEnv()
	a.mu.Lock()
	dirs := a.prefs.existingProjects()
	ws, onboarded, plan := a.prefs.Workspace, a.prefs.Onboarded, a.prefs.Plan
	a.mu.Unlock()
	return Boot{
		Version: a.version, Settings: a.Settings(), Projects: projects(dirs), Workspace: workspace(ws),
		Onboarded: onboarded, Plan: plan, Plans: plans,
	}
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

// SettingsView is the settings screen's state. The app runs only on
// abliteration.ai, and the key is not the user's to set: it comes from
// the environment, a .env file, or config.json, where caveira's plans
// will put it.
type SettingsView struct {
	Model   string   `json:"model"`
	Effort  string   `json:"effort"`
	Confirm bool     `json:"confirm"`
	Theme   string   `json:"theme"`
	Efforts []string `json:"efforts"`
	// Ready is false when a chat could not run: there is no key.
	Ready bool `json:"ready"`
}

func (a *App) Settings() SettingsView {
	home, _ := os.UserHomeDir()
	cfg, _ := config.Load(home)
	a.mu.Lock()
	p := a.prefs
	a.mu.Unlock()
	return SettingsView{
		Model:   cfg.Model,
		Effort:  cfg.ReasoningEffort,
		Confirm: cfg.Confirm,
		Theme:   p.Theme,
		Efforts: config.Efforts,
		Ready:   cfg.APIKey != "" || isLocal(cfg.BaseURL),
	}
}

// SettingsInput is a change from the settings screen.
type SettingsInput struct {
	Model   string `json:"model"`
	Effort  string `json:"effort"`
	Confirm bool   `json:"confirm"`
	Theme   string `json:"theme"`
}

func (a *App) SaveSettings(in SettingsInput) (SettingsView, error) {
	if in.Effort != "" && !config.ValidEffort(in.Effort) {
		return a.Settings(), fmt.Errorf("reasoning effort must be one of %s", strings.Join(config.Efforts, ", "))
	}
	file, err := config.ReadFile()
	if err != nil {
		return a.Settings(), err
	}
	file.Model = unlessDefault(strings.TrimSpace(in.Model), config.DefaultModel)
	file.ReasoningEffort = in.Effort
	file.Confirm = in.Confirm
	if err := config.Save(file); err != nil {
		return a.Settings(), err
	}

	a.mu.Lock()
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
	ID string `json:"id"`
	// Name is what the model is called on screen, under the heading
	// Group, beside the Logos of who makes it.
	Name     string   `json:"name"`
	Group    string   `json:"group,omitempty"`
	Logos    []string `json:"logos,omitempty"`
	Context  int      `json:"context"`
	Note     string   `json:"note"`
	Unusable string   `json:"unusable,omitempty"`
	NoEffort bool     `json:"noEffort,omitempty"`
	// Plan is the cheapest plan that runs it, when Free does not.
	Plan string `json:"plan,omitempty"`
}

// Models lists the models caveira offers on abliteration.ai. An endpoint
// set in the environment instead (a test server, say) is asked what it
// serves, with prices where caveira knows them.
func (a *App) Models() ([]ModelOption, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	home, _ := os.UserHomeDir()
	cfg, _ := config.Load(home)
	if isCatalog(cfg.BaseURL) {
		return catalogOptions(), nil
	}
	models, err := llm.New(cfg.BaseURL, cfg.APIKey).Models(ctx)
	out := make([]ModelOption, 0, len(models))
	for _, mi := range models {
		o := ModelOption{ID: mi.ID, Name: mi.ID, Context: mi.ContextLength}
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

func unlessDefault(v, def string) string {
	if v == def {
		return ""
	}
	return v
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
