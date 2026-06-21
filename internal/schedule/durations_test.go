package schedule

import (
	"math/rand"
	"testing"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/walk"
)

func TestBuildDurations(t *testing.T) {
	d := walk.NewDAG()
	d.Add(&walk.Commit{OID: "A", IsRoot: true, LinesChanged: 1, FilesTouched: 1, NewFiles: 1})
	d.Add(&walk.Commit{OID: "B", Parents: []string{"A"}, LinesChanged: 500, FilesTouched: 5, NewFiles: 0})
	d.Add(&walk.Commit{OID: "M", Parents: []string{"A", "B"}, IsMerge: true})

	rng := rand.New(rand.NewSource(1))
	durs, _ := BuildDurations(d, rng)
	for _, oid := range []string{"A", "B", "M"} {
		if durs[oid] <= 0 {
			t.Errorf("%s: expected positive duration, got %d", oid, durs[oid])
		}
	}
	// Merge commit must use trivial bucket -> [2, 8]
	if durs["M"] < 2 || durs["M"] > 8 {
		t.Errorf("merge M expected in trivial range [2,8], got %d", durs["M"])
	}
	_ = difficulty.Trivial
}

// TestBuildDurations_DeterministicWithSeed guards the --seed reproducibility
// promise: the seeded RNG must be consumed in a stable order independent of the
// DAG's internal map iteration. Many repeated runs with the same seed must yield
// byte-for-byte identical duration tables. (Before the fix, BuildDurations drew
// in random map order, so this varied run to run.)
func TestBuildDurations_DeterministicWithSeed(t *testing.T) {
	build := func() map[string]int {
		d := walk.NewDAG()
		// Enough commits that random map order would almost certainly differ
		// between builds if the draw order weren't stabilized.
		for i := 0; i < 50; i++ {
			oid := string(rune('a'+i%26)) + string(rune('A'+i/26))
			d.Add(&walk.Commit{OID: oid, LinesChanged: i * 7, FilesTouched: i % 4, NewFiles: i % 2})
		}
		durs, _ := BuildDurations(d, rand.New(rand.NewSource(99)))
		return durs
	}

	first := build()
	for run := 0; run < 20; run++ {
		got := build()
		if len(got) != len(first) {
			t.Fatalf("run %d: size %d != %d", run, len(got), len(first))
		}
		for oid, v := range first {
			if got[oid] != v {
				t.Fatalf("run %d: nondeterministic duration for %s: got %d want %d", run, oid, got[oid], v)
			}
		}
	}
}
