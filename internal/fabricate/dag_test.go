package fabricate

import (
	"math/rand"
	"testing"
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
