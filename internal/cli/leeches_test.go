package cli

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/justin06lee/caveira/internal/fabricate"
	"github.com/justin06lee/caveira/internal/walk"
)

func linearWalkDAG(n int) *walk.DAG {
	dag := walk.NewDAG()
	for i := 0; i < n; i++ {
		var parents []string
		if i > 0 {
			parents = []string{strconv.Itoa(i - 1)}
		}
		dag.Add(&walk.Commit{
			OID:       strconv.Itoa(i),
			Parents:   parents,
			Author:    walk.Person{Name: "Orig", Email: "orig@x.com"},
			Committer: walk.Person{Name: "Orig", Email: "orig@x.com"},
			Message:   "c" + strconv.Itoa(i),
			IsRoot:    i == 0,
		})
	}
	return dag
}

func TestScatterLeechAuthors_AssignsFromPoolAndSetsBoth(t *testing.T) {
	dag := linearWalkDAG(30)
	pool := []fabricate.Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
		{Name: "Orig", Email: "orig@x.com"},
	}
	allowed := map[string]bool{}
	for _, id := range pool {
		allowed[id.Name+"|"+id.Email] = true
	}

	counts := scatterLeechAuthors(dag, pool, rand.New(rand.NewSource(1)))

	total := 0
	for _, c := range dag.All() {
		if !allowed[c.Author.Name+"|"+c.Author.Email] {
			t.Fatalf("commit %s got author outside pool: %+v", c.OID, c.Author)
		}
		if c.Author != c.Committer {
			t.Fatalf("commit %s author %+v != committer %+v", c.OID, c.Author, c.Committer)
		}
	}
	for _, n := range counts {
		total += n
	}
	if total != 30 {
		t.Fatalf("counts total = %d, want 30", total)
	}
	// With 30 commits over 3 identities, a uniform draw should touch more than
	// one identity (guards against a rotation/constant-assignment regression).
	if len(counts) < 2 {
		t.Fatalf("expected the scatter to use multiple identities, got %d", len(counts))
	}
}

func TestScatterLeechAuthors_DeterministicUnderSeed(t *testing.T) {
	pool := []fabricate.Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}
	run := func() map[string]walk.Person {
		dag := linearWalkDAG(20)
		scatterLeechAuthors(dag, pool, rand.New(rand.NewSource(99)))
		out := map[string]walk.Person{}
		for _, c := range dag.All() {
			out[c.OID] = c.Author
		}
		return out
	}
	a, b := run(), run()
	for oid, pa := range a {
		if b[oid] != pa {
			t.Fatalf("commit %s differs across runs: %+v vs %+v", oid, pa, b[oid])
		}
	}
}
