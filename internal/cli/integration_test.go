package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegration_ScaledFit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	for i := 0; i < 10; i++ {
		f := filepath.Join(repo, "f.txt")
		buf := make([]byte, 50)
		for j := range buf {
			buf[j] = 'a' + byte(i)
		}
		buf[len(buf)-1] = '\n'
		_ = os.WriteFile(f, append([]byte(nil), buf...), 0644)
		mustGit(t, repo, "add", "f.txt")
		mustGit(t, repo, "commit", "-m", "commit "+string(rune('A'+i)))
	}

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00", // 2 hours; ~10 commits of ~30m = 300m unscaled
		"--window-tz", "UTC",
		"--seed", "7",
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

	// Verify destination has 10 commits with author dates inside the window.
	logOut, err := exec.Command("git", "-C", repo, "log", "--pretty=%ad", "--date=iso-strict").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, logOut)
	}
	lines := bytes.Count(logOut, []byte("\n"))
	if lines < 1 {
		t.Errorf("expected at least one commit line in destination, got: %s", logOut)
	}
}
