package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/input"
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
		"--fabricate",
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
		"--fabricate",
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

// makeFixtureRepoDir builds a real on-disk git repo with one commit and
// returns its path.
func makeFixtureRepoDir(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	if err := os.MkdirAll(filepath.Join(repo, "internal/walk"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal/walk/load.go"), []byte("package walk\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")
	return repo
}

func TestIntegration_Fabricate_DropsSourceBranches(t *testing.T) {
	src := makeFixtureRepoDir(t)
	// An extra branch in the source repo that is NOT part of the
	// fabricated plan; it must be gone after fabrication.
	mustGit(t, src, "branch", "some-old-branch")

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		WindowTZ:  time.UTC,
		Fabricate: true,
		Seed:      1,
		HasSeed:   true,
	}
	var out, errOut bytes.Buffer
	code := Pipeline(cfg, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	refsOut, err := exec.Command("git", "-C", src, "for-each-ref", "--format=%(refname)").CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref: %v\n%s", err, refsOut)
	}
	if bytes.Contains(refsOut, []byte("some-old-branch")) {
		t.Errorf("source branch some-old-branch still present after fabricate:\n%s", refsOut)
	}
	if !bytes.Contains(refsOut, []byte("refs/heads/")) {
		t.Errorf("expected at least one fabricated branch ref:\n%s", refsOut)
	}
}

func TestIntegration_FabricatePigs_SquashesToFitTinyWindow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	// Enough files across distinct directories that flurry produces several
	// commits, so the tiny window forces the scheduler to squash.
	for _, d := range []string{"a", "b", "c", "d", "e"} {
		if err := os.MkdirAll(filepath.Join(repo, d), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, d, "x.go"), []byte("package "+d+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, d, "x_test.go"), []byte("package "+d+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")

	// A deliberately tiny window: 30 minutes. Flurry produces ~13 commits
	// that need roughly two hours un-squashed, so the scheduler must squash
	// heavily (down to ~3 commits) but the squashed history still fits.
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cfg := &input.Config{
		Repo:          repo,
		Start:         start,
		End:           start.Add(30 * time.Minute),
		WindowTZ:      time.UTC,
		Fabricate:     true,
		PigsN:         2,
		PigIdentities: []string{"Alice <a@x.com>", "Bob <b@x.com>"},
		Seed:          1,
		HasSeed:       true,
	}
	var out, errOut bytes.Buffer
	code := Pipeline(cfg, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected pigs run to squash and succeed (exit 0), got exit %d; stderr=%s", code, errOut.String())
	}

	// verifyTreeMatch already ran inside Pipeline, so a passing exit means the
	// squashed history reproduced the source tree exactly.
	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, logOut)
	}
	if len(bytes.TrimSpace(logOut)) == 0 {
		t.Fatalf("result repo has no commits:\n%s", logOut)
	}
	n := bytes.Count(bytes.TrimSpace(logOut), []byte("\n")) + 1
	// Flurry produces ~13 commits un-squashed; the tiny window must have
	// squashed the history down to noticeably fewer.
	if n >= 13 {
		t.Errorf("expected squashing to reduce the commit count below the un-squashed ~13, got %d", n)
	}
	t.Logf("squashed pigs history has %d commit(s)", n)
}
