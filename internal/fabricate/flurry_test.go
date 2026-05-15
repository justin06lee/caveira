package fabricate

import (
	"math/rand"
	"strings"
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

func TestFlurrySequence_SetsFeature(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                  "# x\n",
		"internal/walk/load.go":      "package walk\n",
		"internal/walk/load_test.go": "package walk\nimport \"testing\"\n",
		"internal/cli/main.go":       "package cli\n",
	})
	seq, err := FlurrySequence(repo, Identity{Name: "A", Email: "a@x.com"}, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("FlurrySequence: %v", err)
	}
	sawFeature := false
	for _, c := range seq {
		if strings.HasPrefix(c.Message, "chore") {
			if c.Feature != "" {
				t.Fatalf("chore commit Feature = %q, want empty", c.Feature)
			}
			continue
		}
		if c.Feature == "" {
			t.Fatalf("feature commit %q has empty Feature", c.Message)
		}
		sawFeature = true
	}
	if !sawFeature {
		t.Fatal("expected at least one commit with a Feature set")
	}
}

func msgs(cs []SynthCommit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Message
	}
	return out
}
