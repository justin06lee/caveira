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
