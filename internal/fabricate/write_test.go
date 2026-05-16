package fabricate

import (
	"math/rand"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/justin06lee/caveira/internal/rewrite"
)

// newEmptyRepo builds an empty in-memory git repo with no commits.
func newEmptyRepo(t *testing.T) *git.Repository {
	t.Helper()
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return repo
}

func TestWriteToRepo_DropsPreexistingRefs(t *testing.T) {
	content := []byte("package main\n\nfunc main() {}\n")
	src := newFixtureRepo(t, map[string]string{"main.go": string(content)})
	h := plumbing.ComputeHash(plumbing.BlobObject, content)
	dst, err := rewrite.InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Simulate refs copied verbatim by Duplicate: branches and a
	// remote-tracking ref that are NOT part of the fabricated plan.
	staleHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	stale := []plumbing.ReferenceName{
		"refs/heads/old-feature",
		"refs/remotes/origin/master",
	}
	for _, name := range stale {
		if err := dst.Storer.SetReference(plumbing.NewHashReference(name, staleHash)); err != nil {
			t.Fatalf("seed ref %s: %v", name, err)
		}
	}

	plan := &Plan{
		Commits: []SynthCommit{
			{
				ID:      0,
				Author:  Identity{Name: "A", Email: "a@x.com"},
				Message: "feat: add main",
				Added: []FileRef{
					{Path: "main.go", Blob: h, Mode: filemode.Regular},
				},
			},
		},
		Refs:    map[string]int{"refs/heads/master": 0},
		HEAD:    0,
		HeadRef: "refs/heads/master",
	}
	times := map[string]time.Time{SyntheticOID(0): time.Now()}

	if _, err := WriteToRepo(src, dst, plan, times); err != nil {
		t.Fatalf("WriteToRepo: %v", err)
	}

	want := map[string]bool{"refs/heads/master": true}
	refs, err := dst.References()
	if err != nil {
		t.Fatalf("dst.References: %v", err)
	}
	got := map[string]bool{}
	_ = refs.ForEach(func(r *plumbing.Reference) error {
		if r.Name() == plumbing.HEAD {
			return nil
		}
		got[r.Name().String()] = true
		return nil
	})
	for _, name := range stale {
		if got[name.String()] {
			t.Errorf("stale ref %s still present after WriteToRepo", name)
		}
	}
	if !got["refs/heads/master"] {
		t.Errorf("plan ref refs/heads/master missing after WriteToRepo")
	}
	for name := range got {
		if !want[name] {
			t.Errorf("unexpected ref %s present; only plan refs + HEAD should remain", name)
		}
	}
}

func TestWriteToRepo_PigsLinear(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, nil, rng)

	dst, err := rewrite.InMemoryClone(repo)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	times := map[string]time.Time{}
	for _, c := range plan.Commits {
		times[SyntheticOID(c.ID)] = base.Add(time.Duration(c.ID*10) * time.Minute)
	}

	mapping, err := WriteToRepo(repo, dst, plan, times)
	if err != nil {
		t.Fatalf("WriteToRepo: %v", err)
	}
	if len(mapping) != len(plan.Commits) {
		t.Fatalf("mapping has %d entries, want %d", len(mapping), len(plan.Commits))
	}

	head, err := dst.Head()
	if err != nil {
		t.Fatalf("dst.Head: %v", err)
	}
	if head.Hash() == plumbing.ZeroHash {
		t.Errorf("HEAD hash zero")
	}

	c, err := dst.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject(head): %v", err)
	}
	tree, _ := c.Tree()
	_, err = tree.File("README.md")
	if err != nil {
		t.Errorf("README.md not in dst HEAD tree: %v", err)
	}
}

func TestWriteToRepo_RatsNestedTree(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
		"internal/cli/main.go":  "package cli\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildRatsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, nil, rng)
	dst, err := rewrite.InMemoryClone(repo)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	times := map[string]time.Time{}
	for _, c := range plan.Commits {
		times[SyntheticOID(c.ID)] = base.Add(time.Duration(c.ID*10) * time.Minute)
	}
	if _, err := WriteToRepo(repo, dst, plan, times); err != nil {
		t.Fatalf("WriteToRepo: %v", err)
	}
	head, _ := dst.Head()
	c, _ := dst.CommitObject(head.Hash())
	tree, _ := c.Tree()
	// Nested path must resolve.
	if _, err := tree.File("internal/walk/load.go"); err != nil {
		t.Errorf("nested file internal/walk/load.go not found in HEAD tree: %v", err)
	}
	if _, err := tree.File("internal/cli/main.go"); err != nil {
		t.Errorf("nested file internal/cli/main.go not found in HEAD tree: %v", err)
	}
}
