package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/input"
)

// installFakeClaude writes a `claude` binary on PATH that ignores its stdin
// prompt and prints planJSON, so --claude-code runs deterministically offline.
func installFakeClaude(t *testing.T, planJSON string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'PLAN'\n" + planJSON + "\nPLAN\n"
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fixtureFiles returns the tracked file paths in repoDir's HEAD tree, in the
// same order allFilesPlan walks them.
func fixtureFiles(t *testing.T, repoDir string) []string {
	t.Helper()
	r, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	files := tree.Files()
	for {
		f, err := files.Next()
		if err != nil {
			break
		}
		names = append(names, f.Name)
	}
	return names
}

// allFilesPlan builds a one-commit-per-file "all segments" plan for repoDir.
func allFilesPlan(t *testing.T, repoDir string) string {
	t.Helper()
	plan := `{"commits":[`
	for i, name := range fixtureFiles(t, repoDir) {
		if i > 0 {
			plan += ","
		}
		plan += `{"message":"feat: add ` + name + `","type":"feat","changes":[{"path":"` +
			name + `","segments":"all"}]}`
	}
	plan += `]}`
	return plan
}

func TestIntegration_LLM_ClaudeCode_SingleAuthor(t *testing.T) {
	src := makeFixtureRepoDir(t) // reuse the existing integration helper
	files := fixtureFiles(t, src)
	plan := allFilesPlan(t, src)
	installFakeClaude(t, plan)

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-6 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		Provider:  "claude-code",
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	// The fake-claude plan supplies exactly one `feat: add <path>` commit per
	// fixture file. The rewritten history must reflect that exact plan: no
	// fewer commits (squash guard collapsed them) and no default fabrication.
	subjects := gitLogSubjects(t, src)
	if len(subjects) != len(files) {
		t.Fatalf("expected %d commits from the LLM plan, got %d:\n%s",
			len(files), len(subjects), strings.Join(subjects, "\n"))
	}
	for _, name := range files {
		want := "feat: add " + name
		found := false
		for _, s := range subjects {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing planned commit %q in rewritten log:\n%s",
				want, strings.Join(subjects, "\n"))
		}
	}
}

// makeMultiFeatureRepoDir builds a real on-disk git repo whose tracked files
// span two distinct non-root feature directories (internal/walk and
// internal/cli) plus a root file. reshapeRats assigns rat := ids[fi%len(ids)]
// per feature run, so two feature runs are required for both rats to author
// commits — the single-feature shared fixture would put everything on one rat.
func makeMultiFeatureRepoDir(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	for _, sub := range []string{"internal/walk", "internal/cli"} {
		if err := os.MkdirAll(filepath.Join(repo, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"go.mod":                "module x\n",
		"internal/walk/load.go": "package walk\n",
		"internal/cli/main.go":  "package cli\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")
	return repo
}

func TestIntegration_LLM_ClaudeCode_WithRats(t *testing.T) {
	src := makeMultiFeatureRepoDir(t)
	plan := allFilesPlan(t, src)
	installFakeClaude(t, plan)

	cfg := &input.Config{
		Repo:          src,
		Start:         time.Now().Add(-12 * time.Hour),
		End:           time.Now(),
		WindowTZ:      time.UTC,
		Fabricate:     true,
		Provider:      "claude-code",
		RatsN:         2,
		RatIdentities: []string{"Rat One <r1@x.com>", "Rat Two <r2@x.com>"},
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	// Rats mode must distribute commits across both rat identities. Use --all
	// so commits on feature branches are counted, not just master.
	authors, err := exec.Command("git", "-C", src, "log", "--all", "--pretty=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log authors: %v: %s", err, authors)
	}
	if !bytes.Contains(authors, []byte("r1@x.com")) || !bytes.Contains(authors, []byte("r2@x.com")) {
		t.Errorf("expected both rat authors r1@x.com and r2@x.com in log:\n%s", authors)
	}

	// Rats mode produces feature branches and merges. Require evidence of at
	// least one feat/ branch or one merge commit.
	branches, err := exec.Command("git", "-C", src, "branch", "--list", "feat/*").CombinedOutput()
	if err != nil {
		t.Fatalf("git branch: %v: %s", err, branches)
	}
	merges, err := exec.Command("git", "-C", src, "log", "--all", "--merges", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log merges: %v: %s", err, merges)
	}
	if len(bytes.TrimSpace(branches)) == 0 && len(bytes.TrimSpace(merges)) == 0 {
		t.Errorf("expected rats mode to produce a feat/ branch or a merge commit; got none\nbranches:\n%s\nmerges:\n%s",
			branches, merges)
	}
}

// gitLogSubjects returns every commit subject in repoDir's HEAD history.
func gitLogSubjects(t *testing.T, repoDir string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "log", "--pretty=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	var subjects []string
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) > 0 {
			subjects = append(subjects, string(line))
		}
	}
	return subjects
}
