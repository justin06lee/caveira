package fabricate

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

func TestWalkTree_AndGroup(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                  "# Hello\n",
		"internal/walk/load.go":      "package walk\n",
		"internal/walk/load_test.go": "package walk\nimport \"testing\"\n",
	})

	files, err := WalkHead(repo)
	if err != nil {
		t.Fatalf("WalkHead: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	chore, features := GroupFiles(files)
	if len(chore) != 1 || chore[0].Path != "README.md" {
		t.Errorf("chore set: %+v", chore)
	}
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	f := features[0]
	if f.Dir != "internal/walk" {
		t.Errorf("feature dir: got %q want %q", f.Dir, "internal/walk")
	}
	if len(f.Code) != 1 || f.Code[0].Path != "internal/walk/load.go" {
		t.Errorf("code set: %+v", f.Code)
	}
	if len(f.Test) != 1 || f.Test[0].Path != "internal/walk/load_test.go" {
		t.Errorf("test set: %+v", f.Test)
	}
}

// newFixtureRepo builds an in-memory git repo with the given files (path ->
// content) all committed in a single seed commit. Shared by fabricate tests.
func newFixtureRepo(t *testing.T, files map[string]string) *git.Repository {
	t.Helper()
	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := repo.Worktree()
	for p, body := range files {
		f, err := fs.Create(p)
		if err != nil {
			t.Fatalf("create %s: %v", p, err)
		}
		_, _ = f.Write([]byte(body))
		_ = f.Close()
		_, _ = wt.Add(p)
	}
	_, err = wt.Commit("seed", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return repo
}
