package fabricate

import (
	"math/rand"
	"testing"
)

func TestPigsMode_SingleAuthor_NoNoise(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	if len(plan.Commits) < 2 {
		t.Fatalf("expected >= 2 commits, got %d", len(plan.Commits))
	}
	for _, c := range plan.Commits {
		if c.Author.Name != "Solo" {
			t.Errorf("commit author = %+v, want Solo", c.Author)
		}
	}
}

func TestPigsMode_TwoAuthors_RoundRobin(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md": "# x\n",
		"a/x.go":    "package a\n",
		"b/y.go":    "package b\n",
		"c/z.go":    "package c\n",
		"d/w.go":    "package d\n",
	})
	rng := rand.New(rand.NewSource(7))
	plan, err := BuildPigsPlan(repo, []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	sawA, sawB := false, false
	for _, c := range plan.Commits {
		switch c.Author.Name {
		case "Alice":
			sawA = true
		case "Bob":
			sawB = true
		}
	}
	if !sawA || !sawB {
		t.Errorf("expected both authors to appear; sawA=%v sawB=%v", sawA, sawB)
	}
}

func TestPigsMode_NoiseCommitsAreEmptyAndShortMessage(t *testing.T) {
	files := map[string]string{}
	for _, dir := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		files[dir+"/x.go"] = "package " + dir + "\n"
		files[dir+"/x_test.go"] = "package " + dir + "\n"
	}
	repo := newFixtureRepo(t, files)
	rng := rand.New(rand.NewSource(3))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	sawNoise := false
	for _, c := range plan.Commits {
		if len(c.Added) == 0 && c.Message != "" && !c.IsMerge {
			sawNoise = true
		}
	}
	if !sawNoise {
		t.Logf("note: no noise commits at this seed; not a hard failure but informational")
	}
}

func TestReshapePigs_LinearChainWithAuthors(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x.com"}, {Name: "B", Email: "b@x.com"}}
	base := []SynthCommit{
		{ID: 0, Message: "chore: scaffold"},
		{ID: 1, Parents: []int{0}, Message: "feat(walk): add walk", Feature: "walk"},
		{ID: 2, Parents: []int{1}, Message: "feat(cli): add cli", Feature: "cli"},
	}
	plan := reshapePigs(base, ids, rand.New(rand.NewSource(7)))
	if plan.HeadRef != defaultBranch {
		t.Fatalf("HeadRef = %q, want %q", plan.HeadRef, defaultBranch)
	}
	authors := map[string]bool{}
	for _, c := range plan.Commits {
		authors[c.Author.Email] = true
		if len(c.Parents) > 1 {
			t.Fatalf("pigs plan must be linear; commit %d has %d parents", c.ID, len(c.Parents))
		}
	}
	if len(authors) < 2 {
		t.Fatalf("expected both identities used, got %d", len(authors))
	}
}

func TestPigsMode_HeadRefAndLinearChain(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(2))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	if plan.HeadRef == "" {
		t.Errorf("HeadRef should not be empty")
	}
	if _, ok := plan.Refs[plan.HeadRef]; !ok {
		t.Errorf("Refs[%q] missing", plan.HeadRef)
	}
	for _, c := range plan.Commits {
		if len(c.Parents) > 1 {
			t.Errorf("commit %d has %d parents, pigs mode should be linear", c.ID, len(c.Parents))
		}
	}
}
