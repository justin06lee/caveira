package fabricate

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/justin06lee/caveira/internal/walk"
)

// makeMultiAuthorRepo builds an in-memory go-git repo with one commit per
// supplied author, so DiscoverIdentities yields several distinct identities.
func makeMultiAuthorRepo(t *testing.T, authors []Identity) *git.Repository {
	t.Helper()
	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for i, a := range authors {
		name := "file_" + string(rune('a'+i)) + ".txt"
		f, err := fs.Create(name)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		_, _ = f.Write([]byte("content\n"))
		_ = f.Close()
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("add: %v", err)
		}
		_, err = wt.Commit("commit "+name, &git.CommitOptions{
			Author: &object.Signature{
				Name:  a.Name,
				Email: a.Email,
				When:  base.Add(time.Duration(i) * time.Hour),
			},
		})
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	return repo
}

func TestParseIdentity_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want Identity
	}{
		{"Alice <a@x.com>", Identity{Name: "Alice", Email: "a@x.com"}},
		{"Alice Cooper <alice.cooper@example.com>", Identity{Name: "Alice Cooper", Email: "alice.cooper@example.com"}},
		{"  Bob   <bob@y.com>  ", Identity{Name: "Bob", Email: "bob@y.com"}},
	}
	for _, c := range cases {
		got, err := ParseIdentity(c.in)
		if err != nil {
			t.Errorf("ParseIdentity(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseIdentity(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseIdentity_Invalid(t *testing.T) {
	cases := []string{
		"",
		"Alice",
		"a@x.com",
		"<a@x.com>",
		"Alice <>",
		"Alice <noatsign>",
	}
	for _, in := range cases {
		if _, err := ParseIdentity(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestDiscoverIdentities(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 3, []int{1, 1, 1})
	got, err := DiscoverIdentities(repo, nil)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 unique identity, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Test" || got[0].Email != "test@example.com" {
		t.Errorf("unexpected identity: %+v", got[0])
	}
}

func TestResolveIdentities_AllFromFlags(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{"Alice <a@x.com>", "Bob <b@x.com>"}
	got, err := ResolveIdentities(repo, flags, 2, nil, false, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(got))
	}
	if got[0].Name != "Alice" || got[1].Name != "Bob" {
		t.Errorf("flag identities lost or reordered: %+v", got)
	}
}

func TestResolveIdentities_FillFromGit(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{}
	got, err := ResolveIdentities(repo, flags, 1, nil, false, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(got))
	}
	if got[0].Name != "Test" {
		t.Errorf("expected discovered identity, got %+v", got[0])
	}
}

func TestResolveIdentities_PromptWhenShort(t *testing.T) {
	// Fixture has 1 identity ("Test"). We need 3. Should prompt twice.
	repo, _ := walk.MakeFixtureLinear(t, 1, []int{1})
	flags := []string{}
	stdin := strings.NewReader("Bob\nbob@x.com\nCarol\ncarol@x.com\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 3, nil, false, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(got))
	}
	if got[1].Name != "Bob" || got[2].Name != "Carol" {
		t.Errorf("prompted identities incorrect: %+v", got)
	}
}

func TestResolveIdentities_PickerWhenTooMany(t *testing.T) {
	// Three distinct authors discovered, but only 2 needed: the picker fires.
	// DiscoverIdentities sorts by commit count desc then name asc; each author
	// has exactly 1 commit, so the discovered order is alphabetical:
	// [Alice, Bob, Carol]. Picking "1,3" yields Alice and Carol.
	repo := makeMultiAuthorRepo(t, []Identity{
		{Name: "Alice", Email: "alice@x.com"},
		{Name: "Bob", Email: "bob@x.com"},
		{Name: "Carol", Email: "carol@x.com"},
	})
	flags := []string{}
	stdin := strings.NewReader("1,3\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 2, nil, false, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Alice" || got[0].Email != "alice@x.com" {
		t.Errorf("pick 1 should be Alice, got %+v", got[0])
	}
	if got[1].Name != "Carol" || got[1].Email != "carol@x.com" {
		t.Errorf("pick 3 should be Carol, got %+v", got[1])
	}
}

func TestResolveIdentities_PickerRejectsBadInput(t *testing.T) {
	repo := makeMultiAuthorRepo(t, []Identity{
		{Name: "Alice", Email: "alice@x.com"},
		{Name: "Bob", Email: "bob@x.com"},
		{Name: "Carol", Email: "carol@x.com"},
	})
	cases := []struct {
		name  string
		stdin string
	}{
		{"out of range", "9\n"},
		{"malformed", "abc\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout bytes.Buffer
			_, err := ResolveIdentities(repo, nil, 1, nil, false, strings.NewReader(c.stdin), &stdout)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", c.stdin)
			}
		})
	}
}

func TestDiscoverIdentities_MailmapUnifies(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	a1 := Identity{Name: "Jay", Email: "jay@personal.com"}
	a2 := Identity{Name: "jay06", Email: "jay@work.com"}
	commitAs(t, wt, a1, a1, "feat: one")
	commitAs(t, wt, a2, a2, "feat: two")

	mm := ParseMailmap([]byte("Jay <jay@personal.com> <jay@work.com>\n"))
	got, err := DiscoverIdentities(repo, mm)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 unified identity, got %d: %+v", len(got), got)
	}
	if got[0].Email != "jay@personal.com" || got[0].Commits != 2 {
		t.Fatalf("unified identity wrong: %+v", got[0])
	}
}

func TestDiscoverIdentities_ExcludesModels(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}

	commitAs(t, wt, alice, alice, "feat: human work")
	commitAs(t, wt, claude, claude, "chore: model-authored commit")

	got, err := DiscoverIdentities(repo, nil)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	for _, d := range got {
		if IsModel(d.Identity) {
			t.Fatalf("model %q <%s> leaked into discovered identities", d.Name, d.Email)
		}
	}
	foundAlice := false
	for _, d := range got {
		if d.Email == alice.Email {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Fatal("expected Alice in discovered identities")
	}
}

func TestCurateIdentities_SubsetAndEmpty(t *testing.T) {
	found := []DiscoveredIdentity{
		{Identity: Identity{Name: "A", Email: "a@x"}, Commits: 5},
		{Identity: Identity{Name: "B", Email: "b@x"}, Commits: 3},
		{Identity: Identity{Name: "C", Email: "c@x"}, Commits: 1},
	}
	// Pick a subset of 2 from 3.
	got, err := curateIdentities(found, 3, strings.NewReader("1,3\n"), io.Discard, 3, 0)
	if err != nil {
		t.Fatalf("curate subset: %v", err)
	}
	if len(got) != 2 || got[0].Email != "a@x" || got[1].Email != "c@x" {
		t.Fatalf("curate subset got %+v", got)
	}
	// Empty selection -> zero identities, no error.
	got, err = curateIdentities(found, 3, strings.NewReader("\n"), io.Discard, 3, 0)
	if err != nil {
		t.Fatalf("curate empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("curate empty got %+v", got)
	}
}

func TestCurateIdentities_RejectsOverAndOutOfRange(t *testing.T) {
	found := []DiscoveredIdentity{
		{Identity: Identity{Name: "A", Email: "a@x"}, Commits: 1},
		{Identity: Identity{Name: "B", Email: "b@x"}, Commits: 1},
	}
	if _, err := curateIdentities(found, 1, strings.NewReader("1,2\n"), io.Discard, 1, 0); err == nil {
		t.Fatal("expected error selecting more than max")
	}
	if _, err := curateIdentities(found, 2, strings.NewReader("9\n"), io.Discard, 2, 0); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestResolveIdentities_PickPathPromptsShortfall(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	alice := Identity{Name: "Alice", Email: "alice@x"}
	commitAs(t, wt, alice, alice, "feat: a")

	// --rats 2, pick mode: select the 1 discovered (Alice), then prompt 1 more.
	stdin := strings.NewReader("1\nBob\nbob@x\n")
	got, err := ResolveIdentities(repo, nil, 2, nil, true, stdin, io.Discard)
	if err != nil {
		t.Fatalf("ResolveIdentities pick: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %+v", got)
	}
	if got[0].Email != "alice@x" || got[1].Email != "bob@x" {
		t.Fatalf("pick-path identities wrong: %+v", got)
	}
}
