package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/importer"
)

// Bringing chats over from other agents: ScanImports says what there is,
// Import copies what was picked into ~/.caveira/sessions and adds its
// folders to the projects. See core/importer for how each app is read.

// ImportApp is an agent found on this machine, with its chats by folder.
type ImportApp struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Chats    int             `json:"chats"`
	Projects []ImportProject `json:"projects"`
}

// ImportProject is a folder an app has chats in.
type ImportProject struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Short string `json:"short"`
	Chats int    `json:"chats"`
	// Done is how many of them are in caveira already.
	Done    int    `json:"done"`
	Updated string `json:"updated"`
}

// ImportPick is the folders picked from one app.
type ImportPick struct {
	App  string   `json:"app"`
	Dirs []string `json:"dirs"`
}

// ImportResult is how an import went.
type ImportResult struct {
	Chats    int `json:"chats"`    // brought over now
	Projects int `json:"projects"` // folders added to the projects
	Skipped  int `json:"skipped"`  // there from an earlier import
	Empty    int `json:"empty"`    // nothing said in them
	Failed   int `json:"failed"`
}

// ImportProgress is sent as the "import" event while an import runs.
type ImportProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

func (a *App) ScanImports() []ImportApp {
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	found := importer.Scan(ctx, home)
	a.mu.Lock()
	a.found = found
	empty := a.prefs.emptyImports()
	a.mu.Unlock()

	type key struct{ app, dir string }
	projects := map[key]*ImportProject{}
	latest := map[key]time.Time{}
	for _, c := range found {
		k := key{c.App.ID, c.Dir}
		p := projects[k]
		if p == nil {
			p = &ImportProject{Path: c.Dir, Name: filepath.Base(c.Dir), Short: shortPath(c.Dir)}
			projects[k] = p
		}
		p.Chats++
		if empty[c.SessionID()] || agent.HasSession(c.SessionID()) {
			p.Done++
		}
		if c.Updated.After(latest[k]) {
			latest[k] = c.Updated
			p.Updated = c.Updated.Format(time.RFC3339)
		}
	}
	var out []ImportApp
	for _, app := range importer.Apps {
		ia := ImportApp{ID: app.ID, Name: app.Name}
		for k, p := range projects {
			if k.app == app.ID {
				ia.Projects = append(ia.Projects, *p)
				ia.Chats += p.Chats
			}
		}
		if ia.Chats == 0 {
			continue
		}
		sort.Slice(ia.Projects, func(i, j int) bool { return ia.Projects[i].Updated > ia.Projects[j].Updated })
		out = append(out, ia)
	}
	return out
}

// Import brings over the chats in the folders picked, skipping any
// brought over before, and adds those folders to the projects: after
// the ones already there, most recently worked in first.
func (a *App) Import(picks []ImportPick) (ImportResult, error) {
	a.mu.Lock()
	found := a.found
	empty := a.prefs.emptyImports()
	a.mu.Unlock()
	if found == nil {
		a.ScanImports()
		a.mu.Lock()
		found = a.found
		a.mu.Unlock()
	}

	want := map[string]map[string]bool{}
	for _, p := range picks {
		if want[p.App] == nil {
			want[p.App] = map[string]bool{}
		}
		for _, d := range p.Dirs {
			want[p.App][d] = true
		}
	}
	var res ImportResult
	var todo []importer.Chat
	latest := map[string]time.Time{}
	for _, c := range found {
		if !want[c.App.ID][c.Dir] {
			continue
		}
		if c.Updated.After(latest[c.Dir]) {
			latest[c.Dir] = c.Updated
		}
		if empty[c.SessionID()] || agent.HasSession(c.SessionID()) {
			res.Skipped++
			continue
		}
		todo = append(todo, c)
	}
	var none []string

	done := 0
	a.emit("import", ImportProgress{Total: len(todo)})
	importer.Each(context.Background(), todo, func(c importer.Chat, s *agent.Session, err error) {
		done++
		switch {
		case errors.Is(err, importer.ErrEmpty):
			res.Empty++
			none = append(none, c.SessionID())
		case err != nil:
			res.Failed++
		case s.Save() != nil:
			res.Failed++
		default:
			res.Chats++
		}
		a.emit("import", ImportProgress{Done: done, Total: len(todo)})
	})

	dirs := make([]string, 0, len(latest))
	for d := range latest {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return latest[dirs[i]].After(latest[dirs[j]]) })
	a.mu.Lock()
	defer a.mu.Unlock()
	res.Projects = a.prefs.add(dirs)
	a.prefs.EmptyImports = append(a.prefs.EmptyImports, none...)
	return res, a.prefs.save()
}
