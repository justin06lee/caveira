package walk

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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

// MakeFixtureBranchedMerged builds a repo with this shape:
//
//	A -- B -- C ---- M
//	      \         /
//	       D ----- E
//
// All commits modify a single file unique to that commit.
// Returns repo plus the OIDs in {A,B,C,D,E,M} order.
func MakeFixtureBranchedMerged(t *testing.T) (*git.Repository, map[string]string) {
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
	oids := map[string]string{}

	doCommit := func(label string, body string, dt time.Duration) plumbing.Hash {
		name := "f_" + label + ".txt"
		f, _ := fs.Create(name)
		_, _ = f.Write([]byte(body))
		_ = f.Close()
		_, _ = wt.Add(name)
		h, err := wt.Commit("commit "+label, &git.CommitOptions{
			Author: &object.Signature{Name: "Test", Email: "t@e.com", When: base.Add(dt)},
		})
		if err != nil {
			t.Fatalf("commit %s: %v", label, err)
		}
		oids[label] = h.String()
		return h
	}

	_ = doCommit("A", "a\n", 0)
	b := doCommit("B", "b\n", time.Hour)
	c := doCommit("C", "c\n", 2*time.Hour)

	// Branch off B for D.
	if err := wt.Checkout(&git.CheckoutOptions{Hash: b, Branch: plumbing.NewBranchReferenceName("feat"), Create: true}); err != nil {
		t.Fatalf("checkout feat: %v", err)
	}
	doCommit("D", "d\n", 3*time.Hour)
	doCommit("E", "e\n", 4*time.Hour)

	// Back to master, then merge feat.
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master")}); err != nil {
		// Some setups use main; try that.
		if err2 := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("main")}); err2 != nil {
			t.Fatalf("checkout master/main: %v / %v", err, err2)
		}
	}

	// Create a merge commit manually: take master's tree (C), add merge metadata.
	mergeMsg := "Merge branch feat"
	mergeHash, err := commitMerge(repo, c, oids["E"], mergeMsg, base.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("merge commit: %v", err)
	}
	oids["M"] = mergeHash.String()

	// Move HEAD to the merge commit.
	headRef := plumbing.NewBranchReferenceName("master")
	if _, err := repo.Storer.Reference(headRef); err != nil {
		headRef = plumbing.NewBranchReferenceName("main")
	}
	_ = repo.Storer.SetReference(plumbing.NewHashReference(headRef, mergeHash))

	return repo, oids
}

func commitMerge(repo *git.Repository, parent1 plumbing.Hash, parent2Str string, msg string, when time.Time) (plumbing.Hash, error) {
	p1, err := repo.CommitObject(parent1)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	tree, err := p1.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	p2hash := plumbing.NewHash(parent2Str)

	mc := &object.Commit{
		Author:       object.Signature{Name: "Test", Email: "t@e.com", When: when},
		Committer:    object.Signature{Name: "Test", Email: "t@e.com", When: when},
		Message:      msg,
		TreeHash:     tree.Hash,
		ParentHashes: []plumbing.Hash{parent1, p2hash},
	}
	obj := repo.Storer.NewEncodedObject()
	if err := mc.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}
