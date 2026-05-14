package walk

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

// MakeFixtureLinear builds a fresh in-memory repo with `n` commits, each
// touching its own file with the given number of inserted lines.
// Returns the repo and a slice of commit OIDs in chronological order.
func MakeFixtureLinear(t *testing.T, n int, linesPerCommit []int) (*git.Repository, []string) {
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
	var oids []string
	for i := 0; i < n; i++ {
		name := nameOfCommit(i)
		f, err := fs.Create(name)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		lines := linesPerCommit[i]
		for j := 0; j < lines; j++ {
			if _, err := f.Write([]byte("x\n")); err != nil {
				t.Fatalf("write: %v", err)
			}
		}
		_ = f.Close()
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("add: %v", err)
		}
		oid, err := wt.Commit("commit "+name, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  base.Add(time.Duration(i) * time.Hour),
			},
		})
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		oids = append(oids, oid.String())
	}
	return repo, oids
}

func nameOfCommit(i int) string {
	return "file_" + string(rune('a'+i)) + ".txt"
}
