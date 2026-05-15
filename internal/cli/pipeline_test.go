package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestPipeline_LinearDryRun runs the full pipeline against a temp git repo
// produced by `git init` + a series of commits via the actual git binary. We
// use the binary here instead of go-git's memfs because the pipeline reads
// from a filesystem path.
func TestPipeline_LinearDryRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	for i := 0; i < 3; i++ {
		f := filepath.Join(repo, "f.txt")
		_ = os.WriteFile(f, []byte(string(rune('a'+i))+"\n"), 0644)
		mustGit(t, repo, "add", "f.txt")
		mustGit(t, repo, "commit", "-m", "c"+string(rune('a'+i)))
	}

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 13:00",
		"--window-tz", "UTC",
		"--dry-run",
		"--seed", "1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected dry-run exit 0, got %d; stderr=%s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Span:")) {
		t.Fatalf("expected dry-run output to include Span; got: %s", out.String())
	}
	_ = time.Now
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
