package fabricate

import (
	"math/rand"
	"testing"
)

func TestFlurrySequence_Linear(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                  "# x\n",
		"internal/walk/load.go":      "package walk\n",
		"internal/walk/load_test.go": "package walk\nimport \"testing\"\n",
		"internal/cli/main.go":       "package cli\n",
	})

	identity := Identity{Name: "Solo", Email: "solo@x.com"}
	rng := rand.New(rand.NewSource(1))
	commits, err := FlurrySequence(repo, identity, rng)
	if err != nil {
		t.Fatalf("FlurrySequence: %v", err)
	}
	// 1 chore + 1 code (cli, no tests) + 1 code (walk) + 1 test (walk) = 4
	if len(commits) != 4 {
		t.Fatalf("expected 4 commits, got %d: %+v", len(commits), msgs(commits))
	}
	if commits[0].Message != "chore: project scaffolding" && commits[0].Message != "chore: initial scaffolding" {
		t.Errorf("first commit should be chore, got %q", commits[0].Message)
	}
	for _, c := range commits {
		if c.Author != identity {
			t.Errorf("commit author = %+v, want %+v", c.Author, identity)
		}
	}
	// Parents: chore is parent of first feature commit; each subsequent is
	// parent of the next.
	for i := 1; i < len(commits); i++ {
		if len(commits[i].Parents) != 1 || commits[i].Parents[0] != i-1 {
			t.Errorf("commit %d parents = %v, want [%d]", i, commits[i].Parents, i-1)
		}
	}
}

func msgs(cs []SynthCommit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Message
	}
	return out
}
