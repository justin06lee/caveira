package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The folder picker: the window types a path, this lists what is there.

// Entry is one thing in a folder.
type Entry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	// Link is a symbolic link; Dir says what it points at.
	Link bool `json:"link,omitempty"`
	// Repo is a folder with a git repository in it.
	Repo bool `json:"repo,omitempty"`
}

// Listing is a folder and everything in it, hidden entries included: the
// window decides what to show.
type Listing struct {
	Path    string  `json:"path"`
	Name    string  `json:"name"`
	Short   string  `json:"short"`
	Entries []Entry `json:"entries"`
}

// repoCheckLimit bounds the .git lookups for one folder: a folder with
// thousands of entries (node_modules) is not a folder of projects.
const repoCheckLimit = 1500

// ListDir lists the folder rel names from base. rel can also be absolute
// or start with ~; an empty base is the home folder.
func (a *App) ListDir(base, rel string) (Listing, error) {
	dir := resolve(base, rel)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return Listing{}, readable(dir, err)
	}
	out := make([]Entry, 0, len(ents))
	for _, e := range ents {
		en := Entry{Name: e.Name(), Dir: e.IsDir()}
		if e.Type()&fs.ModeSymlink != 0 {
			en.Link = true
			if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				en.Dir = st.IsDir()
			}
		}
		if en.Dir && len(ents) <= repoCheckLimit && !guarded(dir, e.Name()) {
			if _, err := os.Stat(filepath.Join(dir, e.Name(), ".git")); err == nil {
				en.Repo = true
			}
		}
		out = append(out, en)
	}
	return Listing{Path: dir, Name: filepath.Base(dir), Short: shortPath(dir), Entries: out}, nil
}

// guarded says whether looking inside dir/name would have macOS ask the
// user for access: the home folders it protects, and every volume. Listing
// the home folder should not set off three permission prompts.
func guarded(dir, name string) bool {
	if dir == "/Volumes" {
		return true
	}
	home, _ := os.UserHomeDir()
	if dir != home {
		return false
	}
	switch name {
	case "Desktop", "Documents", "Downloads", "Library", "Movies", "Music", "Pictures":
		return true
	}
	return false
}

// MakeFolder creates the folder rel names from base and opens it as a
// project.
func (a *App) MakeFolder(base, rel string) (Project, error) {
	if strings.TrimSpace(rel) == "" {
		return Project{}, errors.New("name the folder first")
	}
	dir := resolve(base, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Project{}, readable(dir, err)
	}
	return a.OpenProject(dir)
}

// resolve turns what was typed into an absolute path: ~ is the home
// folder, anything else not absolute is under base.
func resolve(base, rel string) string {
	home, _ := os.UserHomeDir()
	expand := func(p string) string {
		switch {
		case p == "" || p == "~":
			return home
		case strings.HasPrefix(p, "~/"):
			return filepath.Join(home, p[2:])
		case filepath.IsAbs(p):
			return filepath.Clean(p)
		}
		return filepath.Join(home, p)
	}
	if rel == "~" || strings.HasPrefix(rel, "~/") || filepath.IsAbs(rel) {
		return expand(rel)
	}
	return filepath.Join(expand(base), rel)
}

// readable says why a folder cannot be read, in the window's words.
func readable(dir string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("there is no %s", shortPath(dir))
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("caveira is not allowed into %s", shortPath(dir))
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s: %v", shortPath(dir), pe.Err)
	}
	return err
}

// Workspace is the folder projects are opened from.
type Workspace struct {
	Path  string `json:"path"`
	Short string `json:"short"`
}

// SetWorkspace makes dir the folder the project picker opens in. An empty
// dir clears it, and the picker opens in the home folder.
func (a *App) SetWorkspace(dir string) (Workspace, error) {
	if dir != "" {
		dir = resolve("", dir)
		st, err := os.Stat(dir)
		if err != nil {
			return Workspace{}, readable(dir, err)
		}
		if !st.IsDir() {
			return Workspace{}, fmt.Errorf("%s is not a folder", shortPath(dir))
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.Workspace = dir
	return workspace(dir), a.prefs.save()
}

func workspace(dir string) Workspace {
	if dir == "" {
		return Workspace{}
	}
	return Workspace{Path: dir, Short: shortPath(dir)}
}

// SuggestWorkspace guesses where projects live: the folder most of the
// known ones share, if two or more do.
func (a *App) SuggestWorkspace() Workspace {
	a.mu.Lock()
	dirs := a.prefs.existingProjects()
	a.mu.Unlock()
	count := map[string]int{}
	for _, d := range dirs {
		count[filepath.Dir(d)]++
	}
	var parents []string
	for p, n := range count {
		if n >= 2 && p != "/" {
			parents = append(parents, p)
		}
	}
	if len(parents) == 0 {
		return Workspace{}
	}
	sort.Slice(parents, func(i, j int) bool {
		if count[parents[i]] != count[parents[j]] {
			return count[parents[i]] > count[parents[j]]
		}
		return parents[i] < parents[j]
	})
	return workspace(parents[0])
}

// FinishOnboarding marks the first-run steps done, so the window opens
// straight into work from now on.
func (a *App) FinishOnboarding() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs.Onboarded = true
	return a.prefs.save()
}
