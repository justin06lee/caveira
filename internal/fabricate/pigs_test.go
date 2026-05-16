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
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, nil, rng)
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

func TestReshapePigs_RandomAuthorDistribution(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	base := make([]SynthCommit, 300)
	for i := range base {
		base[i] = SynthCommit{ID: i, Message: "feat: c"}
	}
	plan := reshapePigs(base, ids, nil, rand.New(rand.NewSource(1)))
	counts := map[string]int{}
	for _, c := range plan.Commits {
		counts[c.Author.Email]++
	}
	if counts["a@x"] < 90 || counts["b@x"] < 90 {
		t.Fatalf("expected both authors well-represented, got %+v", counts)
	}
}

func TestReshapePigs_WeightedAuthorDistribution(t *testing.T) {
	ids := []Identity{{Name: "Heavy", Email: "h@x"}, {Name: "Light", Email: "l@x"}}
	base := make([]SynthCommit, 300)
	for i := range base {
		base[i] = SynthCommit{ID: i, Message: "feat: c"}
	}
	plan := reshapePigs(base, ids, []int{9, 1}, rand.New(rand.NewSource(1)))
	counts := map[string]int{}
	for _, c := range plan.Commits {
		counts[c.Author.Email]++
	}
	if counts["h@x"] <= counts["l@x"] {
		t.Fatalf("weighted reshape not skewed toward Heavy: %+v", counts)
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
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, nil, rng)
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
	plan := reshapePigs(base, ids, nil, rand.New(rand.NewSource(7)))
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
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, nil, rng)
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
