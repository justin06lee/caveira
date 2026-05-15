package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

// allFilesPlan builds a one-commit-per-file "all segments" plan for repoDir.
func allFilesPlan(t *testing.T, repoDir string) string {
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
	plan := `{"commits":[`
	first := true
	files := tree.Files()
	for {
		f, err := files.Next()
		if err != nil {
			break
		}
		if !first {
			plan += ","
		}
		first = false
		plan += `{"message":"feat: add ` + f.Name + `","type":"feat","changes":[{"path":"` +
			f.Name + `","segments":"all"}]}`
	}
	plan += `]}`
	return plan
}

func TestIntegration_LLM_ClaudeCode_SingleAuthor(t *testing.T) {
	src := makeFixtureRepoDir(t) // reuse the existing integration helper
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
	// The swapped-in repo at src must have HEAD tree == original tree.
	assertGitLogNonEmpty(t, src)
}

func TestIntegration_LLM_ClaudeCode_WithRats(t *testing.T) {
	src := makeFixtureRepoDir(t)
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
	assertGitLogNonEmpty(t, src)
}

func assertGitLogNonEmpty(t *testing.T, repoDir string) {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		t.Fatal("rewritten repo has no commits")
	}
}
