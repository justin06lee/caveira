package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIntegration_LeechesScattersAuthorsPreservesTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "justin@x.com")
	mustGit(t, repo, "config", "user.name", "Justin")
	for i := 0; i < 12; i++ {
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(strings.Repeat("x\n", i+1)), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-m", "commit "+strconv.Itoa(i))
	}

	srcTree, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD^{tree}").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse src tree: %v\n%s", err, srcTree)
	}

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 09:00",
		"--end", "2026-05-14 17:00",
		"--window-tz", "UTC",
		"--leeches", "3",
		"--leech", "Alice <a@x.com>",
		"--leech", "Bob <b@x.com>",
		"--leech", "Carol <c@x.com>",
		"--seed", "42",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	// Tree must be byte-identical: leeches only rewrites identities/timestamps.
	dstTree, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD^{tree}").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse dst tree: %v\n%s", err, dstTree)
	}
	if !bytes.Equal(bytes.TrimSpace(srcTree), bytes.TrimSpace(dstTree)) {
		t.Fatalf("HEAD tree changed: %s -> %s", srcTree, dstTree)
	}

	// Collect the author emails across the rewritten history.
	emailsOut, err := exec.Command("git", "-C", repo, "log", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, emailsOut)
	}
	present := map[string]int{}
	for _, e := range strings.Fields(string(emailsOut)) {
		present[e]++
	}

	// At least two leeches and the original author should appear — a random
	// scatter over {Alice, Bob, Carol, Justin}, not a single reassignment.
	leechesSeen := 0
	for _, e := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		if present[e] > 0 {
			leechesSeen++
		}
	}
	if leechesSeen < 2 {
		t.Errorf("expected at least 2 leeches scattered in, saw %d (emails: %v)", leechesSeen, present)
	}
	if present["justin@x.com"] == 0 {
		t.Errorf("expected the original author to remain in the mix, emails: %v", present)
	}
	if len(present) < 3 {
		t.Errorf("expected authorship spread across >=3 identities, saw: %v", present)
	}

	// Commit messages must be preserved unchanged.
	msgOut, _ := exec.Command("git", "-C", repo, "log", "--format=%s").CombinedOutput()
	if !bytes.Contains(msgOut, []byte("commit 0")) || !bytes.Contains(msgOut, []byte("commit 11")) {
		t.Errorf("original commit messages not preserved:\n%s", msgOut)
	}

	if !bytes.Contains(out.Bytes(), []byte("Authors scattered across")) {
		t.Errorf("expected scatter summary in output:\n%s", out.String())
	}
}
