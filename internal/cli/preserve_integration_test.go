package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/input"
)

// makePreserveRepo builds a temp git repo of n commits with deliberately
// varied diff sizes, so the scheduler sees a spread of difficulties (and thus
// gaps). Returns the repo path.
func makePreserveRepo(t *testing.T, n int) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	for i := 0; i < n; i++ {
		// Grow the file by an increasing number of lines to vary difficulty.
		lines := 1 + (i%5)*8
		var b strings.Builder
		for j := 0; j < lines; j++ {
			b.WriteString("line ")
			b.WriteString(strconv.Itoa(i))
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(j))
			b.WriteByte('\n')
		}
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(b.String()), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		mustGit(t, repo, "add", "f.txt")
		mustGit(t, repo, "commit", "-m", "commit "+strconv.Itoa(i))
	}
	return repo
}

var oidLineRe = regexp.MustCompile(`(?m)^[0-9a-f]{7}\s`)

// TestIntegration_Preserve_KeepsAllCommitsInNarrowWindow is the headline
// guarantee: in a window far too narrow for normal mode (which would squash),
// --preserve keeps every commit, ordered, inside the window.
func TestIntegration_Preserve_KeepsAllCommitsInNarrowWindow(t *testing.T) {
	const n = 12
	repo := makePreserveRepo(t, n)

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 13:00:00",
		"--end", "2026-05-14 13:10:00", // 10 minutes for 12 commits
		"--window-tz", "UTC",
		"--preserve",
		"--seed", "7",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("12 -> 12 (0 squashed)")) {
		t.Errorf("expected all 12 commits preserved with 0 squashes; summary:\n%s", out.String())
	}

	// Re-read the rewritten repo and check every author date is inside the
	// window and strictly increasing in commit order.
	start := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 14, 13, 10, 0, 0, time.UTC)
	times := commitAuthorUnix(t, repo)
	if len(times) != n {
		t.Fatalf("expected %d commits in rewritten repo, got %d", n, len(times))
	}
	prev := int64(-1)
	for i, ts := range times {
		if ts < start.Unix() || ts > end.Unix() {
			t.Errorf("commit %d author time %d outside window [%d,%d]", i, ts, start.Unix(), end.Unix())
		}
		if ts <= prev {
			t.Errorf("commit %d author time %d not after previous %d", i, ts, prev)
		}
		prev = ts
	}
}

// TestIntegration_Default_CannotKeepAllInNarrowWindow documents the contrast:
// for the same narrow window where --preserve keeps all 12 commits, default
// mode cannot — it either squashes commits away or fails outright. Either way
// it never produces a lossless "12 -> 12 (0 squashed)".
func TestIntegration_Default_CannotKeepAllInNarrowWindow(t *testing.T) {
	repo := makePreserveRepo(t, 12)

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 13:00:00",
		"--end", "2026-05-14 13:10:00",
		"--window-tz", "UTC",
		"--seed", "7",
	}, &out, &errOut)
	if code == 0 && bytes.Contains(out.Bytes(), []byte("12 -> 12 (0 squashed)")) {
		t.Errorf("default mode unexpectedly kept all 12 commits losslessly:\n%s", out.String())
	}
	if code == 0 && bytes.Contains(out.Bytes(), []byte("(0 squashed)")) {
		t.Errorf("default mode should have squashed in this narrow window:\n%s", out.String())
	}
}

// TestIntegration_Preserve_DryRunReportsNoSquashes checks the dry-run report:
// every commit row is present, the footer shows 0 squashes and a sub-1 scale,
// and nothing is written (no .dead created).
func TestIntegration_Preserve_DryRunReportsNoSquashes(t *testing.T) {
	const n = 12
	repo := makePreserveRepo(t, n)

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 13:00:00",
		"--end", "2026-05-14 13:10:00",
		"--window-tz", "UTC",
		"--preserve",
		"--dry-run",
		"--seed", "3",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Squashes: 0.")) {
		t.Errorf("expected 'Squashes: 0.' in dry-run footer:\n%s", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("Scale: s=1.00")) {
		t.Errorf("expected a scaled-down (s<1.00) schedule for this narrow window:\n%s", out.String())
	}
	if rows := len(oidLineRe.FindAll(out.Bytes(), -1)); rows != n {
		t.Errorf("expected %d commit rows in dry-run table, got %d:\n%s", n, rows, out.String())
	}
	if _, err := os.Stat(repo + ".dead"); err == nil {
		t.Errorf("dry-run must not create %s.dead", repo)
	}
}

// TestIntegration_Preserve_AlreadyFitsNoScaling verifies a comfortably wide
// window keeps all commits at full scale (s=1.00) with no squashing.
func TestIntegration_Preserve_AlreadyFitsNoScaling(t *testing.T) {
	repo := makePreserveRepo(t, 12)

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 09:00:00",
		"--end", "2026-05-14 23:00:00", // 14h, easily fits ~12 small commits
		"--window-tz", "UTC",
		"--preserve",
		"--dry-run",
		"--seed", "5",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Scale: s=1.00")) {
		t.Errorf("expected s=1.00 for a wide window:\n%s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Squashes: 0.")) {
		t.Errorf("expected 0 squashes for a wide window:\n%s", out.String())
	}
}

// TestIntegration_Preserve_FailsImpossiblyNarrowWindow verifies preserve refuses
// (rather than merging) when the window can't hold one second per commit, and
// leaves the source untouched.
func TestIntegration_Preserve_FailsImpossiblyNarrowWindow(t *testing.T) {
	repo := makePreserveRepo(t, 12)

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 13:00:00",
		"--end", "2026-05-14 13:00:05", // 5 seconds for 12 commits
		"--window-tz", "UTC",
		"--preserve",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit for an impossibly narrow window; stdout=%s", out.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("one second per commit")) {
		t.Errorf("expected a one-second-per-commit error, got: %s", errOut.String())
	}
	if _, err := os.Stat(repo + ".dead"); err == nil {
		t.Errorf("failed run must not have swapped the repo (%s.dead exists)", repo)
	}
}

// TestRunPreserveFlagParses confirms --preserve is registered and reaches the
// pipeline (it is not gated behind --fabricate).
func TestRunPreserveFlagParses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-preserve",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--preserve",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("unknown flag")) {
		t.Fatalf("--preserve flag is not registered; stderr=%q", errOut.String())
	}
}

// TestRunHelpListsPreserve confirms the flag is documented in --help.
func TestRunHelpListsPreserve(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := RunWithArgs([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("--help exit %d", code)
	}
	if !bytes.Contains(out.Bytes(), []byte("--preserve")) {
		t.Errorf("expected --preserve in help output:\n%s", out.String())
	}
}

// TestIntegration_FabricatePreserve_KeepsAllInTinyWindow exercises --preserve
// on the fabricate path: the same tiny window that makes pigs mode squash
// heavily keeps every fabricated commit when --preserve is set.
func TestIntegration_FabricatePreserve_KeepsAllInTinyWindow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
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

	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cfg := &input.Config{
		Repo:          repo,
		Start:         start,
		End:           start.Add(30 * time.Minute), // same tiny window that squashes without preserve
		WindowTZ:      time.UTC,
		Fabricate:     true,
		PigsN:         2,
		PigIdentities: []string{"Alice <a@x.com>", "Bob <b@x.com>"},
		Seed:          1,
		HasSeed:       true,
		Preserve:      true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("(0 squashed)")) {
		t.Errorf("expected --preserve to squash nothing on the fabricate path:\n%s", out.String())
	}
	// Without preserve this window collapses to ~3 commits; preserve should
	// keep the full fabricated flurry (well above that).
	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, logOut)
	}
	if n := bytes.Count(logOut, []byte("\n")); n <= 4 {
		t.Errorf("expected preserve to keep the full flurry (>4 commits), got %d:\n%s", n, logOut)
	}
}

// commitAuthorUnix returns author timestamps (unix seconds) of every commit in
// chronological (oldest-first) order.
func commitAuthorUnix(t *testing.T, repo string) []int64 {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "log", "--reverse", "--pretty=%at").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	var ts []int64
	for _, line := range strings.Fields(strings.TrimSpace(string(out))) {
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			t.Fatalf("parse author time %q: %v", line, err)
		}
		ts = append(ts, v)
	}
	return ts
}
