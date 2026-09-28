package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin06lee/caveira/core/importer"
)

func TestListDirSeesWhatLsSees(t *testing.T) {
	a, dir := testApp(t, nil)
	for _, p := range []string{"repo/.git", "plain", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "plain"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	l, err := a.ListDir(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Entry{}
	for _, e := range l.Entries {
		got[e.Name] = e
	}
	if len(got) != 5 {
		t.Fatalf("want a.txt, repo, plain, .hidden, link; got %+v", l.Entries)
	}
	if got["a.txt"].Dir || !got["repo"].Repo || got["plain"].Repo || !got[".hidden"].Dir {
		t.Errorf("kinds: %+v", got)
	}
	if e := got["link"]; !e.Link || !e.Dir {
		t.Errorf("a link to a folder: %+v", e)
	}

	if l, err := a.ListDir(dir, "repo/../plain"); err != nil || l.Path != filepath.Join(dir, "plain") || l.Name != "plain" {
		t.Errorf("relative path: %+v %v", l, err)
	}
	home, _ := os.UserHomeDir()
	if l, err := a.ListDir(dir, "~"); err != nil || l.Path != home || l.Short != "~" {
		t.Errorf("~ is home: %+v %v", l, err)
	}
	if l, err := a.ListDir("", dir); err != nil || l.Path != dir {
		t.Errorf("an absolute path ignores the base: %+v %v", l, err)
	}
	if _, err := a.ListDir(dir, "nope"); err == nil || !strings.Contains(err.Error(), "there is no") {
		t.Errorf("a missing folder: %v", err)
	}
}

func TestMakeFolderOpensIt(t *testing.T) {
	a, dir := testApp(t, nil)
	p, err := a.MakeFolder(dir, "new/thing")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != filepath.Join(dir, "new", "thing") || p.Name != "thing" {
		t.Errorf("project: %+v", p)
	}
	if b := a.Boot(); len(b.Projects) != 1 || b.Projects[0].Path != p.Path {
		t.Errorf("not in the projects: %+v", b.Projects)
	}
	if _, err := a.MakeFolder(dir, " "); err == nil {
		t.Error("made a folder with no name")
	}
}

func TestWorkspaceAndOnboardingOutliveTheApp(t *testing.T) {
	a, dir := testApp(t, nil)
	if b := a.Boot(); b.Onboarded || b.Workspace.Path != "" {
		t.Fatalf("a new install: %+v", b)
	}
	if _, err := a.SetWorkspace(filepath.Join(dir, "a.txt")); err == nil {
		t.Error("a file became the workspace")
	}
	ws, err := a.SetWorkspace(dir)
	if err != nil || ws.Path != dir {
		t.Fatalf("%+v %v", ws, err)
	}
	if err := a.FinishOnboarding(); err != nil {
		t.Fatal(err)
	}
	b := NewApp("test").Boot()
	if !b.Onboarded || b.Workspace.Path != dir {
		t.Errorf("after a restart: %+v", b)
	}
}

func TestSuggestWorkspaceIsWhereProjectsCluster(t *testing.T) {
	a, dir := testApp(t, nil)
	if ws := a.SuggestWorkspace(); ws.Path != "" {
		t.Errorf("nothing to go on, but suggested %+v", ws)
	}
	for _, p := range []string{"code/a", "code/b", "elsewhere/c"} {
		if _, err := a.MakeFolder(dir, p); err != nil {
			t.Fatal(err)
		}
	}
	if ws := a.SuggestWorkspace(); ws.Path != filepath.Join(dir, "code") {
		t.Errorf("suggested %+v", ws)
	}
}

func TestImportBringsChatsAndProjectsOnce(t *testing.T) {
	a, dir := testApp(t, nil)
	old := importer.Scratch
	importer.Scratch = nil
	t.Cleanup(func() { importer.Scratch = old })

	home, _ := os.UserHomeDir()
	chat := filepath.Join(home, ".claude", "projects", "-proj", "s1.jsonl")
	if err := os.MkdirAll(filepath.Dir(chat), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range []map[string]any{
		{"type": "user", "uuid": "u", "cwd": dir, "timestamp": "2026-09-01T10:00:00Z", "message": map[string]any{"role": "user", "content": "read a.txt"}},
		{"type": "assistant", "uuid": "a", "parentUuid": "u", "cwd": dir, "message": map[string]any{"id": "m", "role": "assistant",
			"content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": "Read", "input": map[string]any{"file_path": filepath.Join(dir, "a.txt")}}}}},
		{"type": "user", "uuid": "r", "parentUuid": "a", "cwd": dir, "message": map[string]any{"role": "user",
			"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "hi there"}}}},
		{"type": "assistant", "uuid": "b", "parentUuid": "r", "cwd": dir, "message": map[string]any{"id": "n", "role": "assistant",
			"content": []any{map[string]any{"type": "text", "text": "It says hi."}}}},
		{"type": "ai-title", "aiTitle": "Read the file"},
	} {
		b, _ := json.Marshal(l)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(chat, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	apps := a.ScanImports()
	if len(apps) != 1 || apps[0].ID != "claude" || len(apps[0].Projects) != 1 || apps[0].Projects[0].Path != dir || apps[0].Projects[0].Done != 0 {
		t.Fatalf("scan: %+v", apps)
	}
	var progress []ImportProgress
	a.sink = func(name string, data any) {
		if p, ok := data.(ImportProgress); ok && name == "import" {
			progress = append(progress, p)
		}
	}
	res, err := a.Import([]ImportPick{{App: "claude", Dirs: []string{dir}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chats != 1 || res.Projects != 1 || res.Failed != 0 {
		t.Errorf("result: %+v", res)
	}
	if n := len(progress); n != 2 || progress[n-1] != (ImportProgress{Done: 1, Total: 1}) {
		t.Errorf("progress: %+v", progress)
	}

	headers, err := a.Chats(dir)
	if err != nil || len(headers) != 1 || headers[0].ID != "claude-s1" || headers[0].Title != "Read the file" {
		t.Fatalf("chats: %+v %v", headers, err)
	}
	v, err := a.OpenChat("claude-s1")
	if err != nil {
		t.Fatal(err)
	}
	if k := kinds(v.Items); k != "notice user tool:Read:done assistant" {
		t.Errorf("transcript: %s", k)
	}
	if v.Items[0].Text != "Imported from Claude Code." {
		t.Errorf("notice: %q", v.Items[0].Text)
	}
	if tool := v.Items[2].Tool; tool.Label != "Read" || tool.Summary != "1 lines" || !strings.HasSuffix(tool.Preview, "a.txt") {
		t.Errorf("the Read call is caveira's read_file: %+v", tool)
	}

	// A chat with nothing said in it is looked at once.
	blank := filepath.Join(home, ".claude", "projects", "-proj", "s2.jsonl")
	b, _ := json.Marshal(map[string]any{"type": "user", "uuid": "x", "cwd": dir, "isMeta": true, "message": map[string]any{"role": "user", "content": "meta"}})
	if err := os.WriteFile(blank, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := a.ScanImports()[0].Projects[0]; p.Chats != 2 || p.Done != 1 {
		t.Fatalf("with a blank chat: %+v", p)
	}
	if res, err := a.Import([]ImportPick{{App: "claude", Dirs: []string{dir}}}); err != nil || res.Empty != 1 || res.Skipped != 1 {
		t.Fatalf("blank import: %+v %v", res, err)
	}

	if p := a.ScanImports()[0].Projects[0]; p.Done != 2 {
		t.Error("the scan does not know it was imported")
	}
	again, err := a.Import([]ImportPick{{App: "claude", Dirs: []string{dir}}})
	if err != nil || again.Chats != 0 || again.Skipped != 2 || again.Empty != 0 || again.Projects != 0 {
		t.Errorf("importing twice: %+v %v", again, err)
	}
}
