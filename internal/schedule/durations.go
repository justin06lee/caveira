package schedule

import (
	"math/rand"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/walk"
)

// BuildDurations assigns a duration in minutes to every commit in the DAG.
// Returns the duration table and the difficulty assignment (for reporting).
func BuildDurations(d *walk.DAG, rng *rand.Rand) (map[string]int, map[string]difficulty.Difficulty) {
	durs := make(map[string]int, len(d.All()))
	diffs := make(map[string]difficulty.Difficulty, len(d.All()))
	for _, c := range d.All() {
		score := difficulty.Score(c.LinesChanged, c.FilesTouched, c.NewFiles)
		b := difficulty.Bucket(score, c.IsMerge)
		diffs[c.OID] = b
		durs[c.OID] = difficulty.Draw(b, rng)
	}
	return durs, diffs
}
