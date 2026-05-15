package fabricate

import (
	"math/rand"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestPlanToDAG(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)

	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		t.Fatalf("PlanToDAG: %v", err)
	}
	if len(dag.All()) != len(plan.Commits) {
		t.Fatalf("dag has %d commits, plan has %d", len(dag.All()), len(plan.Commits))
	}
	for _, c := range plan.Commits {
		dc := dag.Get(SyntheticOID(c.ID))
		if dc == nil {
			t.Errorf("DAG missing commit %d", c.ID)
		}
	}
	chore := dag.Get(SyntheticOID(0))
	if chore.NewFiles != 1 {
		t.Errorf("chore commit NewFiles = %d, want 1", chore.NewFiles)
	}
}

func TestPlanToDAG_HonorsExplicitStats(t *testing.T) {
	src := newEmptyRepo(t)
	plan := &Plan{
		Commits: []SynthCommit{
			{
				ID:      0,
				Author:  Identity{Name: "A", Email: "a@x.com"},
				Message: "feat: layered",
				Added: []FileRef{
					{Path: "f.go", Content: []byte("a\nb\nc\nd\ne\n"),
						Blob: plumbing.ComputeHash(plumbing.BlobObject, []byte("a\nb\nc\nd\ne\n"))},
				},
				Stats: &DiffStat{Lines: 2, Files: 1, NewFiles: 1},
			},
		},
		Refs: map[string]int{"refs/heads/master": 0}, HEAD: 0, HeadRef: "refs/heads/master",
	}
	dag, err := PlanToDAG(src, plan)
	if err != nil {
		t.Fatalf("PlanToDAG: %v", err)
	}
	c := dag.Get(SyntheticOID(0))
	if c.LinesChanged != 2 {
		t.Fatalf("LinesChanged = %d, want 2 (from explicit Stats)", c.LinesChanged)
	}
}
