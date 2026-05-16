package fabricate

import (
	"math/rand"
	"strings"
)

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

// EarnedWeights builds a weights slice parallel to ids for the --earned draw.
// Each player's weight is its real commit count from discovered (matched by
// mailmap-canonicalized, lowercased email). A player absent from discovered
// gets the rounded mean of the discovered counts (min 1) — an "average
// contributor". If discovered is empty there is nothing to weight by and nil
// is returned, signalling a uniform fallback.
func EarnedWeights(ids []Identity, discovered []DiscoveredIdentity, mm *Mailmap) []int {
	if len(discovered) == 0 {
		return nil
	}
	counts := make(map[string]int, len(discovered))
	total := 0
	for _, d := range discovered {
		counts[strings.ToLower(strings.TrimSpace(d.Email))] = d.Commits
		total += d.Commits
	}
	mean := int(float64(total)/float64(len(discovered)) + 0.5)
	if mean < 1 {
		mean = 1
	}
	weights := make([]int, len(ids))
	for i, id := range ids {
		c := mm.Canonical(id)
		if w, ok := counts[strings.ToLower(strings.TrimSpace(c.Email))]; ok {
			weights[i] = w
		} else {
			weights[i] = mean
		}
	}
	return weights
}
