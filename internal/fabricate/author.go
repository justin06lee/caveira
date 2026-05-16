package fabricate

import "math/rand"

// pickAuthor selects one player from ids using rng. With a nil, wrong-length,
// or all-non-positive weights slice it draws uniformly. Otherwise it draws
// weighted: weights[i] is the (parallel) weight of ids[i]; non-positive weights
// are treated as zero. ids must be non-empty.
func pickAuthor(ids []Identity, weights []int, rng *rand.Rand) Identity {
	n := len(ids)
	total := 0
	if len(weights) == n {
		for _, w := range weights {
			if w > 0 {
				total += w
			}
		}
	}
	if total <= 0 {
		return ids[rng.Intn(n)]
	}
	r := rng.Intn(total)
	acc := 0
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		acc += w
		if r < acc {
			return ids[i]
		}
	}
	return ids[n-1]
}
