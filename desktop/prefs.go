package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/justin06lee/caveira/core/config"
)

// prefs is what only the desktop app keeps, in ~/.caveira/desktop.json.
// The model, effort, and confirm setting live in config.json, which the
// terminal client reads too.
type prefs struct {
	// Projects are folders opened before, most recent first.
	Projects []string `json:"projects,omitempty"`
	// Theme is "system", "light", or "dark".
	Theme string `json:"theme,omitempty"`
	// Workspace is the folder the project picker opens in.
	Workspace string `json:"workspace,omitempty"`
	// Onboarded is set once the first-run steps (import, workspace) are
	// done or skipped.
	Onboarded bool `json:"onboarded,omitempty"`
	// Plan is the caveira plan picked, "" for none (see plans.go).
	Plan string `json:"plan,omitempty"`
	// EmptyImports are chats an import found nothing in, by the id they
	// would have had, so the next look does not offer them again.
	EmptyImports []string `json:"empty_imports,omitempty"`
}

// maxProjects is room for everything an import brings in; the sidebar
// scrolls.
const maxProjects = 40

func prefsPath() string { return filepath.Join(config.Dir(), "desktop.json") }

func loadPrefs() prefs {
	var p prefs
	b, err := os.ReadFile(prefsPath())
	if err == nil {
		_ = json.Unmarshal(b, &p)
	}
	if p.Theme == "" {
		p.Theme = "system"
	}
	return p
}

func (p prefs) save() error {
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := prefsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, prefsPath())
}

// touch moves dir to the front of the recent projects.
func (p *prefs) touch(dir string) {
	out := []string{dir}
	for _, d := range p.Projects {
		if d != dir && len(out) < maxProjects {
			out = append(out, d)
		}
	}
	p.Projects = out
}

// add puts dirs after the projects already known, in the order given,
// and says how many were new.
func (p *prefs) add(dirs []string) int {
	known := map[string]bool{}
	for _, d := range p.Projects {
		known[d] = true
	}
	n := 0
	for _, d := range dirs {
		if !known[d] && len(p.Projects) < maxProjects {
			p.Projects = append(p.Projects, d)
			known[d] = true
			n++
		}
	}
	return n
}

func (p prefs) emptyImports() map[string]bool {
	out := make(map[string]bool, len(p.EmptyImports))
	for _, id := range p.EmptyImports {
		out[id] = true
	}
	return out
}

func (p *prefs) forget(dir string) {
	out := p.Projects[:0]
	for _, d := range p.Projects {
		if d != dir {
			out = append(out, d)
		}
	}
	p.Projects = out
}

// existingProjects is the recent folders that are there right now. One on
// a volume that is not mounted is left out but not forgotten.
func (p prefs) existingProjects() []string {
	var out []string
	for _, d := range p.Projects {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}
