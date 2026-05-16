package fabricate

import (
	"math/rand"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/walk"
)

// Generate runs the appropriate fabricator (pigs / rats / single-author) and
// returns the Plan plus a walk.DAG view of it for the scheduler.
func Generate(repo *git.Repository, ids []Identity, weights []int, mode string, rng *rand.Rand) (*Plan, *walk.DAG, error) {
	var plan *Plan
	var err error
	switch mode {
	case "rats":
		plan, err = BuildRatsPlan(repo, ids, weights, rng)
	default:
		// "pigs" and "single" both use the pigs builder. With one identity
		// and no extra noise tuning, single-author output is just a clean-ish
		// single-person history.
		plan, err = BuildPigsPlan(repo, ids, weights, rng)
	}
	if err != nil {
		return nil, nil, err
	}
	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		return nil, nil, err
	}
	return plan, dag, nil
}
