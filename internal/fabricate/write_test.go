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

func TestWriteToRepo_SyntheticContentBlob(t *testing.T) {
	src := newEmptyRepo(t)
	dst := newEmptyRepo(t)

	content := []byte("package main\n\nfunc main() {}\n")
	h := plumbing.ComputeHash(plumbing.BlobObject, content)

	plan := &Plan{
		Commits: []SynthCommit{
			{
				ID:      0,
				Author:  Identity{Name: "A", Email: "a@x.com"},
				Message: "feat: add main",
				Added: []FileRef{
					{Path: "main.go", Content: content, Blob: h, Mode: filemode.Regular},
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
	obj, err := dst.Storer.EncodedObject(plumbing.BlobObject, h)
	if err != nil {
		t.Fatalf("synthetic blob not written to dst: %v", err)
	}
	if obj.Size() != int64(len(content)) {
		t.Fatalf("blob size = %d, want %d", obj.Size(), len(content))
	}
}

func TestWriteToRepo_PigsLinear(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)

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
	plan, _ := BuildRatsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
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
