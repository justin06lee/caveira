package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegration_FabricateFlurry_SingleAuthor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	_ = os.MkdirAll(filepath.Join(repo, "internal/walk"), 0755)
	_ = os.MkdirAll(filepath.Join(repo, "internal/cli"), 0755)
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "internal/walk/load.go"), []byte("package walk\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "internal/cli/cli.go"), []byte("package cli\n"), 0644)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--flurry",
		"--seed", "1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	deadPath := repo + ".dead"
	if _, err := os.Stat(deadPath); err != nil {
		t.Errorf("expected %s to exist after run", deadPath)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Errorf("expected %s (rewritten) to exist after run", repo)
	}

	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, logOut)
	}
	if !bytes.Contains(logOut, []byte("chore")) {
		t.Errorf("expected chore commit in destination log:\n%s", logOut)
	}
	if !bytes.Contains(logOut, []byte("feat(walk)")) {
		t.Errorf("expected feat(walk) commit in destination log:\n%s", logOut)
	}
}

func TestIntegration_FabricatePigs_TwoAuthors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	_ = os.MkdirAll(filepath.Join(repo, "a"), 0755)
	_ = os.MkdirAll(filepath.Join(repo, "b"), 0755)
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "a/x.go"), []byte("package a\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "b/y.go"), []byte("package b\n"), 0644)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--flurry",
		"--pigs", "2",
		"--pig", "Alice <a@x.com>",
		"--pig", "Bob <b@x.com>",
		"--seed", "1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	logOut, _ := exec.Command("git", "-C", repo, "log", "--pretty=%an %ae").CombinedOutput()
	if !bytes.Contains(logOut, []byte("a@x.com")) || !bytes.Contains(logOut, []byte("b@x.com")) {
		t.Errorf("expected both authors in destination log:\n%s", logOut)
	}
}
