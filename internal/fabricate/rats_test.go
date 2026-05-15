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

func TestRatsMode_OffBranchForkAtSomeSeed(t *testing.T) {
	// With enough features and the off-branch probability, at least one seed
	// should produce a feature branch whose first commit's parent is NOT the
	// chore commit (i.e., it forked from another open branch).
	files := map[string]string{}
	for _, dir := range []string{"a", "b", "c", "d", "e", "f"} {
		files[dir+"/x.go"] = "package " + dir + "\n"
	}
	repo := newFixtureRepo(t, files)
	ids := []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}
	sawOffBranch := false
	for s := int64(0); s < 50; s++ {
		rng := rand.New(rand.NewSource(s))
		plan, _ := BuildRatsPlan(repo, ids, rng)
		// Find non-merge non-chore commits whose parent is not the chore.
		for _, c := range plan.Commits {
			if c.IsMerge || len(c.Added) == 0 || c.ID == 0 {
				continue
			}
			if len(c.Parents) == 1 && c.Parents[0] != 0 {
				parent := plan.Commits[c.Parents[0]]
				if !parent.IsMerge && parent.ID != 0 {
					sawOffBranch = true
					break
				}
			}
		}
		if sawOffBranch {
			break
		}
	}
	if !sawOffBranch {
		t.Errorf("expected at least one seed across 50 trials to fork off another branch")
	}
}
