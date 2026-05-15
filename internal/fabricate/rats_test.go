package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestRatsMode_FeatureBranches(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
		"internal/cli/main.go":  "package cli\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, err := BuildRatsPlan(repo, []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}, rng)
	if err != nil {
		t.Fatalf("BuildRatsPlan: %v", err)
	}

	sawFeatRef := 0
	for ref := range plan.Refs {
		if strings.HasPrefix(ref, "refs/heads/feat/") {
			sawFeatRef++
		}
	}
	if sawFeatRef < 2 {
		t.Errorf("expected at least 2 feat/ branches, got %d (refs: %+v)", sawFeatRef, plan.Refs)
	}

	sawMerge := 0
	for _, c := range plan.Commits {
		if c.IsMerge {
			sawMerge++
		}
	}
	if sawMerge < 2 {
		t.Errorf("expected at least 2 merge commits, got %d", sawMerge)
	}
}

func TestRatsMode_MergesAttributedToOwner(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"a/x.go": "package a\n",
		"b/y.go": "package b\n",
	})
	rng := rand.New(rand.NewSource(1))
	ids := []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}
	plan, _ := BuildRatsPlan(repo, ids, rng)
	for _, c := range plan.Commits {
		if !c.IsMerge {
			continue
		}
		if c.Author.Email != "a@x.com" && c.Author.Email != "b@x.com" {
			t.Errorf("merge commit author should be one of the rats, got %+v", c.Author)
		}
	}
}
