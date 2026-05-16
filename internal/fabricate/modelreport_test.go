package fabricate

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestParseCoAuthors(t *testing.T) {
	msg := "feat: add thing\n\n" +
		"Some body text.\n\n" +
		"Co-Authored-By: Claude <noreply@anthropic.com>\n" +
		"co-authored-by: Bob Jones <bob@example.com>\n"
	got := parseCoAuthors(msg)
	if len(got) != 2 {
		t.Fatalf("got %d co-authors, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Claude" || got[0].Email != "noreply@anthropic.com" {
		t.Errorf("co-author 0 = %+v", got[0])
	}
	if got[1].Name != "Bob Jones" || got[1].Email != "bob@example.com" {
		t.Errorf("co-author 1 = %+v", got[1])
	}
}

func TestParseCoAuthors_None(t *testing.T) {
	if got := parseCoAuthors("feat: a plain commit\n\nno trailers here"); len(got) != 0 {
		t.Fatalf("expected no co-authors, got %+v", got)
	}
}

func TestParseCoAuthors_SkipsInvalid(t *testing.T) {
	msg := "feat: add thing\n\n" +
		"Co-Authored-By: no brackets here\n" +
		"Co-Authored-By: Real Person <real@example.com>\n"
	got := parseCoAuthors(msg)
	if len(got) != 1 {
		t.Fatalf("got %d co-authors, want 1: %+v", len(got), got)
	}
	if got[0].Name != "Real Person" || got[0].Email != "real@example.com" {
		t.Errorf("co-author 0 = %+v", got[0])
	}
}

// commitAs creates one (empty) commit on wt with the given author, committer,
// and message. Used to build controlled history fixtures.
func commitAs(t *testing.T, wt *git.Worktree, author, committer Identity, msg string) {
	t.Helper()
	when := time.Now()
	_, err := wt.Commit(msg, &git.CommitOptions{
		Author:            &object.Signature{Name: author.Name, Email: author.Email, When: when},
		Committer:         &object.Signature{Name: committer.Name, Email: committer.Email, When: when},
		AllowEmptyCommits: true,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestScanModelReport(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	bob := Identity{Name: "Bob", Email: "bob@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}

	// Alice: 4 commits, 3 co-authored by Claude.
	commitAs(t, wt, alice, alice, "feat: a1\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a2\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a3\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a4 (solo)")
	// Bob: 2 commits, none with a model.
	commitAs(t, wt, bob, bob, "fix: b1")
	commitAs(t, wt, bob, bob, "fix: b2")

	report, err := ScanModelReport(repo)
	if err != nil {
		t.Fatalf("ScanModelReport: %v", err)
	}

	if len(report.Models) != 1 || report.Models[0].Email != claude.Email {
		t.Fatalf("Models = %+v, want [Claude]", report.Models)
	}
	pa, ok := report.Profiles["alice@example.com"]
	if !ok {
		t.Fatal("no profile for Alice")
	}
	if pa.Rate < 0.74 || pa.Rate > 0.76 { // 3/4
		t.Errorf("Alice Rate = %v, want ~0.75", pa.Rate)
	}
	if mix := pa.Mix["noreply@anthropic.com"]; mix < 0.99 { // 3/3
		t.Errorf("Alice Mix[claude] = %v, want 1.0", mix)
	}
	pb, ok := report.Profiles["bob@example.com"]
	if !ok {
		t.Fatal("no profile for Bob")
	}
	if pb.Rate != 0 || len(pb.Mix) != 0 {
		t.Errorf("Bob profile = %+v, want zero rate / empty mix", pb)
	}
}

func TestScanModelReport_MultiModel(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	alice := Identity{Name: "Alice", Email: "alice@example.com"}

	// Alice: 2 commits co-authored by Claude, 1 by Codex.
	commitAs(t, wt, alice, alice, "feat: a1\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a2\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a3\n\nCo-Authored-By: Codex <codex@openai.com>")

	report, err := ScanModelReport(repo)
	if err != nil {
		t.Fatalf("ScanModelReport: %v", err)
	}

	if len(report.Models) != 2 {
		t.Fatalf("Models = %+v, want 2 entries", report.Models)
	}

	pa, ok := report.Profiles["alice@example.com"]
	if !ok {
		t.Fatal("no profile for Alice")
	}
	claudeMix, hasClaude := pa.Mix["noreply@anthropic.com"]
	codexMix, hasCodex := pa.Mix["codex@openai.com"]
	if !hasClaude || !hasCodex {
		t.Fatalf("Mix missing a model key: %+v", pa.Mix)
	}
	if sum := claudeMix + codexMix; sum < 0.99 || sum > 1.01 {
		t.Errorf("Mix values sum = %v, want ~1.0", sum)
	}
	if claudeMix < 0.65 || claudeMix > 0.68 { // 2/3
		t.Errorf("Mix[claude] = %v, want ~0.667", claudeMix)
	}
	if codexMix < 0.32 || codexMix > 0.35 { // 1/3
		t.Errorf("Mix[codex] = %v, want ~0.333", codexMix)
	}
}

func TestScanModelReport_ModelCommitter(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}

	// Human author, but the model is the committer.
	commitAs(t, wt, alice, claude, "feat: a1")
	commitAs(t, wt, alice, alice, "feat: a2 (solo)")

	report, err := ScanModelReport(repo)
	if err != nil {
		t.Fatalf("ScanModelReport: %v", err)
	}

	pa, ok := report.Profiles["alice@example.com"]
	if !ok {
		t.Fatal("no profile for Alice")
	}
	if pa.Rate < 0.49 || pa.Rate > 0.51 { // 1/2
		t.Errorf("Alice Rate = %v, want ~0.5 (model committer counts)", pa.Rate)
	}
	if mix := pa.Mix["noreply@anthropic.com"]; mix < 0.99 {
		t.Errorf("Alice Mix[claude] = %v, want 1.0", mix)
	}
}
