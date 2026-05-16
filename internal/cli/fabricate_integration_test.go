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

func TestIntegration_Fabricate_NoModels_NoCoAuthors(t *testing.T) {
	src := makeFixtureRepoDir(t) // fixture history has no AI-model identities
	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-30 * 24 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}
	logOut, err := exec.Command("git", "-C", src, "log", "--format=%B").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, logOut)
	}
	if bytes.Contains(logOut, []byte("Co-Authored-By")) {
		t.Fatalf("no models in source, but output has Co-Authored-By trailers:\n%s", logOut)
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

func TestIntegration_Fabricate_ModelBecomesCoAuthor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "repo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", src}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com",
			"GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=alice@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.name", "Alice")
	run("config", "user.email", "alice@example.com")
	// Several files across two feature dirs so flurry yields multiple commits.
	for _, f := range []string{"README.md", "internal/walk/load.go", "internal/cli/main.go"} {
		p := filepath.Join(src, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", f)
		run("commit", "-m", "add "+f+"\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	}

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-30 * 24 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		PigsN:     1,
		Seed:      7,
		HasSeed:   true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	bodies, err := exec.Command("git", "-C", src, "log", "--all", "--format=%B").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, bodies)
	}
	if !bytes.Contains(bodies, []byte("Co-Authored-By: Claude <noreply@anthropic.com>")) {
		t.Fatalf("expected Claude co-author trailers in fabricated history:\n%s", bodies)
	}

	authors, err := exec.Command("git", "-C", src, "log", "--all", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log authors: %v: %s", err, authors)
	}
	if bytes.Contains(authors, []byte("noreply@anthropic.com")) {
		t.Fatalf("Claude appeared as a commit author; it must only be a co-author:\n%s", authors)
	}
	if !bytes.Contains(authors, []byte("alice@example.com")) {
		t.Fatalf("expected Alice as the fabricated author:\n%s", authors)
	}
}

func TestIntegration_Fabricate_MailmapUnifies(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "repo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	commit := func(name, email, file string) {
		t.Helper()
		p := filepath.Join(src, file)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", file}, {"commit", "-m", "add " + file}} {
			cmd := exec.Command("git", append([]string{"-C", src}, args...)...)
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email,
				"GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
	}
	initCmd := exec.Command("git", "-C", src, "init")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// Same person, two emails, across two feature dirs.
	commit("Jay", "jay@personal.com", "internal/walk/load.go")
	commit("jay06", "jay@work.com", "internal/cli/main.go")
	// .mailmap unifies the two emails into one identity.
	if err := os.WriteFile(filepath.Join(src, ".mailmap"),
		[]byte("Jay <jay@personal.com> <jay@work.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-30 * 24 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		RatsN:     1,
		Seed:      5,
		HasSeed:   true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	emails, err := exec.Command("git", "-C", src, "log", "--all", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, emails)
	}
	if bytes.Contains(emails, []byte("jay@work.com")) {
		t.Fatalf("non-canonical email leaked into fabricated history:\n%s", emails)
	}
	if !bytes.Contains(emails, []byte("jay@personal.com")) {
		t.Fatalf("expected the canonical email as the fabricated author:\n%s", emails)
	}
}
