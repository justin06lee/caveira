package schedule

import (
	"math/rand"
	"sort"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/walk"
)

// BuildDurations assigns a duration in minutes to every commit in the DAG.
// Returns the duration table and the difficulty assignment (for reporting).
//
// Commits are processed in a stable OID order so that the seeded RNG is
// consumed in the same sequence on every run. Iterating DAG.All() directly
// would walk the underlying map in random order, making the per-commit draws
// nondeterministic and breaking the reproducibility promised by --seed.
func BuildDurations(d *walk.DAG, rng *rand.Rand) (map[string]int, map[string]difficulty.Difficulty) {
	all := d.All()
	sort.Slice(all, func(i, j int) bool { return all[i].OID < all[j].OID })

	durs := make(map[string]int, len(all))
	diffs := make(map[string]difficulty.Difficulty, len(all))
	for _, c := range all {
		score := difficulty.Score(c.LinesChanged, c.FilesTouched, c.NewFiles)
		b := difficulty.Bucket(score, c.IsMerge)
		diffs[c.OID] = b
		durs[c.OID] = difficulty.Draw(b, rng)
	}
	return durs, diffs
}
